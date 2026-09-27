package server

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LeonardCooray/cloud-fetch/engine"
	ctstatic "github.com/LeonardCooray/cloud-fetch/static"
)

// newTestServer mirrors what Run wires up, minus the listener and pollers.
func newTestServer(t *testing.T, auth string) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	dl := filepath.Join(root, "downloads")
	if err := os.Mkdir(dl, 0755); err != nil {
		t.Fatal(err)
	}
	s := &Server{Auth: auth, ConfigPath: filepath.Join(root, DefaultConfigName)}
	s.state.Users = map[string]string{}
	s.state.Config.DownloadDirectory = dl
	s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.engine = engine.New()
	return s, root
}

func do(t *testing.T, h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func movie() []byte {
	b := make([]byte, 100)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// IDM (multi-connection) and VLC (seeking) both rely on Range requests
// authenticated with basic auth.
func TestDownloadWithBasicAuthServesByteRanges(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "downloads", "movie.mkv"), movie(), 0644)
	h := s.handler()

	r := httptest.NewRequest("GET", "/download/movie.mkv", nil)
	r.SetBasicAuth("leo", "pw")
	r.Header.Set("Range", "bytes=10-19")
	r.Header.Set("Accept-Encoding", "gzip")
	w := do(t, h, r)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status %d, want 206", w.Code)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 10-19/100" {
		t.Fatalf("Content-Range %q", got)
	}
	if !bytes.Equal(w.Body.Bytes(), []byte{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}) {
		t.Fatalf("wrong bytes: %v", w.Body.Bytes())
	}
}

func TestDownloadWithoutCredentialsIsRefusedWhenAuthSet(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "downloads", "movie.mkv"), movie(), 0644)
	w := do(t, s.handler(), httptest.NewRequest("GET", "/download/movie.mkv", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", w.Code)
	}
}

func TestDownloadRejectsSiblingDirectoryWithSharedPrefix(t *testing.T) {
	s, root := newTestServer(t, "")
	os.Mkdir(filepath.Join(root, "downloads2"), 0755)
	os.WriteFile(filepath.Join(root, "downloads2", "secret.txt"), []byte("top secret"), 0644)

	w := do(t, s.handler(), httptest.NewRequest("GET", "/download/../downloads2/secret.txt", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
	if strings.Contains(w.Body.String(), "top secret") {
		t.Fatal("served a file outside the download directory")
	}
	if strings.Contains(w.Body.String(), root) {
		t.Fatalf("error leaks server paths: %q", w.Body.String())
	}
}

func TestDeleteRejectsSiblingDirectoryWithSharedPrefix(t *testing.T) {
	s, root := newTestServer(t, "")
	os.Mkdir(filepath.Join(root, "downloads2"), 0755)
	victim := filepath.Join(root, "downloads2", "keep.txt")
	os.WriteFile(victim, []byte("x"), 0644)

	do(t, s.handler(), httptest.NewRequest("DELETE", "/download/../downloads2/keep.txt", nil))
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("deleted a file outside the download directory")
	}
}

func TestDownloadRejectsParentTraversal(t *testing.T) {
	s, root := newTestServer(t, "")
	os.WriteFile(filepath.Join(root, "outside.txt"), []byte("nope"), 0644)
	w := do(t, s.handler(), httptest.NewRequest("GET", "/download/../outside.txt", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
}

func TestDownloadServesNestedFile(t *testing.T) {
	s, root := newTestServer(t, "")
	os.MkdirAll(filepath.Join(root, "downloads", "Show", "S01"), 0755)
	os.WriteFile(filepath.Join(root, "downloads", "Show", "S01", "e1.mkv"), []byte("episode"), 0644)
	w := do(t, s.handler(), httptest.NewRequest("GET", "/download/Show/S01/e1.mkv", nil))
	if w.Code != http.StatusOK || w.Body.String() != "episode" {
		t.Fatalf("status %d body %q", w.Code, w.Body.String())
	}
}

func post(t *testing.T, s *Server, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, s.handler(), httptest.NewRequest("POST", path, body))
}

func TestAddByURLRefusesOversizedTorrent(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("d"), maxTorrentBytes+1))
	}))
	defer big.Close()
	s, _ := newTestServer(t, "")

	w := post(t, s, "/api/url", strings.NewReader(big.URL))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "too large") {
		t.Fatalf("status %d body %q, want 400 too large", w.Code, w.Body.String())
	}
}

func TestAddByURLGivesUpOnSlowServer(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer slow.Close()
	defer close(release)
	old := remoteFetchTimeout
	remoteFetchTimeout = 200 * time.Millisecond
	defer func() { remoteFetchTimeout = old }()
	s, _ := newTestServer(t, "")

	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- post(t, s, "/api/url", strings.NewReader(slow.URL)) }()
	select {
	case w := <-done:
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", w.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("add-by-URL hung on a server that never responds")
	}
}

func TestUploadRefusesOversizedBody(t *testing.T) {
	s, _ := newTestServer(t, "")
	w := post(t, s, "/api/torrentfile", bytes.NewReader(bytes.Repeat([]byte("d"), maxTorrentBytes+1)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "too large") {
		t.Fatalf("status %d body %q, want 400 too large", w.Code, w.Body.String())
	}
}

// freePort finds a port free for TCP and UDP on every interface, since the
// torrent client listens on both there. Something else can still take it
// before it's used, so callers that bind it retry on "address already in use".
func freePort(t *testing.T) int {
	t.Helper()
	for i := 0; i < 50; i++ {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		u, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
		l.Close()
		if err == nil {
			u.Close()
			return port
		}
	}
	t.Fatal("no port free for both TCP and UDP")
	return 0
}

func TestConfigFileIsOwnerOnly(t *testing.T) {
	s, root := newTestServer(t, "")
	// existing installs already have a 0755 file; the fix must tighten it
	os.WriteFile(s.ConfigPath, []byte("{}"), 0755)
	err := s.reconfigure(engine.Config{DownloadDirectory: filepath.Join(root, "downloads"), IncomingPort: freePort(t)})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("config file mode %o, want 600", perm)
	}
}

func TestListenHost(t *testing.T) {
	for _, tc := range []struct{ host, auth, want string }{
		{"", "", "127.0.0.1"},
		{"", "leo:pw", "0.0.0.0"},
		{"0.0.0.0", "", "0.0.0.0"},
		{"10.0.0.5", "leo:pw", "10.0.0.5"},
	} {
		if got := listenHost(tc.host, tc.auth); got != tc.want {
			t.Errorf("listenHost(%q, %q) = %q, want %q", tc.host, tc.auth, got, tc.want)
		}
	}
}
