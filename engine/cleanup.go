package engine

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupDownloads deletes each top-level item in the download directory
// that nothing has changed in for DeleteDownloadsAfterDays, and drops the
// finished torrents it belonged to. Items a torrent still needs are kept:
// one still downloading or paused part-way, or one waiting for metadata
// whose name matches. Hidden items (the .cloud-fetch state) are skipped.
// It returns the names it deleted.
func (e *Engine) CleanupDownloads(now time.Time) []string {
	e.mut.Lock()
	days := e.config.DeleteDownloadsAfterDays
	dir := e.config.DownloadDirectory
	e.mut.Unlock()
	if days <= 0 || dir == "" {
		return nil
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	// the slow part, walking the disk, happens before taking the lock
	var stale []string
	for _, ent := range entries {
		name := ent.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if newest, ok := newestChange(filepath.Join(dir, name)); ok && newest.Before(cutoff) {
			stale = append(stale, name)
		}
	}
	if len(stale) == 0 {
		return nil
	}

	e.mut.Lock()
	if e.client != nil {
		for _, tt := range e.client.Torrents() {
			e.upsertTorrent(tt)
		}
	}
	var removed []string
	for _, name := range stale {
		var finished []*Torrent
		needed := false
		for _, t := range e.ts {
			if !t.owns(name) {
				continue
			}
			if t.Complete() {
				finished = append(finished, t)
			} else {
				needed = true
			}
		}
		if needed {
			continue
		}
		for _, t := range finished {
			e.dropLocked(t)
		}
		removed = append(removed, name)
	}
	e.mut.Unlock()

	// deleting outside the lock keeps the UI's polls from waiting on a
	// large RemoveAll; the torrents are already dropped
	var deleted []string
	for _, name := range removed {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			log.Printf("Cleanup: deleting %s: %s", name, err)
			continue
		}
		log.Printf("Cleanup: deleted %s, unchanged for %d days", name, days)
		deleted = append(deleted, name)
	}
	return deleted
}

// owns reports whether the top-level download item name holds any of the
// torrent's files. Before metadata arrives the files are unknown, so the
// torrent's name stands in for them.
func (t *Torrent) owns(name string) bool {
	if !t.Loaded {
		return t.Name != "" && (name == t.Name || name == t.Name+".part")
	}
	for _, f := range t.Files {
		if f == nil {
			continue
		}
		top, _, _ := strings.Cut(f.Path, "/")
		if name == top || name == top+".part" {
			return true
		}
	}
	return false
}

// newestChange is the latest modification time of path or anything under it.
func newestChange(path string) (time.Time, bool) {
	var newest time.Time
	found := false
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil {
			found = true
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		return nil
	})
	return newest, found
}
