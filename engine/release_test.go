package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

// heldFiles lists paths this process holds open or memory-mapped. A deleted
// file's space is only freed once nothing holds it, and a mapping doesn't
// show up as a file descriptor, so both are checked.
func heldFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	switch runtime.GOOS {
	case "linux":
		maps, err := os.ReadFile("/proc/self/maps")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(maps), "\n") {
			if f := strings.Fields(line); len(f) >= 6 {
				paths = append(paths, strings.Join(f[5:], " "))
			}
		}
		fds, _ := os.ReadDir("/proc/self/fd")
		for _, fd := range fds {
			if p, err := os.Readlink("/proc/self/fd/" + fd.Name()); err == nil {
				paths = append(paths, p)
			}
		}
	case "darwin":
		// lsof lists mapped files as well as descriptors
		out, err := exec.Command("lsof", "-nP", "-p", strconv.Itoa(os.Getpid()), "-Fn").Output()
		if err != nil {
			t.Skipf("lsof: %v", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n") {
				paths = append(paths, line[1:])
			}
		}
	default:
		t.Skipf("no way to list held files on %s", runtime.GOOS)
	}
	return paths
}

func heldUnder(t *testing.T, dir string) []string {
	t.Helper()
	var held []string
	for _, p := range heldFiles(t) {
		if strings.HasPrefix(p, dir+string(filepath.Separator)) {
			held = append(held, p)
		}
	}
	return held
}

// Deleting a torrent and then its files must give the disk space back. With
// anacrolix v1.59.1's default mmap file IO the mappings outlived the torrent,
// so the space stayed allocated until the process restarted.
func TestDeletedTorrentReleasesItsFiles(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	dir, err := filepath.EvalSymlinks(t.TempDir()) // lsof reports /private/var on macOS
	if err != nil {
		t.Fatal(err)
	}
	leech := startEngineWith(t, Config{DownloadDirectory: dir, AutoStart: true})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	connect(t, leech, seed, ih)
	percentEventually(t, leech, ih, 100)

	if err := leech.DeleteTorrent(ih); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "pack")); err != nil {
		t.Fatal(err)
	}
	// the client's piece-completion database stays open by design; only the
	// torrent's own files must be let go
	pack := filepath.Join(dir, "pack")
	var held []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if held = heldUnder(t, pack); len(held) == 0 {
			return
		}
	}
	t.Fatalf("deleted torrent's files still held by the process: %v", held)
}
