package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shareLinks asks the API, logged in, for links to paths lasting ttls.
func shareLinks(t *testing.T, h http.Handler, ttls []int64, paths ...string) shareResponse {
	t.Helper()
	body, _ := json.Marshal(shareRequest{Paths: paths, TTLs: ttls})
	r := httptest.NewRequest("POST", "/api/share", strings.NewReader(string(body)))
	r.SetBasicAuth("leo", "pw")
	w := do(t, h, r)
	if w.Code != http.StatusOK {
		t.Fatalf("share API: %d %s", w.Code, w.Body)
	}
	var res shareResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func get(t *testing.T, h http.Handler, link string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h, httptest.NewRequest("GET", "/"+link, nil))
}

func TestShareLinkServesWithoutLogin(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.MkdirAll(filepath.Join(root, "downloads", "Show S01"), 0755)
	os.WriteFile(filepath.Join(root, "downloads", "Show S01", "ep 1.mkv"), movie(), 0644)
	h := s.handler()

	res := shareLinks(t, h, []int64{3600, 86400}, "Show S01/ep 1.mkv")
	if len(res.Links) != 2 || len(res.Links[0]) != 1 || len(res.Expires) != 2 {
		t.Fatalf("got %+v, want one link per TTL", res)
	}
	link := res.Links[1][0]
	if !strings.HasPrefix(link, "share/") || !strings.HasSuffix(link, "/Show%20S01/ep%201.mkv") {
		t.Fatalf("link %q", link)
	}
	if want := time.Now().Add(24 * time.Hour).Unix(); res.Expires[1] < want-5 || res.Expires[1] > want {
		t.Errorf("24h link expires at %d, want about %d", res.Expires[1], want)
	}

	w := get(t, h, link)
	if w.Code != http.StatusOK || w.Body.Len() != 100 {
		t.Fatalf("share link: %d, %d bytes", w.Code, w.Body.Len())
	}
	if w.Header().Get("Content-Encoding") != "" {
		t.Errorf("share response is %s-encoded; downloads must not be", w.Header().Get("Content-Encoding"))
	}

	r := httptest.NewRequest("GET", "/"+link, nil)
	r.Header.Set("Range", "bytes=10-19")
	w = do(t, h, r)
	if w.Code != http.StatusPartialContent || w.Body.Bytes()[0] != 10 {
		t.Fatalf("Range: got %d, first byte %v", w.Code, w.Body.Bytes())
	}

	w = do(t, h, httptest.NewRequest("GET", "/download/Show%20S01/ep%201.mkv", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("/download/ without login: got %d, want 401", w.Code)
	}
}

func TestShareLinkSharesFolderAsZip(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.MkdirAll(filepath.Join(root, "downloads", "Album"), 0755)
	os.WriteFile(filepath.Join(root, "downloads", "Album", "a.mp3"), movie(), 0644)
	h := s.handler()
	w := get(t, h, shareLinks(t, h, []int64{3600}, "Album").Links[0][0])
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("folder link: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestShareLinkRefusesTamperingAndExpiry(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "downloads", "a.mkv"), movie(), 0644)
	os.WriteFile(filepath.Join(root, "downloads", "b.mkv"), movie(), 0644)
	h := s.handler()
	link := shareLinks(t, h, []int64{3600}, "a.mkv").Links[0][0]
	parts := strings.SplitN(link, "/", 4) // share, expires, sig, path
	key, _ := s.shareKey()
	past := time.Now().Add(-time.Second).Unix()

	for _, tc := range []struct {
		name, link string
		want       int
	}{
		{"other file", strings.Join([]string{parts[0], parts[1], parts[2], "b.mkv"}, "/"), http.StatusForbidden},
		{"later expiry", strings.Join([]string{parts[0], "9999999999", parts[2], parts[3]}, "/"), http.StatusForbidden},
		{"bad signature", strings.Join([]string{parts[0], parts[1], "AAAAAAAAAAAAAAAAAAAAAA", parts[3]}, "/"), http.StatusForbidden},
		{"expired", shareLink(key, past, "a.mkv"), http.StatusGone},
		{"no path", "share/123/abc", http.StatusNotFound},
		{"not a number", "share/soon/abc/a.mkv", http.StatusNotFound},
	} {
		if w := get(t, h, tc.link); w.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}

func TestShareLinkCannotDelete(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	file := filepath.Join(root, "downloads", "keep.mkv")
	os.WriteFile(file, movie(), 0644)
	h := s.handler()
	link := shareLinks(t, h, []int64{3600}, "keep.mkv").Links[0][0]
	if w := do(t, h, httptest.NewRequest("DELETE", "/"+link, nil)); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: got %d, want 405", w.Code)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("file gone after DELETE on a share link: %v", err)
	}
}

func TestShareRefusesPathsOutsideDownloads(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "secret.txt"), []byte("x"), 0644)
	h := s.handler()
	for _, p := range []string{"../secret.txt", "", "a/../../secret.txt"} {
		body, _ := json.Marshal(shareRequest{Paths: []string{p}, TTLs: []int64{3600}})
		r := httptest.NewRequest("POST", "/api/share", strings.NewReader(string(body)))
		r.SetBasicAuth("leo", "pw")
		if w := do(t, h, r); w.Code != http.StatusBadRequest {
			t.Errorf("share %q: got %d, want 400", p, w.Code)
		}
	}
	// even a correctly signed traversal is stopped by the download handler
	key, _ := s.shareKey()
	w := get(t, h, shareLink(key, time.Now().Add(time.Hour).Unix(), "../secret.txt"))
	if w.Code == http.StatusOK {
		t.Fatalf("signed ../ link served the file")
	}
}

