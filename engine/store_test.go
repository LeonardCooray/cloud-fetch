package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	ihA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ihB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func testMetainfo(t *testing.T, name string) *metainfo.MetaInfo {
	t.Helper()
	info := metainfo.Info{
		Name:        name,
		PieceLength: 16384,
		Length:      5,
		Pieces:      make([]byte, 20),
	}
	b, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &metainfo.MetaInfo{InfoBytes: b, Announce: "udp://tracker.example:80"}
}

func TestStoreRoundTripsRecord(t *testing.T) {
	s := newStore(t.TempDir())
	if err := s.save(record{InfoHash: ihA, Magnet: "magnet:?xt=urn:btih:" + ihA, Started: true}); err != nil {
		t.Fatal(err)
	}
	got, errs := s.load()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 saved torrent, got %d", len(got))
	}
	if got[0].InfoHash != ihA || got[0].Magnet != "magnet:?xt=urn:btih:"+ihA || !got[0].Started {
		t.Fatalf("record not round-tripped: %+v", got[0].record)
	}
	if got[0].Metainfo != nil {
		t.Fatalf("no metainfo was saved, but load returned one")
	}
}

func TestStoreSaveOverwritesStartedFlag(t *testing.T) {
	s := newStore(t.TempDir())
	s.save(record{InfoHash: ihA, Magnet: "magnet:?xt=urn:btih:" + ihA, Started: true})
	if err := s.save(record{InfoHash: ihA, Magnet: "magnet:?xt=urn:btih:" + ihA, Started: false}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.load()
	if len(got) != 1 || got[0].Started {
		t.Fatalf("want one stopped record, got %+v", got)
	}
}

func TestStoreRoundTripsMetainfo(t *testing.T) {
	s := newStore(t.TempDir())
	mi := testMetainfo(t, "a.txt")
	ih := mi.HashInfoBytes().HexString()
	if err := s.save(record{InfoHash: ih}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveMetainfo(ih, mi); err != nil {
		t.Fatal(err)
	}
	got, errs := s.load()
	if len(errs) != 0 || len(got) != 1 {
		t.Fatalf("want 1 torrent and no errors, got %d / %v", len(got), errs)
	}
	if got[0].Metainfo == nil {
		t.Fatal("metainfo not loaded")
	}
	if h := got[0].Metainfo.HashInfoBytes().HexString(); h != ih {
		t.Fatalf("loaded metainfo hash %s, want %s", h, ih)
	}
	if got[0].Metainfo.Announce != "udp://tracker.example:80" {
		t.Fatalf("trackers lost: %q", got[0].Metainfo.Announce)
	}
}

func TestStoreSkipsCorruptRecordButLoadsOthers(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	s.save(record{InfoHash: ihA, Magnet: "magnet:?xt=urn:btih:" + ihA})
	if err := os.WriteFile(filepath.Join(dir, ihB+".json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	got, errs := s.load()
	if len(got) != 1 || got[0].InfoHash != ihA {
		t.Fatalf("want only the good record, got %+v", got)
	}
	if len(errs) != 1 {
		t.Fatalf("want 1 error for the corrupt record, got %v", errs)
	}
}

func TestStoreSkipsRecordWithNothingToRestoreFrom(t *testing.T) {
	s := newStore(t.TempDir())
	s.save(record{InfoHash: ihA, Started: true})
	got, errs := s.load()
	if len(got) != 0 || len(errs) != 1 {
		t.Fatalf("want record skipped with an error, got %+v / %v", got, errs)
	}
}

func TestStoreFallsBackToMagnetWhenTorrentFileCorrupt(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	s.save(record{InfoHash: ihA, Magnet: "magnet:?xt=urn:btih:" + ihA})
	os.WriteFile(filepath.Join(dir, ihA+".torrent"), []byte("garbage"), 0600)
	got, errs := s.load()
	if len(got) != 1 || got[0].Metainfo != nil || got[0].Magnet == "" {
		t.Fatalf("want magnet-only record, got %+v", got)
	}
	if len(errs) != 1 {
		t.Fatalf("want the bad .torrent reported, got %v", errs)
	}
}

func TestStoreRemoveDeletesBothFiles(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	mi := testMetainfo(t, "a.txt")
	ih := mi.HashInfoBytes().HexString()
	s.save(record{InfoHash: ih})
	s.saveMetainfo(ih, mi)
	if err := s.remove(ih); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("want empty store dir, found %d entries", len(entries))
	}
}

func TestStoreRejectsInvalidInfoHash(t *testing.T) {
	dir := t.TempDir()
	s := newStore(filepath.Join(dir, "store"))
	for _, ih := range []string{"../../escape", "", "zz" + ihA[2:], ihA + "a"} {
		if err := s.save(record{InfoHash: ih, Magnet: "magnet:?"}); err == nil {
			t.Errorf("save accepted infohash %q", ih)
		}
		if err := s.remove(ih); err == nil {
			t.Errorf("remove accepted infohash %q", ih)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.json")); err == nil {
		t.Fatal("path traversal wrote outside the store")
	}
}

func TestStoreLoadMissingDirIsEmpty(t *testing.T) {
	s := newStore(filepath.Join(t.TempDir(), "never-created"))
	got, errs := s.load()
	if len(got) != 0 || len(errs) != 0 {
		t.Fatalf("want nothing, got %+v / %v", got, errs)
	}
}

func TestStoreLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := newStore(dir)
	mi := testMetainfo(t, "a.txt")
	ih := mi.HashInfoBytes().HexString()
	s.save(record{InfoHash: ih})
	s.saveMetainfo(ih, mi)
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != ih+".json" || names[1] != ih+".torrent" {
		t.Fatalf("want exactly %s.json and %s.torrent, got %v", ih, ih, names)
	}
}
