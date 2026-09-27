package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jpillora/archive"
)

const fileNumberLimit = 1000

type fsNode struct {
	Name     string
	Size     int64
	Modified time.Time
	Children []*fsNode
}

// listFiles expects s.state to be locked.
func (s *Server) listFiles() *fsNode {
	rootDir := s.state.Config.DownloadDirectory
	root := &fsNode{}
	if info, err := os.Stat(rootDir); err == nil {
		if err := list(rootDir, info, root, new(int)); err != nil {
			log.Printf("File listing failed: %s", err)
		}
	}
	return root
}

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/download/") {
		url := strings.TrimPrefix(r.URL.Path, "/download/")
		//dldir is absolute
		s.state.Lock()
		dldir := s.state.Config.DownloadDirectory
		s.state.Unlock()
		file := filepath.Join(dldir, url)
		if !insideDir(dldir, file) {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		info, err := os.Stat(file)
		if err != nil {
			http.Error(w, "File stat error: "+err.Error(), http.StatusBadRequest)
			return
		}
		switch r.Method {
		case "GET":
			if info.IsDir() {
				w.Header().Set("Content-Type", "application/zip")
				w.WriteHeader(200)
				//write .zip archive directly into response
				a := archive.NewZipWriter(w)
				a.AddDir(file)
				a.Close()
			} else {
				f, err := os.Open(file)
				if err != nil {
					http.Error(w, "File open error: "+err.Error(), http.StatusBadRequest)
					return
				}
				http.ServeContent(w, r, info.Name(), info.ModTime(), f)
				f.Close()
			}
		case "DELETE":
			if err := os.RemoveAll(file); err != nil {
				http.Error(w, "Delete failed: "+err.Error(), http.StatusInternalServerError)
			}
		default:
			http.Error(w, "Not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	s.static.ServeHTTP(w, r)
}

// insideDir reports whether p is strictly below dir. A plain prefix check
// would let "/dl" match "/dl2/secret".
func insideDir(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

//custom directory walk

func list(path string, info os.FileInfo, node *fsNode, n *int) error {
	if (!info.IsDir() && !info.Mode().IsRegular()) || strings.HasPrefix(info.Name(), ".") {
		return errors.New("Non-regular file")
	}
	(*n)++
	if (*n) > fileNumberLimit {
		return errors.New("Over file limit") //limit number of files walked
	}
	node.Name = info.Name()
	node.Size = info.Size()
	node.Modified = info.ModTime()
	if !info.IsDir() {
		return nil
	}
	children, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("Failed to list files")
	}
	node.Size = 0
	// non-nil so an empty folder marshals as [] and still reads as a folder
	node.Children = []*fsNode{}
	for _, e := range children {
		i, err := e.Info()
		if err != nil {
			continue
		}
		c := &fsNode{}
		p := filepath.Join(path, i.Name())
		if err := list(p, i, c, n); err != nil {
			continue
		}
		node.Size += c.Size
		node.Children = append(node.Children, c)
	}
	return nil
}
