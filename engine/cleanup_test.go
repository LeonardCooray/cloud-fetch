package engine

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

const day = 24 * time.Hour

// age backdates path and everything under it.
func age(t *testing.T, path string, d time.Duration) {
	t.Helper()
	when := time.Now().Add(-d)
	filepath.Walk(path, func(p string, _ os.FileInfo, err error) error {
		if err == nil {
			os.Chtimes(p, when, when)
		}
		return nil
	})
}

func write(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cleaning(t *testing.T, days int) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	return startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true, DeleteDownloadsAfterDays: days}), dir
}

func TestCleanupDeletesOnlyItemsUnchangedForTheSetDays(t *testing.T) {
	e, dir := cleaning(t, 7)
	write(t, filepath.Join(dir, "old.mkv"))
	age(t, filepath.Join(dir, "old.mkv"), 8*day)
	write(t, filepath.Join(dir, "Old Show", "e1.mkv"))
	age(t, filepath.Join(dir, "Old Show"), 30*day)
	write(t, filepath.Join(dir, "recent.mkv"))
	age(t, filepath.Join(dir, "recent.mkv"), 6*day)
	// hidden items hold the engine's state and the client's piece database
	for _, hidden := range []string{filepath.Join(storeDirName, "x.json"), ".torrent.db"} {
		write(t, filepath.Join(dir, hidden))
	}
	age(t, filepath.Join(dir, storeDirName), 30*day)
	age(t, filepath.Join(dir, ".torrent.db"), 30*day)
	// one fresh file keeps the whole folder
	write(t, filepath.Join(dir, "Mixed", "old.mkv"))
	age(t, filepath.Join(dir, "Mixed"), 30*day)
	write(t, filepath.Join(dir, "Mixed", "new.mkv"))

	got := e.CleanupDownloads(time.Now())
	sort.Strings(got)
	if want := []string{"Old Show", "old.mkv"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("deleted %v, want %v", got, want)
	}
	for _, name := range []string{"old.mkv", "Old Show"} {
		if exists(filepath.Join(dir, name)) {
			t.Errorf("%s still on disk", name)
		}
	}
	for _, name := range []string{"recent.mkv", "Mixed", storeDirName, ".torrent.db"} {
		if !exists(filepath.Join(dir, name)) {
			t.Errorf("%s was deleted", name)
		}
	}
}

func TestCleanupIsOffAtZeroDays(t *testing.T) {
	e, dir := cleaning(t, 0)
	write(t, filepath.Join(dir, "old.mkv"))
	age(t, filepath.Join(dir, "old.mkv"), 3650*day)
	if got := e.CleanupDownloads(time.Now()); len(got) != 0 {
		t.Fatalf("deleted %v with cleanup off", got)
	}
}

func TestCleanupDropsFinishedTorrentsWithTheirFiles(t *testing.T) {
	e, dir := cleaning(t, 7)
	mi := dataTorrent(t, dir, "done.bin", nil)
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	ih := mi.HashInfoBytes().HexString()
	completeEventually(t, e, ih)
	age(t, filepath.Join(dir, "done.bin"), 8*day)

	if got := e.CleanupDownloads(time.Now()); len(got) != 1 || got[0] != "done.bin" {
		t.Fatalf("deleted %v, want [done.bin]", got)
	}
	if exists(filepath.Join(dir, "done.bin")) {
		t.Error("finished torrent's file still on disk")
	}
	if _, ok := e.GetTorrents()[ih]; ok {
		t.Error("finished torrent still listed after its files were cleaned up")
	}
	e = restart(t, e, dir)
	if _, ok := e.GetTorrents()[ih]; ok {
		t.Error("cleaned-up torrent came back after restart")
	}
}

func TestCleanupKeepsWhatAnUnfinishedTorrentNeeds(t *testing.T) {
	e, dir := cleaning(t, 7)
	mi := dataTorrent(t, dir, "partial.bin", make([]byte, 300000))
	if err := e.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	ih := mi.HashInfoBytes().HexString()
	percentEventually(t, e, ih, 0)
	if err := e.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	// unfinished files carry .part until they complete
	part := filepath.Join(dir, "partial.bin.part")
	if !exists(part) {
		t.Fatal("no partial.bin.part on disk")
	}
	age(t, part, 30*day)

	if got := e.CleanupDownloads(time.Now()); len(got) != 0 {
		t.Fatalf("deleted %v, want nothing while a paused torrent needs it", got)
	}
	if !exists(part) {
		t.Error("paused torrent's partial data deleted")
	}
	if _, ok := e.GetTorrents()[ih]; !ok {
		t.Error("paused torrent dropped")
	}
}

func TestCleanupKeepsWhatAMagnetWaitingForMetadataNamed(t *testing.T) {
	e, dir := cleaning(t, 7)
	// no peers will ever send this metadata
	magnet := "magnet:?xt=urn:btih:" + "0123456789abcdef0123456789abcdef01234567" + "&dn=Waiting+Show"
	if err := e.NewMagnet(magnet); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "Waiting Show", "e1.mkv"))
	age(t, filepath.Join(dir, "Waiting Show"), 30*day)
	if got := e.CleanupDownloads(time.Now()); len(got) != 0 {
		t.Fatalf("deleted %v, want nothing a pending magnet names", got)
	}
}

func TestDeleteAfterDaysIsValidatedAndNeedsNoRestart(t *testing.T) {
	e, _ := cleaning(t, 7)
	before := e.client
	c := e.Config()
	c.DeleteDownloadsAfterDays = 30
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.client != before {
		t.Fatal("changing only the cleanup days restarted the torrent client")
	}
	for _, bad := range []int{-1, 3651} {
		c.DeleteDownloadsAfterDays = bad
		if err := e.Configure(c); err == nil {
			t.Fatalf("%d days accepted", bad)
		}
	}
}
