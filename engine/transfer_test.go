package engine

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

const pieceLen = 16384

// multiFileTorrent writes a two-file torrent under dir/pack. Each file is a
// whole number of pieces, so no piece straddles both files.
func multiFileTorrent(t *testing.T, dir string) *metainfo.MetaInfo {
	t.Helper()
	root := filepath.Join(dir, "pack")
	os.MkdirAll(root, 0755)
	for i, name := range []string{"a.bin", "b.bin"} {
		data := make([]byte, 4*pieceLen)
		for j := range data {
			data[j] = byte((j*31 + i*7 + 3) % 251)
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	info := metainfo.Info{PieceLength: pieceLen}
	if err := info.BuildFromFilePath(root); err != nil {
		t.Fatal(err)
	}
	b, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &metainfo.MetaInfo{InfoBytes: b}
}

func startEngineWith(t *testing.T, c Config) *Engine {
	t.Helper()
	e := New()
	if c.IncomingPort == 0 {
		c.IncomingPort = freePort(t)
	}
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeEngine(e) })
	return e
}

// seeder serves mi from a directory that already holds the data.
func seeder(t *testing.T) (*Engine, *metainfo.MetaInfo) {
	t.Helper()
	dir := t.TempDir()
	mi := multiFileTorrent(t, dir)
	e := startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true, EnableUpload: true, EnableSeeding: true})
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	percentEventually(t, e, mi.HashInfoBytes().HexString(), 100)
	return e, mi
}

// connect keeps offering the seeder to the leecher until the test ends. A
// peer from AddPeers is dialled once and forgotten if that connection fails;
// trackers and the DHT offer peers again and again, so this does the same.
func connect(t *testing.T, leech, seed *Engine, ih string) {
	t.Helper()
	h, _ := str2ih(ih)
	tt, ok := leech.client.Torrent(h)
	if !ok {
		t.Fatal("torrent not in the leecher's client")
	}
	peer := []torrent.PeerInfo{{
		Addr:    &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: seed.Config().IncomingPort},
		Trusted: true,
	}}
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			tt.AddPeers(peer)
			select {
			case <-done:
				return
			case <-tt.Closed():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}

func filePercent(e *Engine, ih, path string) float32 {
	tr, ok := e.GetTorrents()[ih]
	if !ok {
		return -1
	}
	for _, f := range tr.Files {
		if f != nil && f.Path == path {
			return f.Percent
		}
	}
	return -1
}

// stillAt fails if the file's progress moves off want within d.
func stillAt(t *testing.T, e *Engine, ih, path string, want float32, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if got := filePercent(e, ih, path); got != want {
			t.Fatalf("%s at %v%%, want it to stay at %v%%", path, got, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func filePercentEventually(t *testing.T, e *Engine, ih, path string, want float32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var got float32
	for time.Now().Before(deadline) {
		if got = filePercent(e, ih, path); got == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s at %v%%, want %v%%", path, got, want)
}

func TestReaddingPausedTorrentResumesDownload(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	spec := torrent.TorrentSpecFromMetaInfo(mi)
	if err := leech.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}
	if err := leech.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if !torrentState(t, leech, ih).Started {
		t.Fatal("re-added torrent not marked started")
	}
	connect(t, leech, seed, ih)
	percentEventually(t, leech, ih, 100)
}

func TestAutoStartOffAddsTorrentsPaused(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: false})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if torrentState(t, leech, ih).Started {
		t.Fatal("AutoStart is off but the torrent was added started")
	}
	connect(t, leech, seed, ih)
	stillAt(t, leech, ih, "pack/a.bin", 0, 2*time.Second)
	if err := leech.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	percentEventually(t, leech, ih, 100)
}

func TestStoppedFileIsNotDownloaded(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: false})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if err := leech.StopFile(ih, "pack/b.bin"); err != nil {
		t.Fatal(err)
	}
	if err := leech.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	connect(t, leech, seed, ih)
	filePercentEventually(t, leech, ih, "pack/a.bin", 100)
	stillAt(t, leech, ih, "pack/b.bin", 0, 2*time.Second)

	if err := leech.StartFile(ih, "pack/b.bin"); err != nil {
		t.Fatal(err)
	}
	filePercentEventually(t, leech, ih, "pack/b.bin", 100)
}

func fileStarted(t *testing.T, e *Engine, ih, path string) bool {
	t.Helper()
	for _, f := range torrentState(t, e, ih).Files {
		if f != nil && f.Path == path {
			return f.Started
		}
	}
	t.Fatalf("no file %s", path)
	return false
}

func TestFileSelectionSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	mi := multiFileTorrent(t, t.TempDir())
	ih := mi.HashInfoBytes().HexString()
	e := startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true})
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if err := e.StopFile(ih, "pack/b.bin"); err != nil {
		t.Fatal(err)
	}

	closeEngine(e)
	e = startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true})
	if !fileStarted(t, e, ih, "pack/a.bin") {
		t.Fatal("a.bin came back stopped")
	}
	if fileStarted(t, e, ih, "pack/b.bin") {
		t.Fatal("b.bin came back started")
	}
}

func TestNewTorrentFilesStartSelected(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	mi := multiFileTorrent(t, t.TempDir())
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	ih := mi.HashInfoBytes().HexString()
	for _, p := range []string{"pack/a.bin", "pack/b.bin"} {
		if !fileStarted(t, e, ih, p) {
			t.Fatalf("%s not selected on a new torrent", p)
		}
	}
}

func TestFileControlsRejectUnknownFiles(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	ih := addTestTorrent(t, e)
	if err := e.StopFile(ih, "nope.txt"); err == nil {
		t.Fatal("stopping a file that isn't in the torrent succeeded")
	}
	if err := e.StartFile(ih, "nope.txt"); err == nil {
		t.Fatal("starting a file that isn't in the torrent succeeded")
	}
}

func TestV2OnlyMagnetIsRefused(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	v2 := "magnet:?xt=urn:btmh:1220" + "cafe0000000000000000000000000000000000000000000000000000000000ee"
	if err := e.NewMagnet(v2); err == nil {
		t.Fatal("a v2-only magnet was accepted")
	}
	if n := len(e.GetTorrents()); n != 0 {
		t.Fatalf("%d torrents listed after a refused magnet", n)
	}
}

func TestHybridMagnetWithBtmhFirstIsAccepted(t *testing.T) {
	e := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	hybrid := "magnet:?xt=urn:btmh:1220" + "cafe0000000000000000000000000000000000000000000000000000000000ee" +
		"&xt=urn:btih:" + ihA
	if err := e.NewMagnet(hybrid); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.GetTorrents()[ihA]; !ok {
		t.Fatalf("hybrid magnet not listed under its v1 hash; have %v", e.GetTorrents())
	}
}

func TestDisableEncryptionForcesPlaintext(t *testing.T) {
	on := clientConfig(Config{})
	if !on.HeaderObfuscationPolicy.Preferred || on.HeaderObfuscationPolicy.RequirePreferred {
		t.Fatalf("default policy %+v, want encryption preferred but not required", on.HeaderObfuscationPolicy)
	}
	off := clientConfig(Config{DisableEncryption: true})
	if off.HeaderObfuscationPolicy.Preferred || !off.HeaderObfuscationPolicy.RequirePreferred {
		t.Fatalf("DisableEncryption policy %+v, want plaintext only", off.HeaderObfuscationPolicy)
	}
}
