package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

const gb = int64(1) << 30

func guarded(t *testing.T, autoStart bool) *Engine {
	t.Helper()
	return startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: autoStart, MinimumFreeSpaceGB: 5})
}

func TestLowDiskHoldsDownloadsAndResumesThem(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := guarded(t, true)
	if !leech.FreeSpace(4 * gb) {
		t.Fatal("4 GB free with a 5 GB minimum: want downloads held")
	}
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	st := torrentState(t, leech, ih)
	if !st.Started || !st.SpacePaused {
		t.Fatalf("torrent added while low: Started=%v SpacePaused=%v, want both", st.Started, st.SpacePaused)
	}
	connect(t, leech, seed, ih)
	stillAt(t, leech, ih, "pack/a.bin", 0, 2*time.Second)

	// above the minimum but inside the margin: still held
	if !leech.FreeSpace(5*gb + gb/2) {
		t.Fatal("5.5 GB free: want downloads still held")
	}
	stillAt(t, leech, ih, "pack/a.bin", 0, time.Second)

	if leech.FreeSpace(6 * gb) {
		t.Fatal("6 GB free: want downloads resumed")
	}
	if torrentState(t, leech, ih).SpacePaused {
		t.Fatal("SpacePaused still set after space came back")
	}
	percentEventually(t, leech, ih, 100)
}

func TestLowDiskHoldsRunningDownloads(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := guarded(t, true)
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	leech.FreeSpace(4 * gb)
	if !torrentState(t, leech, ih).SpacePaused {
		t.Fatal("running torrent not held when space ran low")
	}
	connect(t, leech, seed, ih)
	stillAt(t, leech, ih, "pack/a.bin", 0, 2*time.Second)
	leech.FreeSpace(100 * gb)
	percentEventually(t, leech, ih, 100)
}

func TestLowDiskLeavesHandPausedTorrentsPaused(t *testing.T) {
	_, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := guarded(t, true)
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if err := leech.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	leech.FreeSpace(4 * gb)
	err := leech.StartTorrent(ih)
	if err == nil || !strings.Contains(err.Error(), "on hold") {
		t.Fatalf("StartTorrent while low: err = %v, want the on-hold refusal", err)
	}
	leech.FreeSpace(100 * gb)
	st := torrentState(t, leech, ih)
	if st.Started || st.SpacePaused {
		t.Fatalf("hand-paused torrent after space came back: Started=%v SpacePaused=%v, want neither", st.Started, st.SpacePaused)
	}
	if err := leech.StartTorrent(ih); err != nil {
		t.Fatalf("StartTorrent with space back: %v", err)
	}
}

func TestPausingAHeldTorrentKeepsItPaused(t *testing.T) {
	_, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := guarded(t, true)
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	leech.FreeSpace(4 * gb)
	if err := leech.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	leech.FreeSpace(100 * gb)
	if st := torrentState(t, leech, ih); st.Started || st.SpacePaused {
		t.Fatalf("Started=%v SpacePaused=%v, want the hand pause to stick", st.Started, st.SpacePaused)
	}
}

func TestZeroMinimumTurnsTheGuardOff(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	if e.FreeSpace(0) {
		t.Fatal("guard held downloads with MinimumFreeSpaceGB 0")
	}
}

func TestChangingTheMinimumKeepsTheClient(t *testing.T) {
	e := guarded(t, true)
	before := e.client
	c := e.Config()
	c.MinimumFreeSpaceGB = 20
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.client != before {
		t.Fatal("changing only the minimum restarted the torrent client")
	}
	if !e.FreeSpace(10 * gb) {
		t.Fatal("10 GB free with the new 20 GB minimum: want downloads held")
	}
	c.MinimumFreeSpaceGB = 0
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.FreeSpace(10 * gb) {
		t.Fatal("guard turned off but downloads still held")
	}
	for _, bad := range []int{-1, 1000001} {
		c.MinimumFreeSpaceGB = bad
		if err := e.Configure(c); err == nil {
			t.Fatalf("minimum %d accepted", bad)
		}
	}
}