func TestShareAPIValidatesTTLAndLogin(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "downloads", "a.mkv"), movie(), 0644)
	h := s.handler()
	for _, ttl := range []int64{0, -1, int64(shareMaxTTL/time.Second) + 1} {
		body, _ := json.Marshal(shareRequest{Paths: []string{"a.mkv"}, TTLs: []int64{ttl}})
		r := httptest.NewRequest("POST", "/api/share", strings.NewReader(string(body)))
		r.SetBasicAuth("leo", "pw")
		if w := do(t, h, r); w.Code != http.StatusBadRequest {
			t.Errorf("ttl %d: got %d, want 400", ttl, w.Code)
		}
	}
	shareLinks(t, h, []int64{int64(shareMaxTTL / time.Second)}, "a.mkv")

	body, _ := json.Marshal(shareRequest{Paths: []string{"a.mkv"}, TTLs: []int64{3600}})
	for _, path := range []string{"/api/share", "/api/share-revoke"} {
		w := do(t, h, httptest.NewRequest("POST", path, strings.NewReader(string(body))))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s without login: got %d, want 401", path, w.Code)
		}
	}
}

func TestRevokeInvalidatesShareLinks(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "downloads", "a.mkv"), movie(), 0644)
	h := s.handler()
	link := shareLinks(t, h, []int64{3600}, "a.mkv").Links[0][0]
	r := httptest.NewRequest("POST", "/api/share-revoke", nil)
	r.SetBasicAuth("leo", "pw")
	if w := do(t, h, r); w.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w.Code, w.Body)
	}
	if w := get(t, h, link); w.Code != http.StatusForbidden {
		t.Fatalf("old link after revoke: got %d, want 403", w.Code)
	}
	if w := get(t, h, shareLinks(t, h, []int64{3600}, "a.mkv").Links[0][0]); w.Code != http.StatusOK {
		t.Fatalf("new link after revoke: got %d, want 200", w.Code)
	}
}

func TestShareKeySurvivesRestart(t *testing.T) {
	s, root := newTestServer(t, "leo:pw")
	os.WriteFile(filepath.Join(root, "downloads", "a.mkv"), movie(), 0644)
	link := shareLinks(t, s.handler(), []int64{3600}, "a.mkv").Links[0][0]

	info, err := os.Stat(filepath.Join(root, shareKeyName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("key file mode %o, want 600", info.Mode().Perm())
	}

	s2, _ := newTestServer(t, "leo:pw")
	s2.ConfigPath = s.ConfigPath
	s2.state.Config.DownloadDirectory = s.state.Config.DownloadDirectory
	if w := get(t, s2.handler(), link); w.Code != http.StatusOK {
		t.Fatalf("link after restart: got %d, want 200", w.Code)
	}
}
