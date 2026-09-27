package engine

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// dataTorrent writes name with real content, builds its metainfo, then
// replaces the file with onDisk (nil keeps the real content).
func dataTorrent(t *testing.T, dir, name string, onDisk []byte) *metainfo.MetaInfo {
	t.Helper()
	data := make([]byte, 300000)
	for i := range data {
		data[i] = byte((i*31 + 7) % 251)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16384}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatal(err)
	}
	b, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk != nil {
		os.WriteFile(path, onDisk, 0644)
	}
	return &metainfo.MetaInfo{InfoBytes: b}
}

func percentEventually(t *testing.T, e *Engine, ih string, want float32) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var got float32 = -1
	for time.Now().Before(deadline) {
		if tr, ok := e.GetTorrents()[ih]; ok {
			got = tr.Percent
			if got == want {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("torrent %s at %v%%, want %v%%", ih, got, want)
}

// The storage layer assumes a same-size file at the final name is complete;
// adding a torrent must check that claim against the piece hashes.
func TestAddedTorrentWithWrongDataOnDiskIsNotComplete(t *testing.T) {
	dir := t.TempDir()
	mi := dataTorrent(t, dir, "junk.bin", bytes.Repeat([]byte{0x55}, 300000))
	e := startEngine(t, dir)
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	percentEventually(t, e, mi.HashInfoBytes().HexString(), 0)
}

func TestMagnetThenMetadataWithWrongDataOnDiskIsNotComplete(t *testing.T) {
	dir := t.TempDir()
	mi := dataTorrent(t, dir, "junk.bin", bytes.Repeat([]byte{0x55}, 300000))
	e := startEngine(t, dir)
	ih := mi.HashInfoBytes().HexString()
	if err := e.NewMagnet("magnet:?xt=urn:btih:" + ih); err != nil {
		t.Fatal(err)
	}
	// metadata arrives later, straight into the library as it would from peers
	h, _ := str2ih(ih)
	tt, ok := e.client.Torrent(h)
	if !ok {
		t.Fatal("magnet not in client")
	}
	if err := tt.SetInfoBytes(mi.InfoBytes); err != nil {
		t.Fatal(err)
	}
	percentEventually(t, e, ih, 0)
}

func TestAddedTorrentWithCorrectDataOnDiskIsComplete(t *testing.T) {
	dir := t.TempDir()
	mi := dataTorrent(t, dir, "good.bin", nil)
	e := startEngine(t, dir)
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	ih := mi.HashInfoBytes().HexString()
	percentEventually(t, e, ih, 100)
	time.Sleep(1500 * time.Millisecond) // still complete once verification has run
	percentEventually(t, e, ih, 100)
}

func runningTorrentSurvives(t *testing.T, e *Engine, ih string) {
	t.Helper()
	h, _ := str2ih(ih)
	if _, ok := e.client.Torrent(h); !ok {
		t.Fatal("torrent gone from the client")
	}
	if err := e.StopTorrent(ih); err != nil {
		t.Fatalf("torrent unusable after rejected reconfigure: %v", err)
	}
}

func TestConfigureRejectsBadPortsWithoutStoppingTorrents(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)
	for _, port := range []int{0, -1, 70000} {
		if err := e.Configure(Config{DownloadDirectory: dir, IncomingPort: port}); err == nil {
			t.Fatalf("port %d accepted", port)
		}
	}
	runningTorrentSurvives(t, e, ih)
}

func TestConfigurePortInUseKeepsTheOldClient(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)
	oldPort := e.Config().IncomingPort
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	if err := e.Configure(Config{DownloadDirectory: dir, IncomingPort: port}); err == nil {
		t.Fatal("configure succeeded on a port that is in use")
	}
	if got := e.Config().IncomingPort; got != oldPort {
		t.Fatalf("config says port %d, want the previous %d", got, oldPort)
	}
	runningTorrentSurvives(t, e, ih)
}
