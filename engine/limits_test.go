package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"
)

func TestSpeedLimitsChangeWithoutRestart(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	before := e.client
	c := e.Config()
	c.MaxDownloadKBps, c.MaxUploadKBps = 100, 2000
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.client != before {
		t.Fatal("changing the speed limits restarted the torrent client")
	}
	if got := e.limits.down.Limit(); got != 100000 {
		t.Fatalf("download limit %v B/s, want 100000", got)
	}
	if got, want := e.limits.up.Limit(), rate.Limit(2000000); got != want || e.limits.up.Burst() != 2000000 {
		t.Fatalf("upload limit %v burst %d, want %v and a 1 s burst", got, e.limits.up.Burst(), want)
	}
	if e.limits.down.Burst() != minBurst {
		t.Fatalf("download burst %d, want the %d floor", e.limits.down.Burst(), minBurst)
	}
	c.MaxDownloadKBps, c.MaxUploadKBps = 0, 0
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.limits.down.Limit() != rate.Inf || e.limits.up.Limit() != rate.Inf {
		t.Fatal("0 didn't lift the limits")
	}
	for _, bad := range []int{-1, maxRateKBps + 1} {
		c.MaxDownloadKBps = bad
		if err := e.Configure(c); err == nil {
			t.Fatalf("download limit %d accepted", bad)
		}
		c.MaxDownloadKBps, c.MaxUploadKBps = 0, bad
		if err := e.Configure(c); err == nil {
			t.Fatalf("upload limit %d accepted", bad)
		}
		c.MaxUploadKBps = 0
	}
}

// While uploads are unlimited anacrolix accepts peer requests up to its
// allocation cap; it panics if one turns out bigger than the upload burst,
// so a cap switched on later must never shrink the burst below that.
func TestUploadBurstAlwaysFitsTheLargestAcceptedRequest(t *testing.T) {
	l := newLimiters()
	for _, kbps := range []int{0, 1, 50, 100, 5000, 0} {
		cfg := clientConfig(Config{MaxUploadKBps: kbps}, l)
		if got, need := l.up.Burst(), cfg.MaxAllocPeerRequestDataPerConn; got < need {
			t.Fatalf("upload cap %d KB/s: burst %d, below the %d byte requests anacrolix accepts", kbps, got, need)
		}
	}
}

// A restart (here a port change) must keep the limits.
func TestSpeedLimitsSurviveAClientRestart(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true, MaxDownloadKBps: 300})
	c := e.Config()
	c.IncomingPort = freePort(t)
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if got := e.limits.down.Limit(); got != 300000 {
		t.Fatalf("download limit %v after a restart, want 300000", got)
	}
}

const bigSize = 4 << 20

// bigSeeder seeds one 4 MiB file, four times the largest first burst a
// limiter here allows, so a limit shows once that burst is spent.
func bigSeeder(t *testing.T, c Config) (*Engine, *metainfo.MetaInfo) {
	t.Helper()
	dir := t.TempDir()
	data := make([]byte, bigSize)
	for i := range data {
		data[i] = byte((i*13 + 5) % 251)
	}
	path := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 32 << 10}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatal(err)
	}
	b, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: b}
	c.DownloadDirectory, c.AutoStart, c.EnableUpload, c.EnableSeeding = dir, true, true, true
	e := startEngineWith(t, c)
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	percentEventually(t, e, mi.HashInfoBytes().HexString(), 100)
	return e, mi
}

// limitedThenLifted checks the transfer crawls for 3 s (unlimited, 4 MiB
// over loopback takes about a second), then lifts the limit on limited and
// expects the rest to arrive.
func limitedThenLifted(t *testing.T, leech, limited *Engine, ih string) {
	t.Helper()
	time.Sleep(3 * time.Second)
	// at most a 1 MiB burst + 3 s at 50 KB/s, about 29%
	if got := torrentState(t, leech, ih).Percent; got >= 60 {
		t.Fatalf("%v%% after 3 s at 50 KB/s, want the limit to hold it under 60%%", got)
	}
	before := limited.client
	c := limited.Config()
	c.MaxDownloadKBps, c.MaxUploadKBps = 0, 0
	if err := limited.Configure(c); err != nil {
		t.Fatal(err)
	}
	if limited.client != before {
		t.Fatal("lifting the limit restarted the client")
	}
	percentEventually(t, leech, ih, 100)
}

func TestDownloadLimitSlowsATransferUntilLifted(t *testing.T) {
	seed, mi := bigSeeder(t, Config{})
	ih := mi.HashInfoBytes().HexString()
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true, MaxDownloadKBps: 50})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	connect(t, leech, seed, ih)
	limitedThenLifted(t, leech, leech, ih)
}

func TestUploadLimitSlowsATransferUntilLifted(t *testing.T) {
	seed, mi := bigSeeder(t, Config{MaxUploadKBps: 50})
	ih := mi.HashInfoBytes().HexString()
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	connect(t, leech, seed, ih)
	limitedThenLifted(t, leech, seed, ih)
}
