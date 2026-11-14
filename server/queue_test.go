package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LeonardCooray/cloud-fetch/engine"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// idleTorrent has no data and no peers, so it never finishes.
func idleTorrent(t *testing.T, name string) *metainfo.MetaInfo {
	t.Helper()
	b, err := bencode.Marshal(metainfo.Info{Name: name, PieceLength: 16384, Length: 5, Pieces: make([]byte, 20)})
	if err != nil {
		t.Fatal(err)
	}
	return &metainfo.MetaInfo{InfoBytes: b}
}

func TestStartNowActionAndQueueSettingsAreSaved(t *testing.T) {
	s, root := newTestServer(t, "")
	c := engine.Config{DownloadDirectory: filepath.Join(root, "downloads"), AutoStart: true, MaxActiveTorrents: 1, MaxDownloadKBps: 700, MaxUploadKBps: 80}
	if err := reconfigureOnFreePort(t, s, c); err != nil {
		t.Fatal(err)
	}
	var ihs []string
	for _, name := range []string{"a", "b"} {
		mi := idleTorrent(t, name)
		if err := s.engine.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
			t.Fatal(err)
		}
		ihs = append(ihs, mi.HashInfoBytes().HexString())
	}
	if !s.engine.GetTorrents()[ihs[1]].Queued {
		t.Fatal("second torrent not queued with Max Active Torrents 1")
	}
	if w := post(t, s, "/api/torrent", strings.NewReader("startnow:"+ihs[1])); w.Code != http.StatusOK {
		t.Fatalf("startnow: %d %s", w.Code, w.Body.String())
	}
	if s.engine.GetTorrents()[ihs[1]].Queued {
		t.Fatal("still queued after Start now")
	}
	if w := post(t, s, "/api/torrent", strings.NewReader("startnow:nothex")); w.Code == http.StatusOK {
		t.Fatal("startnow on a bad infohash succeeded")
	}

	b, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved engine.Config
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.MaxActiveTorrents != 1 || saved.MaxDownloadKBps != 700 || saved.MaxUploadKBps != 80 {
		t.Fatalf("saved config %+v, want the queue and speed settings kept", saved)
	}
}
