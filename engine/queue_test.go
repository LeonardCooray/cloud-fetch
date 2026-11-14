package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

func queueing(t *testing.T, dir string, max int) *Engine {
	t.Helper()
	return startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true, MaxActiveTorrents: max})
}

// addIdle adds a torrent with no peers and no data, so it never finishes.
func addIdle(t *testing.T, e *Engine, name string) string {
	t.Helper()
	mi := testMetainfo(t, name)
	mi.Announce = ""
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	return mi.HashInfoBytes().HexString()
}

func wantRunning(t *testing.T, e *Engine, name, ih string) {
	t.Helper()
	st := torrentState(t, e, ih)
	if !st.Started || st.Queued || st.QueuePosition != 0 || !st.downloading {
		t.Fatalf("%s: Started=%v Queued=%v QueuePosition=%d downloading=%v, want it downloading",
			name, st.Started, st.Queued, st.QueuePosition, st.downloading)
	}
}

func wantQueued(t *testing.T, e *Engine, name, ih string, pos int) {
	t.Helper()
	st := torrentState(t, e, ih)
	if !st.Started || !st.Queued || st.QueuePosition != pos || st.downloading {
		t.Fatalf("%s: Started=%v Queued=%v QueuePosition=%d downloading=%v, want it queued at %d",
			name, st.Started, st.Queued, st.QueuePosition, st.downloading, pos)
	}
}

// downloadingNow reads the engine's own state, without the queue pass
// GetTorrents runs first, which would cover up a wrong allow until then.
func downloadingNow(e *Engine, ih string) bool {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.ts[ih].downloading
}

func TestQueueStartsTorrentsInAddOrder(t *testing.T) {
	e := queueing(t, t.TempDir(), 2)
	a, b, c, d := addIdle(t, e, "a"), addIdle(t, e, "b"), addIdle(t, e, "c"), addIdle(t, e, "d")
	wantRunning(t, e, "a", a)
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "c", c, 1)
	wantQueued(t, e, "d", d, 2)
}

// The queue holds back real downloading, and a pause hands the slot on.
func TestQueuedTorrentDownloadsOnceASlotFrees(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := queueing(t, t.TempDir(), 1)
	first := addIdle(t, leech, "first")
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	wantQueued(t, leech, "seeded", ih, 1)
	connect(t, leech, seed, ih)
	stillAt(t, leech, ih, "pack/a.bin", 0, 2*time.Second)
	if err := leech.StopTorrent(first); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, leech, "seeded", ih)
	percentEventually(t, leech, ih, 100)
}

func TestFinishingFreesASlot(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := queueing(t, t.TempDir(), 1)
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	next := addIdle(t, leech, "next")
	wantQueued(t, leech, "next", next, 1)
	connect(t, leech, seed, ih)
	percentEventually(t, leech, ih, 100)
	wantRunning(t, leech, "next", next)
}

func TestResumeAndReaddJoinTheBackOfTheQueue(t *testing.T) {
	e := queueing(t, t.TempDir(), 1)
	a, b, c := addIdle(t, e, "a"), addIdle(t, e, "b"), addIdle(t, e, "c")
	if err := e.StopTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "c", c, 1)
	if st := torrentState(t, e, a); st.Queued || st.downloading {
		t.Fatalf("paused a: Queued=%v downloading=%v, want neither", st.Queued, st.downloading)
	}
	if err := e.StartTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "c", c, 1)
	wantQueued(t, e, "a", a, 2)

	// a re-add that resumes a paused torrent queues it the same way
	if err := e.StopTorrent(c); err != nil {
		t.Fatal(err)
	}
	addIdle(t, e, "c")
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "a", a, 1)
	wantQueued(t, e, "c", c, 2)
	// re-adding one that's already queued keeps its place
	addIdle(t, e, "a")
	wantQueued(t, e, "a", a, 1)
}

func TestAutoStartOffAddsTorrentsPausedNotQueued(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), MaxActiveTorrents: 1})
	a, b := addIdle(t, e, "a"), addIdle(t, e, "b")
	for name, ih := range map[string]string{"a": a, "b": b} {
		if st := torrentState(t, e, ih); st.Started || st.Queued {
			t.Fatalf("%s: Started=%v Queued=%v, want paused", name, st.Started, st.Queued)
		}
	}
	if err := e.StartTorrent(b); err != nil {
		t.Fatal(err)
	}
	if err := e.StartTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "a", a, 1)
}

func TestDeletingARunningTorrentFreesItsSlot(t *testing.T) {
	e := queueing(t, t.TempDir(), 1)
	a, b := addIdle(t, e, "a"), addIdle(t, e, "b")
	if err := e.DeleteTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "b", b)
}

func TestQueueSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	e := queueing(t, dir, 1)
	a, b, c := addIdle(t, e, "a"), addIdle(t, e, "b"), addIdle(t, e, "c")
	// a goes to the back, after c
	if err := e.StopTorrent(a); err != nil {
		t.Fatal(err)
	}
	if err := e.StartTorrent(a); err != nil {
		t.Fatal(err)
	}
	closeEngine(e)
	e = queueing(t, dir, 1)
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "c", c, 1)
	wantQueued(t, e, "a", a, 2)
	// a torrent added after the restart still goes behind them
	d := addIdle(t, e, "d")
	wantQueued(t, e, "d", d, 3)
}

