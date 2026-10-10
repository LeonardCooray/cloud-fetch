package server

import (
	"bytes"
	"context"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeonardCooray/cloud-fetch/engine"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// clipTorrent writes a four-piece single-file torrent's data into dir.
func clipTorrent(t *testing.T, dir string) (*metainfo.MetaInfo, []byte) {
	t.Helper()
	data := make([]byte, 4*16384)
	for i := range data {
		data[i] = byte((i*13 + 5) % 251)
	}
	p := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: 16384}
	if err := info.BuildFromFilePath(p); err != nil {
		t.Fatal(err)
	}
	b, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &metainfo.MetaInfo{InfoBytes: b}, data
}

// seedTo serves mi from dir and keeps dialling the engine listening on port
// until the test ends, since one failed dial would be forgotten.
func seedTo(t *testing.T, dir string, mi *metainfo.MetaInfo, port int) {
	t.Helper()
	c := torrent.NewDefaultClientConfig()
	c.DataDir = dir
	c.Seed = true
	c.ListenPort = 0
	c.NoDHT = true
	cl, err := torrent.NewClient(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	tt, err := cl.AddTorrent(mi)
	if err != nil {
		t.Fatal(err)
	}
	if err := tt.VerifyData(); err != nil {
		t.Fatal(err)
	}
	peer := []torrent.PeerInfo{{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}, Trusted: true}}
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			tt.AddPeers(peer)
			select {
			case <-done:
				return
			case <-time.After(time.Second):
			}
		}
	}()
}

// The file is deselected, so it isn't on disk at all until the request asks
// for its pieces: a 206 with the right bytes can only come from the stream.
func TestDownloadStreamsAnUnfinishedFileWithRanges(t *testing.T) {
	s, root := newTestServer(t, "")
	dl := filepath.Join(root, "downloads")
	if err := reconfigureOnFreePort(t, s, engine.Config{DownloadDirectory: dl, AutoStart: true}); err != nil {
		t.Fatal(err)
	}
	seedDir := t.TempDir()
	mi, data := clipTorrent(t, seedDir)
	ih := mi.HashInfoBytes().HexString()
	if err := s.engine.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if err := s.engine.StopFile(ih, "clip.mp4"); err != nil {
		t.Fatal(err)
	}
	seedTo(t, seedDir, mi, s.engine.Config().IncomingPort)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r := httptest.NewRequest("GET", "/download/clip.mp4", nil).WithContext(ctx)
	r.Header.Set("Range", "bytes=40000-40099")
	w := do(t, s.handler(), r)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status %d (%s), want 206", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 40000-40099/65536" {
		t.Fatalf("Content-Range %q", got)
	}
	// set from the name rather than sniffed (the host's MIME table decides
	// which); Go's own table has no .mp4
	want := mime.TypeByExtension(".mp4")
	if want == "" {
		want = "application/octet-stream"
	}
	if got := w.Header().Get("Content-Type"); got != want {
		t.Fatalf("Content-Type %q, want %q", got, want)
	}
	if !bytes.Equal(w.Body.Bytes(), data[40000:40100]) {
		t.Fatal("streamed bytes differ from the file's")
	}
}

func TestDownloadOfAPausedUnfinishedFileIsAConflict(t *testing.T) {
	s, root := newTestServer(t, "")
	dl := filepath.Join(root, "downloads")
	if err := reconfigureOnFreePort(t, s, engine.Config{DownloadDirectory: dl, AutoStart: false}); err != nil {
		t.Fatal(err)
	}
	mi, _ := clipTorrent(t, t.TempDir())
	if err := s.engine.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	w := do(t, s.handler(), httptest.NewRequest("GET", "/download/clip.mp4", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("status %d (%s), want 409", w.Code, w.Body.String())
	}
}
