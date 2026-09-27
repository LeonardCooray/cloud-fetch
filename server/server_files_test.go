package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFilesWalksTreeSkippingHiddenAndSymlinks(t *testing.T) {
	s, root := newTestServer(t, "")
	dl := filepath.Join(root, "downloads")
	os.MkdirAll(filepath.Join(dl, "Show", "S01"), 0755)
	os.WriteFile(filepath.Join(dl, "Show", "S01", "e1.mkv"), make([]byte, 300), 0644)
	os.WriteFile(filepath.Join(dl, "Show", "notes.txt"), make([]byte, 20), 0644)
	os.WriteFile(filepath.Join(dl, "b.iso"), make([]byte, 7), 0644)
	os.MkdirAll(filepath.Join(dl, ".cloud-fetch"), 0700)
	os.WriteFile(filepath.Join(dl, ".cloud-fetch", "x.json"), make([]byte, 1000), 0600)
	os.Symlink("/etc", filepath.Join(dl, "etc-link"))

	tree := s.listFiles()

	if tree.Size != 327 {
		t.Fatalf("root size %d, want 327 (hidden store and symlink excluded)", tree.Size)
	}
	var names []string
	for _, c := range tree.Children {
		names = append(names, c.Name)
	}
	if len(names) != 2 || names[0] != "Show" || names[1] != "b.iso" {
		t.Fatalf("root children %v, want [Show b.iso]", names)
	}
	show := tree.Children[0]
	if show.Size != 320 || len(show.Children) != 2 || show.Children[0].Name != "S01" || show.Children[1].Name != "notes.txt" {
		t.Fatalf("Show node wrong: size %d children %d", show.Size, len(show.Children))
	}
	if ep := show.Children[0].Children; len(ep) != 1 || ep[0].Name != "e1.mkv" || ep[0].Size != 300 {
		t.Fatalf("S01 contents wrong: %+v", ep)
	}
}

// The UI tells folders from files by Children being a list, so an empty
// folder must send [] rather than null.
func TestListFilesSendsEmptyFolderAsFolder(t *testing.T) {
	s, root := newTestServer(t, "")
	os.MkdirAll(filepath.Join(root, "downloads", "empty"), 0755)

	b, err := json.Marshal(s.listFiles())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"Name":"empty","Size":0,`) || !strings.Contains(string(b), `"Children":[]`) {
		t.Fatalf("empty folder not sent as a folder: %s", b)
	}
}