// Records saved before the queue existed have no order; they're the oldest,
// so they keep their slots ahead of anything added later.
func TestRecordsFromBeforeTheQueueGoFirst(t *testing.T) {
	dir := t.TempDir()
	e := queueing(t, dir, 0)
	old := addIdle(t, e, "old")
	closeEngine(e)
	path := filepath.Join(dir, storeDirName, old+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var r map[string]any
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	delete(r, "order")
	if b, err = json.Marshal(r); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	e = queueing(t, dir, 1)
	fresh := addIdle(t, e, "fresh")
	wantRunning(t, e, "old", old)
	wantQueued(t, e, "fresh", fresh, 1)
}

func TestMaxActiveChangesWithoutRestart(t *testing.T) {
	e := queueing(t, t.TempDir(), 1)
	a, b := addIdle(t, e, "a"), addIdle(t, e, "b")
	before := e.client
	c := e.Config()
	c.MaxActiveTorrents = 0
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.client != before {
		t.Fatal("changing the max active torrents restarted the client")
	}
	wantRunning(t, e, "b", b)
	c.MaxActiveTorrents = 1
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "a", a)
	wantQueued(t, e, "b", b, 1)
	for _, bad := range []int{-1, 10001} {
		c.MaxActiveTorrents = bad
		if err := e.Configure(c); err == nil {
			t.Fatalf("max active %d accepted", bad)
		}
	}
}

func TestStartNowRunsOverTheLimitUntilASlotReachesIt(t *testing.T) {
	dir := t.TempDir()
	e := queueing(t, dir, 1)
	a, b, c := addIdle(t, e, "a"), addIdle(t, e, "b"), addIdle(t, e, "c")
	if err := e.StartNow(c); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "a", a)
	wantQueued(t, e, "b", b, 1)
	wantRunning(t, e, "c", c)

	// it survives a restart
	closeEngine(e)
	e = queueing(t, dir, 1)
	wantRunning(t, e, "a", a)
	wantQueued(t, e, "b", b, 1)
	wantRunning(t, e, "c", c)

	// a's slot goes to b, the next in line; c carries on as the extra
	if err := e.StopTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "b", b)
	wantRunning(t, e, "c", c)
	// b's goes to c, which stops being an extra, so a resumed a waits
	if err := e.StopTorrent(b); err != nil {
		t.Fatal(err)
	}
	if err := e.StartTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "c", c)
	wantQueued(t, e, "a", a, 1)

	if err := e.StartNow(c); err != nil {
		t.Fatalf("Start now on a running torrent: %v, want a no-op", err)
	}
	if err := e.StartNow(b); err == nil || !strings.Contains(err.Error(), "resume") {
		t.Fatalf("Start now on a paused torrent: %v, want a refusal", err)
	}
}

// Pausing an extra ends it: resumed, it queues like anything else.
func TestPausingAStartedNowTorrentEndsTheOverride(t *testing.T) {
	e := queueing(t, t.TempDir(), 1)
	a, b := addIdle(t, e, "a"), addIdle(t, e, "b")
	if err := e.StartNow(b); err != nil {
		t.Fatal(err)
	}
	if err := e.StopTorrent(b); err != nil {
		t.Fatal(err)
	}
	if err := e.StartTorrent(b); err != nil {
		t.Fatal(err)
	}
	wantRunning(t, e, "a", a)
	wantQueued(t, e, "b", b, 1)
}

func TestStreamingAQueuedTorrentStartsIt(t *testing.T) {
	leech := queueing(t, t.TempDir(), 1)
	first := addIdle(t, leech, "first")
	mi := multiFileTorrent(t, t.TempDir())
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	ih := mi.HashInfoBytes().HexString()
	wantQueued(t, leech, "streamed", ih, 1)
	r, ok, err := leech.StreamFile(context.Background(), "pack/a.bin")
	if err != nil || !ok {
		t.Fatalf("StreamFile = %v, %v; want a stream", ok, err)
	}
	r.Close()
	wantRunning(t, leech, "streamed", ih)
	wantRunning(t, leech, "first", first)
}

// The guard and the queue hold downloads separately: space coming back
// resumes only what has a slot, and a slot handed on while space is low
// stays held until it's back.
func TestQueueAndLowDiskGuardDontUndoEachOther(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true, MaxActiveTorrents: 1, MinimumFreeSpaceGB: 5})
	a, b, c := addIdle(t, e, "a"), addIdle(t, e, "b"), addIdle(t, e, "c")
	e.FreeSpace(4 * gb)
	for name, ih := range map[string]string{"a": a, "b": b, "c": c} {
		if st := torrentState(t, e, ih); !st.SpacePaused || st.downloading {
			t.Fatalf("%s while low: SpacePaused=%v downloading=%v, want held", name, st.SpacePaused, st.downloading)
		}
	}
	if err := e.StartNow(c); err == nil || !strings.Contains(err.Error(), "on hold") {
		t.Fatalf("Start now while low: %v, want the on-hold refusal", err)
	}
	// a's slot passes to b, but b stays held for space
	if err := e.StopTorrent(a); err != nil {
		t.Fatal(err)
	}
	if st := torrentState(t, e, b); st.Queued || !st.SpacePaused || st.downloading {
		t.Fatalf("b: Queued=%v SpacePaused=%v downloading=%v, want its slot but held for space", st.Queued, st.SpacePaused, st.downloading)
	}
	e.FreeSpace(100 * gb)
	if !downloadingNow(e, b) || downloadingNow(e, c) {
		t.Fatalf("space back: b downloading=%v, c downloading=%v; want only b, which has the slot", downloadingNow(e, b), downloadingNow(e, c))
	}
	wantRunning(t, e, "b", b)
	wantQueued(t, e, "c", c, 1)
	if st := torrentState(t, e, a); st.Started || st.downloading {
		t.Fatalf("hand-paused a: Started=%v downloading=%v, want it to stay paused", st.Started, st.downloading)
	}
	if err := e.StartTorrent(a); err != nil {
		t.Fatal(err)
	}
	wantQueued(t, e, "a", a, 2)
}
