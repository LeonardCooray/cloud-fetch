package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

func TestCompleteFollowsPickedFiles(t *testing.T) {
	f := func(started bool, pct float32) *File { return &File{Started: started, Percent: pct} }
	for _, tc := range []struct {
		name string
		t    Torrent
		want bool
	}{
		{"not loaded", Torrent{Percent: 100}, false},
		{"all picked files done", Torrent{Loaded: true, Percent: 60, Files: []*File{f(true, 100), f(false, 0)}}, true},
		{"a picked file unfinished", Torrent{Loaded: true, Percent: 90, Files: []*File{f(true, 100), f(true, 80)}}, false},
		{"none picked, whole torrent done", Torrent{Loaded: true, Percent: 100, Files: []*File{f(false, 100)}}, true},
		{"none picked, whole torrent unfinished", Torrent{Loaded: true, Percent: 50, Files: []*File{f(false, 50)}}, false},
		{"still verifying", Torrent{Loaded: true, Percent: 100, verify: &verification{done: make(chan struct{})}}, false},
		{"verified", Torrent{Loaded: true, Percent: 100, verify: &verification{done: closedDone()}}, true},
	} {
		if got := tc.t.Complete(); got != tc.want {
			t.Errorf("%s: Complete() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestClearFinishedDropsOnlyCompleteTorrentsAndKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	done := dataTorrent(t, dir, "done.bin", nil)
	partial := dataTorrent(t, dir, "partial.bin", make([]byte, 300000))
	e := startEngine(t, dir)
	for _, mi := range []*torrent.TorrentSpec{torrent.TorrentSpecFromMetaInfo(done), torrent.TorrentSpecFromMetaInfo(partial)} {
		if err := e.NewTorrent(mi); err != nil {
			t.Fatal(err)
		}
	}
	doneIH, partialIH := done.HashInfoBytes().HexString(), partial.HashInfoBytes().HexString()
	// the zeroed file reads as complete until verification checks it
	percentEventually(t, e, partialIH, 0)
	completeEventually(t, e, doneIH)

	if n := e.ClearFinished(); n != 1 {
		t.Fatalf("ClearFinished() = %d, want 1", n)
	}
	ts := e.GetTorrents()
	if _, ok := ts[doneIH]; ok {
		t.Error("finished torrent still listed")
	}
	if _, ok := ts[partialIH]; !ok {
		t.Error("unfinished torrent was cleared")
	}
	if _, err := os.Stat(filepath.Join(dir, "done.bin")); err != nil {
		t.Errorf("finished torrent's file gone: %v", err)
	}

	e = restart(t, e, dir)
	if _, ok := e.GetTorrents()[doneIH]; ok {
		t.Error("cleared torrent came back after restart")
	}
	if n := e.ClearFinished(); n != 0 {
		t.Errorf("second ClearFinished() = %d, want 0", n)
	}
}

func completeEventually(t *testing.T, e *Engine, ih string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if tr, ok := e.GetTorrents()[ih]; ok && tr.Complete() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("torrent %s never complete", ih)
}

func closedDone() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}
