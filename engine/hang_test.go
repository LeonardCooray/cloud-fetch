package engine

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// within fails the test if f hasn't returned after d, so a deadlock fails
// fast instead of running into go test's 10 minute timeout.
func within(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s still blocked after %s", what, d)
	}
}

// manyPieces has thousands of pieces and no data on disk, so verification
// is still walking the pieces when the test acts on the torrent.
func manyPieces(t *testing.T) *metainfo.MetaInfo {
	t.Helper()
	const n = 4096
	hashes := make([]byte, n*20)
	rand.Read(hashes)
	info := metainfo.Info{Name: "big.bin", PieceLength: 16384, Length: n * 16384, Pieces: hashes}
	b, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &metainfo.MetaInfo{InfoBytes: b}
}

// anacrolix/torrent v1.59.1 returns from Piece.VerifyDataContext with the
// client lock still held when the torrent has closed, which froze every
// later call on the client (CI run 36301714788).
func TestDeletingDuringVerificationDoesNotFreezeTheClient(t *testing.T) {
	for i := 0; i < 20; i++ {
		e := startEngine(t, t.TempDir())
		mi := manyPieces(t)
		if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Duration(i) * time.Millisecond)
		if err := e.DeleteTorrent(mi.HashInfoBytes().HexString()); err != nil {
			t.Fatal(err)
		}
		within(t, 5*time.Second, "GetTorrents after delete", func() { e.GetTorrents() })
		within(t, 5*time.Second, "closing the client", func() { closeEngine(e) })
	}
}

func TestReconfiguringDuringVerificationDoesNotFreeze(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		e := startEngine(t, dir)
		mi := manyPieces(t)
		if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Duration(i) * time.Millisecond)
		within(t, 10*time.Second, "reconfigure", func() {
			if err := e.Configure(Config{DownloadDirectory: dir, AutoStart: true, IncomingPort: freePort(t)}); err != nil {
				t.Error(err)
			}
		})
		within(t, 5*time.Second, "GetTorrents after reconfigure", func() { e.GetTorrents() })
		within(t, 5*time.Second, "closing the client", func() { closeEngine(e) })
	}
}

// If the new client can't start and the previous config can't be brought
// back either, the engine must say so rather than keep a closed client.
func TestFailedRestoreLeavesEngineStoppedNotHung(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)

	attempts := 0
	realNew := newClient
	newClient = func(c Config) (*torrent.Client, error) {
		attempts++
		return nil, errors.New("port taken")
	}
	t.Cleanup(func() { newClient = realNew })

	within(t, 10*time.Second, "failing reconfigure", func() {
		if err := e.Configure(Config{DownloadDirectory: dir, AutoStart: true, IncomingPort: freePort(t)}); err == nil {
			t.Error("reconfigure succeeded with no client")
		}
	})
	if attempts != 1+restoreAttempts {
		t.Fatalf("%d client starts, want 1 for the new config plus %d restores", attempts, restoreAttempts)
	}

	calls := map[string]func() error{
		"NewMagnet":     func() error { return e.NewMagnet("magnet:?xt=urn:btih:" + ihB) },
		"NewTorrent":    func() error { return e.NewTorrent(torrent.TorrentSpecFromMetaInfo(testMetainfo(t, "b.txt"))) },
		"StartTorrent":  func() error { return e.StartTorrent(ih) },
		"StopTorrent":   func() error { return e.StopTorrent(ih) },
		"DeleteTorrent": func() error { return e.DeleteTorrent(ih) },
		"StartFile":     func() error { return e.StartFile(ih, "a.txt") },
		"StopFile":      func() error { return e.StopFile(ih, "a.txt") },
	}
	for name, call := range calls {
		within(t, 2*time.Second, name, func() {
			if err := call(); err == nil || !strings.Contains(err.Error(), "not running") {
				t.Errorf("%s: got %v, want a 'not running' error", name, err)
			}
		})
	}
	within(t, 2*time.Second, "GetTorrents", func() {
		if got := e.GetTorrents(); len(got) != 0 {
			t.Errorf("GetTorrents lists %d torrents from a closed client", len(got))
		}
	})

	newClient = realNew
	if err := e.Configure(Config{DownloadDirectory: dir, AutoStart: true, IncomingPort: freePort(t)}); err != nil {
		t.Fatalf("engine can't recover once a client starts again: %v", err)
	}
	torrentState(t, e, ih)
}

func TestRestoreRetriesUntilThePreviousConfigStarts(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)

	calls := 0
	realNew := newClient
	newClient = func(c Config) (*torrent.Client, error) {
		calls++
		if calls <= 3 { // the new config, then two failed restores
			return nil, errors.New("port taken")
		}
		return realNew(c)
	}
	t.Cleanup(func() { newClient = realNew })

	if err := e.Configure(Config{DownloadDirectory: dir, AutoStart: true, IncomingPort: freePort(t)}); err == nil {
		t.Fatal("reconfigure reported success")
	}
	if calls != 4 {
		t.Fatalf("%d client starts, want 4", calls)
	}
	runningTorrentSurvives(t, e, ih)
}
