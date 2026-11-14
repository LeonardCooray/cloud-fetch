package engine

import (
	"errors"
	"testing"
	"time"
)

func TestCheckPassesWithARunningClient(t *testing.T) {
	e := startEngine(t, t.TempDir())
	defer closeEngine(e)
	if err := e.Check(time.Second); err != nil {
		t.Fatalf("Check on a running engine: %v", err)
	}
}

func TestCheckFailsWithoutAClient(t *testing.T) {
	e := New()
	if err := e.Check(time.Second); !errors.Is(err, errNotRunning) {
		t.Fatalf("Check before Configure: %v, want %v", err, errNotRunning)
	}
	e = startEngine(t, t.TempDir())
	e.Close()
	if err := e.Check(time.Second); !errors.Is(err, errNotRunning) {
		t.Fatalf("Check after Close: %v, want %v", err, errNotRunning)
	}
}

// A wedged engine must fail the check quickly, and repeated checks must not
// each leave another goroutine stuck on the lock.
func TestCheckTimesOutOnAWedgedEngineWithOneProbe(t *testing.T) {
	e := startEngine(t, t.TempDir())
	defer closeEngine(e)
	e.mut.Lock()
	start := time.Now()
	if err := e.Check(50 * time.Millisecond); !errors.Is(err, errCheckTimeout) {
		e.mut.Unlock()
		t.Fatalf("Check while wedged: %v, want %v", err, errCheckTimeout)
	}
	if took := time.Since(start); took > time.Second {
		e.mut.Unlock()
		t.Fatalf("Check took %s to give up", took)
	}
	e.probeMu.Lock()
	first := e.probe
	e.probeMu.Unlock()
	for i := 0; i < 5; i++ {
		e.Check(10 * time.Millisecond)
	}
	e.probeMu.Lock()
	same := e.probe == first
	e.probeMu.Unlock()
	e.mut.Unlock()
	if first == nil || !same {
		t.Fatal("repeated checks started new probes instead of sharing the stuck one")
	}
	// once the lock frees up the stuck probe finishes and checks pass again
	deadline := time.Now().Add(5 * time.Second)
	for e.Check(time.Second) != nil {
		if time.Now().After(deadline) {
			t.Fatal("Check still failing after the engine was released")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A result isn't reused past a close or a start: the client it was about has
// changed.
func TestCheckIsFreshAfterClose(t *testing.T) {
	e := startEngine(t, t.TempDir())
	if err := e.Check(time.Second); err != nil {
		t.Fatalf("Check on a running engine: %v", err)
	}
	e.Close()
	if err := e.Check(time.Second); !errors.Is(err, errNotRunning) {
		t.Fatalf("Check straight after Close: %v, want %v", err, errNotRunning)
	}
	// and the reverse: a client that just started isn't reported missing
	if err := configureOnFreePort(t, e, e.Config()); err != nil {
		t.Fatal(err)
	}
	defer closeEngine(e)
	if err := e.Check(time.Second); err != nil {
		t.Fatalf("Check straight after Configure: %v", err)
	}
}

// The health check needs no login, so a result is reused rather than taking
// the engine's locks for every caller.
func TestCheckReusesAResultForASecond(t *testing.T) {
	e := startEngine(t, t.TempDir())
	defer closeEngine(e)
	if err := e.Check(time.Second); err != nil {
		t.Fatalf("first Check: %v", err)
	}
	e.mut.Lock()
	client := e.client
	e.client = nil
	e.mut.Unlock()
	defer func() {
		e.mut.Lock()
		e.client = client
		e.mut.Unlock()
	}()
	if err := e.Check(time.Second); err != nil {
		t.Fatalf("Check straight after: %v, want the reused nil", err)
	}
	time.Sleep(checkReuse + 100*time.Millisecond)
	if err := e.Check(time.Second); !errors.Is(err, errNotRunning) {
		t.Fatalf("Check after %s: %v, want %v", checkReuse, err, errNotRunning)
	}
}
