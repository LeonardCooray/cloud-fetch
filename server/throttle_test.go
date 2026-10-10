package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jpillora/cookieauth"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time                    { return c.t }
func (c *fakeClock) add(d time.Duration)               { c.t = c.t.Add(d) }
func newFakeClock() *fakeClock                         { return &fakeClock{t: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)} }
func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func throttled(clock *fakeClock) http.Handler {
	lt := newLoginThrottle()
	lt.now = clock.now
	return lt.wrap(cookieauth.New().SetUserPass("leo", "pw").Wrap, http.HandlerFunc(okHandler))
}

func login(t *testing.T, h http.Handler, ip, user, pass string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = net.JoinHostPort(ip, "40000")
	if user != "" || pass != "" {
		r.SetBasicAuth(user, pass)
	}
	return do(t, h, r)
}

func TestLoginThrottleBlocksAfterFiveFailures(t *testing.T) {
	clock := newFakeClock()
	h := throttled(clock)
	for i := 0; i < loginMaxFails; i++ {
		if w := login(t, h, "203.0.113.9", "leo", "guess"); w.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: got %d, want 401", i+1, w.Code)
		}
	}
	w := login(t, h, "203.0.113.9", "leo", "pw")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("right password while blocked: got %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "900" {
		t.Errorf("Retry-After = %q, want 900", got)
	}
	clock.add(loginBlock - time.Second)
	if w := login(t, h, "203.0.113.9", "leo", "pw"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("a second before the block ends: got %d, want 429", w.Code)
	}
	clock.add(time.Second)
	if w := login(t, h, "203.0.113.9", "leo", "pw"); w.Code != http.StatusOK {
		t.Fatalf("after the block: got %d, want 200", w.Code)
	}
}

func TestLoginThrottleIgnoresPromptRequests(t *testing.T) {
	h := throttled(newFakeClock())
	for i := 0; i < 20; i++ {
		if w := login(t, h, "203.0.113.9", "", ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("no credentials: got %d, want 401", w.Code)
		}
	}
	if w := login(t, h, "203.0.113.9", "leo", "pw"); w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestLoginThrottleSuccessResetsCount(t *testing.T) {
	h := throttled(newFakeClock())
	for round := 0; round < 3; round++ {
		for i := 0; i < loginMaxFails-1; i++ {
			login(t, h, "203.0.113.9", "leo", "guess")
		}
		if w := login(t, h, "203.0.113.9", "leo", "pw"); w.Code != http.StatusOK {
			t.Fatalf("round %d: got %d, want 200", round, w.Code)
		}
	}
}

func TestLoginThrottleForgetsFailuresOutsideWindow(t *testing.T) {
	clock := newFakeClock()
	h := throttled(clock)
	for i := 0; i < loginMaxFails-1; i++ {
		login(t, h, "203.0.113.9", "leo", "guess")
	}
	clock.add(loginWindow + time.Second)
	for i := 0; i < loginMaxFails-1; i++ {
		login(t, h, "203.0.113.9", "leo", "guess")
	}
	if w := login(t, h, "203.0.113.9", "leo", "pw"); w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
}

func TestLoginThrottleIsPerClient(t *testing.T) {
	h := throttled(newFakeClock())
	for i := 0; i < loginMaxFails; i++ {
		login(t, h, "203.0.113.9", "leo", "guess")
		login(t, h, "2001:db8::1", "leo", "guess")
	}
	for _, tc := range []struct {
		ip   string
		want int
	}{
		{"203.0.113.9", http.StatusTooManyRequests},
		{"203.0.113.10", http.StatusOK},
		{"2001:db8::1", http.StatusTooManyRequests},
		{"2001:db8::ffff", http.StatusTooManyRequests}, // same /64
		{"2001:db8:0:1::1", http.StatusOK},
	} {
		if w := login(t, h, tc.ip, "leo", "pw"); w.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.ip, w.Code, tc.want)
		}
	}
}

func TestLoginThrottleCountsBadCookies(t *testing.T) {
	h := throttled(newFakeClock())
	for i := 0; i < loginMaxFails; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: authCookie, Value: "bm90LWEtaGFzaA=="})
		if w := do(t, h, r); w.Code != http.StatusUnauthorized {
			t.Fatalf("bad cookie: got %d, want 401", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("leo", "pw")
	if w := do(t, h, r); w.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", w.Code)
	}
}

// gatedAuth checks Basic credentials only once release is closed, so a test
// can hold many logins in flight at once.
func gatedAuth(release <-chan struct{}) func(http.Handler) http.Handler {
	return func(inner http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
			if u, p, ok := r.BasicAuth(); ok && u == "leo" && p == "pw" {
				inner.ServeHTTP(w, r)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
}

// burst sends n logins at once and returns the status codes.
func burst(t *testing.T, h http.Handler, n int, pass string, release chan struct{}) map[int]int {
	t.Helper()
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("GET", "/", nil)
			r.SetBasicAuth("leo", pass)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	// the refused ones answer without waiting; give the rest time to queue
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for c := range codes {
		got[c]++
	}
	return got
}

func TestLoginThrottleLimitsSimultaneousGuesses(t *testing.T) {
	release := make(chan struct{})
	h := newLoginThrottle().wrap(gatedAuth(release), http.HandlerFunc(okHandler))
	got := burst(t, h, 50, "guess", release)
	if got[http.StatusUnauthorized] != loginMaxFails || got[http.StatusTooManyRequests] != 50-loginMaxFails {
		t.Fatalf("50 simultaneous guesses: got %v, want %d checked and the rest refused", got, loginMaxFails)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("leo", "pw")
	if w := do(t, h, r); w.Code != http.StatusTooManyRequests {
		t.Fatalf("after the burst: got %d, want 429", w.Code)
	}
}

// IDM opens many connections with the same Basic header; once that header
// has worked, they aren't treated as guesses.
func TestLoginThrottleLetsKnownCredentialsThroughInParallel(t *testing.T) {
	lt := newLoginThrottle()
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("leo", "pw")
	if w := do(t, lt.wrap(gatedAuth(closedChan()), http.HandlerFunc(okHandler)), r); w.Code != http.StatusOK {
		t.Fatalf("first login: got %d, want 200", w.Code)
	}
	release := make(chan struct{})
	got := burst(t, lt.wrap(gatedAuth(release), http.HandlerFunc(okHandler)), 16, "pw", release)
	if got[http.StatusOK] != 16 {
		t.Fatalf("16 parallel logins with a known header: got %v, want all 200", got)
	}
}

func TestServerHandlerThrottlesLogins(t *testing.T) {
	s, _ := newTestServer(t, "leo:pw")
	h := s.handler()
	for i := 0; i < loginMaxFails; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.SetBasicAuth("leo", "guess")
		do(t, h, r)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("leo", "pw")
	if w := do(t, h, r); w.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", w.Code)
	}
}

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
