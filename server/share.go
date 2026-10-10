package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	shareKeyName = "share.key"
	shareMaxTTL  = 7 * 24 * time.Hour
	// bytes of the HMAC kept in a link; 128 bits can't be guessed
	shareSigLen = 16
)

// shareKey returns the key share links are signed with, creating it next to
// the config file on first use so links survive restarts.
func (s *Server) shareKey() ([]byte, error) {
	s.shareMu.Lock()
	defer s.shareMu.Unlock()
	if s.shareSecret != nil {
		return s.shareSecret, nil
	}
	path := filepath.Join(filepath.Dir(s.ConfigPath), shareKeyName)
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		s.shareSecret = b
		return b, nil
	}
	return s.newShareKeyLocked(path)
}

// revokeShares replaces the key, which invalidates every link signed so far.
func (s *Server) revokeShares() error {
	s.shareMu.Lock()
	defer s.shareMu.Unlock()
	_, err := s.newShareKeyLocked(filepath.Join(filepath.Dir(s.ConfigPath), shareKeyName))
	return err
}

func (s *Server) newShareKeyLocked(path string) ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		return nil, fmt.Errorf("Can't save the share key: %s", err)
	}
	// WriteFile keeps an existing file's mode
	os.Chmod(path, 0600)
	s.shareSecret = b
	return b, nil
}

func shareSig(key []byte, expires int64, path string) string {
	m := hmac.New(sha256.New, key)
	fmt.Fprintf(m, "%d\n%s", expires, path)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:shareSigLen])
}

// shareLink is relative, like the UI's download links, so it works under
// whatever host and port the browser used.
func shareLink(key []byte, expires int64, path string) string {
	return fmt.Sprintf("share/%d/%s/%s", expires, shareSig(key, expires, path), (&url.URL{Path: path}).EscapedPath())
}

// serveShare serves /share/<expires>/<sig>/<path> without a login, by
// handing a checked request to the /download/ handler.
func (s *Server) serveShare(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/share/"), "/", 3)
	if len(parts) != 3 || parts[2] == "" {
		http.Error(w, "Invalid share link", http.StatusNotFound)
		return
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid share link", http.StatusNotFound)
		return
	}
	key, err := s.shareKey()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !hmac.Equal([]byte(parts[1]), []byte(shareSig(key, expires, parts[2]))) {
		http.Error(w, "Invalid share link", http.StatusForbidden)
		return
	}
	if time.Now().Unix() >= expires {
		http.Error(w, "This share link has expired", http.StatusGone)
		return
	}
	// /download/ also deletes; a share link only reads
	if r.Method != "GET" {
		http.Error(w, "Not allowed", http.StatusMethodNotAllowed)
		return
	}
	// a clone, so the request log still shows the /share/ path
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/download/" + parts[2]
	r2.URL.RawPath = ""
	s.files.ServeHTTP(w, r2)
}

type shareRequest struct {
	Paths []string
	TTLs  []int64 // seconds
}

type shareResponse struct {
	// Links[i][j] is Paths[j] for TTLs[i]
	Links   [][]string
	Expires []int64
}

func (s *Server) apiShare(r *http.Request) (*shareResponse, error) {
	data, err := readCapped(r.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read request body: %s", err)
	}
	var req shareRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("Invalid request: %s", err)
	}
	if len(req.Paths) == 0 || len(req.TTLs) == 0 {
		return nil, fmt.Errorf("Nothing to share")
	}
	s.state.Lock()
	dldir := s.state.Config.DownloadDirectory
	s.state.Unlock()
	for _, p := range req.Paths {
		if p == "" || !insideDir(dldir, filepath.Join(dldir, p)) {
			return nil, fmt.Errorf("Invalid path: %s", p)
		}
	}
	key, err := s.shareKey()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	res := &shareResponse{}
	for _, ttl := range req.TTLs {
		d := time.Duration(ttl) * time.Second
		if ttl <= 0 || d > shareMaxTTL {
			return nil, fmt.Errorf("A share link can last up to %s", shareMaxTTL)
		}
		expires := now.Add(d).Unix()
		links := make([]string, len(req.Paths))
		for i, p := range req.Paths {
			links[i] = shareLink(key, expires, p)
		}
		res.Links = append(res.Links, links)
		res.Expires = append(res.Expires, expires)
	}
	return res, nil
}

func (s *Server) handleShareAPI(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if r.Method != "POST" {
		http.Error(w, "Invalid request method (expecting POST)", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/api/share-revoke" {
		if err := s.revokeShares(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("OK"))
		return
	}
	res, err := s.apiShare(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
