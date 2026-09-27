package engine

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// freePort finds a port free for both TCP and UDP on every interface, since
// the client listens on both there.
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		u, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
		l.Close()
		if err == nil {
			u.Close()
			return port
		}
	}
	t.Fatal("no port free for both TCP and UDP")
	return 0
}

// startEngine stands in for a process start: a fresh Engine over dir, with
// AutoStart on as the server defaults it.
func startEngine(t *testing.T, dir string) *Engine {
	t.Helper()
	return startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true})
}

func closeEngine(e *Engine) {
	e.mut.Lock()
	defer e.mut.Unlock()
	e.closeClientLocked()
}

func restart(t *testing.T, e *Engine, dir string) *Engine {
	t.Helper()
	closeEngine(e)
	return startEngine(t, dir)
}

func addTestTorrent(t *testing.T, e *Engine) string {
	t.Helper()
	mi := testMetainfo(t, "a.txt")
	mi.Announce = ""
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	return mi.HashInfoBytes().HexString()
}

func torrentState(t *testing.T, e *Engine, ih string) *Torrent {
	t.Helper()
	tr, ok := e.GetTorrents()[ih]
	if !ok {
		t.Fatalf("torrent %s missing after restart", ih)
	}
	return tr
}

func TestAddedTorrentSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)

	e = restart(t, e, dir)
	tr := torrentState(t, e, ih)
	if !tr.Started {
		t.Fatal("a started torrent came back stopped")
	}
	if !tr.Loaded || tr.Name != "a.txt" {
		t.Fatalf("metadata not restored: loaded=%v name=%q", tr.Loaded, tr.Name)
	}
}

func TestStoppedTorrentStaysStoppedAfterRestart(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)
	if err := e.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}

	e = restart(t, e, dir)
	if torrentState(t, e, ih).Started {
		t.Fatal("a stopped torrent came back started")
	}
	if err := e.StartTorrent(ih); err != nil {
		t.Fatalf("restored stopped torrent can't be started: %v", err)
	}
}

func TestStopThenStartKeepsTorrentInClient(t *testing.T) {
	e := startEngine(t, t.TempDir())
	ih := addTestTorrent(t, e)
	if err := e.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	if err := e.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	h, _ := str2ih(ih)
	if _, ok := e.client.Torrent(h); !ok {
		t.Fatal("stop removed the torrent from the client, so start can't resume it")
	}
	if !torrentState(t, e, ih).Started {
		t.Fatal("torrent not marked started")
	}
}

func TestDeletedTorrentDoesNotComeBack(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)
	if err := e.DeleteTorrent(ih); err != nil {
		t.Fatal(err)
	}

	e = restart(t, e, dir)
	if _, ok := e.GetTorrents()[ih]; ok {
		t.Fatal("deleted torrent was restored")
	}
}

func TestReconfigureKeepsTorrents(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)

	if err := e.Configure(Config{DownloadDirectory: dir, IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	torrentState(t, e, ih)
	if err := e.StopTorrent(ih); err != nil {
		t.Fatalf("torrent unusable after reconfigure: %v", err)
	}
	h, _ := str2ih(ih)
	if _, ok := e.client.Torrent(h); !ok {
		t.Fatal("torrent listed but not in the new client")
	}
}

func TestChangingDownloadDirDropsTorrentsFromOldClient(t *testing.T) {
	e := startEngine(t, t.TempDir())
	ih := addTestTorrent(t, e)

	if err := e.Configure(Config{DownloadDirectory: t.TempDir(), IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.GetTorrents()[ih]; ok {
		t.Fatal("torrent from the old download dir still listed against a closed client")
	}
}

func TestMagnetSurvivesRestartBeforeMetadata(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	if err := e.NewMagnet("magnet:?xt=urn:btih:" + ihA + "&dn=example"); err != nil {
		t.Fatal(err)
	}

	e = restart(t, e, dir)
	tr := torrentState(t, e, ihA)
	if !tr.Started {
		t.Fatal("magnet came back stopped")
	}
	if tr.Loaded {
		t.Fatal("test magnet unexpectedly has metadata")
	}
}

func TestTorrentFileAddSavesMetainfo(t *testing.T) {
	dir := t.TempDir()
	e := startEngine(t, dir)
	ih := addTestTorrent(t, e)

	mi, err := metainfo.LoadFromFile(filepath.Join(dir, storeDirName, ih+".torrent"))
	if err != nil {
		t.Fatalf("no .torrent saved: %v", err)
	}
	if mi.HashInfoBytes().HexString() != ih {
		t.Fatal("saved .torrent has the wrong infohash")
	}
	if _, err := os.Stat(filepath.Join(dir, storeDirName, ih+".json")); err != nil {
		t.Fatalf("no record saved: %v", err)
	}
}
