package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeonardCooray/cloud-fetch/engine"
)

func healthy(t *testing.T, auth string) *Server {
	t.Helper()
	s, root := newTestServer(t, auth)
	if err := reconfigureOnFreePort(t, s, engine.Config{DownloadDirectory: filepath.Join(root, "downloads")}); err != nil {
		t.Fatal(err)
	}
	return s
}

func healthz(t *testing.T, h http.Handler, method string, edit func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/healthz", nil)
	if edit != nil {
		edit(r)
	}
	return do(t, h, r)
}

func TestHealthzNeedsNoLoginAndSaysOnlyOK(t *testing.T) {
	s := healthy(t, "leo:pw")
	s.state.Stats.Version = "9.8.7"
	h := s.handler()
	w := healthz(t, h, "GET", nil)
	if w.Code != http.StatusOK || w.Body.String() != "ok\n" {
		t.Fatalf("got %d %q, want 200 \"ok\\n\"", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control %q, want no-store", got)
	}
	for name := range w.Header() {
		switch name {
		case "Content-Type", "Cache-Control":
		default:
			t.Errorf("unexpected header %s: %q", name, w.Header().Get(name))
		}
	}
	if w := healthz(t, h, "HEAD", nil); w.Code != http.StatusOK {
		t.Fatalf("HEAD status %d, want 200", w.Code)
	}
	// only the exact path skips the login
	r := httptest.NewRequest("GET", "/healthz/", nil)
	if w := do(t, h, r); w.Code != http.StatusUnauthorized {
		t.Fatalf("/healthz/ status %d, want 401", w.Code)
	}
}

func TestHealthzIsUnavailableWithoutATorrentClient(t *testing.T) {
	s, _ := newTestServer(t, "leo:pw") // engine never configured
	w := healthz(t, s.handler(), "GET", nil)
	if w.Code != http.StatusServiceUnavailable || w.Body.String() != "unavailable\n" {
		t.Fatalf("got %d %q, want 503 \"unavailable\\n\"", w.Code, w.Body.String())
	}
	// the reason ("torrent client not running") stays in the log
	s = healthy(t, "leo:pw")
	s.engine.Close()
	if w := healthz(t, s.handler(), "GET", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("after the client closed: status %d, want 503", w.Code)
	}
}

// Callers need no login, so they mustn't be able to make the engine take
// its locks more than once per healthCacheFor.
func TestHealthzReusesAResultForASecond(t *testing.T) {
	s := healthy(t, "leo:pw")
	h := s.handler()
	if w := healthz(t, h, "GET", nil); w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	s.engine.Close()
	if w := healthz(t, h, "GET", nil); w.Code != http.StatusOK {
		t.Fatalf("straight after: status %d, want the cached 200", w.Code)
	}
	time.Sleep(healthCacheFor + 100*time.Millisecond)
	if w := healthz(t, h, "GET", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("after %s: status %d, want 503", healthCacheFor, w.Code)
	}
}

func TestHealthzRefusesOtherMethods(t *testing.T) {
	s := healthy(t, "")
	w := healthz(t, s.handler(), "POST", nil)
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST got %d Allow %q, want 405 with Allow: GET, HEAD", w.Code, w.Header().Get("Allow"))
	}
}

// Credentials sent to /healthz are never checked, so they can't count
// toward the throttle, and a blocked address can still be monitored.
func TestHealthzBypassesTheLoginThrottle(t *testing.T) {
	s := healthy(t, "leo:pw")
	h := s.handler()
	const ip = "203.0.113.9"
	wrong := func(r *http.Request) {
		r.RemoteAddr = ip + ":40000"
		r.SetBasicAuth("leo", "guess")
	}
	for i := 0; i < 3*loginMaxFails; i++ {
		if w := healthz(t, h, "GET", wrong); w.Code != http.StatusOK {
			t.Fatalf("healthz with a wrong password, try %d: status %d, want 200", i+1, w.Code)
		}
	}
	if w := login(t, h, ip, "leo", "pw"); w.Code == http.StatusTooManyRequests || w.Code == http.StatusUnauthorized {
		t.Fatalf("login after healthz calls: status %d, so they counted as failures", w.Code)
	}
	for i := 0; i < loginMaxFails; i++ {
		login(t, h, ip, "leo", "guess")
	}
	if w := login(t, h, ip, "leo", "pw"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("login after %d failures: status %d, want 429", loginMaxFails, w.Code)
	}
	if w := healthz(t, h, "GET", wrong); w.Code != http.StatusOK {
		t.Fatalf("healthz from a blocked address: status %d, want 200", w.Code)
	}
}
