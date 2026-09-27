package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/acme/autocert"
)

func TestCertManagerOnlyAcceptsConfiguredDomain(t *testing.T) {
	s := &Server{Domain: "fetch.example.com", ConfigPath: filepath.Join(t.TempDir(), "c.json")}
	m, err := s.certManager()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.HostPolicy(context.Background(), "fetch.example.com"); err != nil {
		t.Fatalf("configured domain refused: %v", err)
	}
	// otherwise anyone could point a hostname at the VPS and burn our rate limit
	if err := m.HostPolicy(context.Background(), "attacker.example.net"); err == nil {
		t.Fatal("certificate would be requested for an unconfigured host")
	}
}

// autocert's HTTP-01 handler checks r.Host verbatim, which carries a port
// whenever the challenge port isn't 80.
func TestCertManagerAcceptsConfiguredDomainWithPort(t *testing.T) {
	s := &Server{Domain: "fetch.example.com", ConfigPath: "c.json"}
	m, _ := s.certManager()
	if err := m.HostPolicy(context.Background(), "fetch.example.com:8080"); err != nil {
		t.Fatalf("challenge Host with port refused: %v", err)
	}
	if err := m.HostPolicy(context.Background(), "attacker.example.net:8080"); err == nil {
		t.Fatal("unconfigured host accepted when it carries a port")
	}
}

func TestCertManagerCachesNextToConfigByDefault(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Domain: "fetch.example.com", ConfigPath: filepath.Join(dir, "cloud-torrent.json")}
	m, err := s.certManager()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Cache.(autocert.DirCache); !ok || string(got) != filepath.Join(dir, "certs") {
		t.Fatalf("cache %#v, want DirCache %q", m.Cache, filepath.Join(dir, "certs"))
	}
}

func TestCertManagerUsesCertCacheFlag(t *testing.T) {
	s := &Server{Domain: "fetch.example.com", ConfigPath: "c.json", CertCache: "/var/lib/cloud-fetch/certs"}
	m, _ := s.certManager()
	if got, _ := m.Cache.(autocert.DirCache); string(got) != "/var/lib/cloud-fetch/certs" {
		t.Fatalf("cache %#v", m.Cache)
	}
}

func TestCertManagerDirectory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		s       *Server
		wantURL string
	}{
		{"production", &Server{}, "https://acme-v02.api.letsencrypt.org/directory"},
		{"staging", &Server{ACMEStaging: true}, "https://acme-staging-v02.api.letsencrypt.org/directory"},
		{"custom", &Server{ACMEDirectory: "https://ca.internal/dir"}, "https://ca.internal/dir"},
	} {
		tc.s.Domain, tc.s.ConfigPath = "fetch.example.com", "c.json"
		m, err := tc.s.certManager()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := autocert.DefaultACMEDirectory
		if m.Client != nil && m.Client.DirectoryURL != "" {
			got = m.Client.DirectoryURL
		}
		if got != tc.wantURL {
			t.Errorf("%s: directory %q, want %q", tc.name, got, tc.wantURL)
		}
	}
}

func TestCertManagerRejectsConflictingOptions(t *testing.T) {
	for name, s := range map[string]*Server{
		"cert paths":            {Domain: "fetch.example.com", CertPath: "c.pem", KeyPath: "k.pem"},
		"staging and directory": {Domain: "fetch.example.com", ACMEStaging: true, ACMEDirectory: "https://ca.internal/dir"},
	} {
		s.ConfigPath = "c.json"
		if _, err := s.certManager(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTLSConfigAvoidsHTTP2(t *testing.T) {
	s := &Server{Domain: "fetch.example.com", ConfigPath: "c.json"}
	m, _ := s.certManager()
	protos := map[string]bool{}
	for _, p := range tlsConfigFor(m).NextProtos {
		protos[p] = true
	}
	// the server disables HTTP/2 for velox; advertising h2 would make
	// browsers negotiate a protocol the server then won't speak
	if protos["h2"] {
		t.Fatal("h2 advertised")
	}
	if !protos["http/1.1"] || !protos["acme-tls/1"] {
		t.Fatalf("missing protocols: %v", protos)
	}
}

func TestHTTPSRedirect(t *testing.T) {
	for _, tc := range []struct {
		port int
		host string
		url  string
		want string
	}{
		{443, "fetch.example.com", "/download/a%20b.mkv?x=1", "https://fetch.example.com/download/a%20b.mkv?x=1"},
		{3443, "fetch.example.com", "/", "https://fetch.example.com:3443/"},
		// never redirect to whatever Host the client sent
		{443, "evil.example.net", "/x", "https://fetch.example.com/x"},
	} {
		r := httptest.NewRequest("GET", tc.url, nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		httpsRedirect("fetch.example.com", tc.port).ServeHTTP(w, r)
		if w.Code != http.StatusMovedPermanently {
			t.Errorf("%s: status %d", tc.url, w.Code)
		}
		if got := w.Header().Get("Location"); got != tc.want {
			t.Errorf("%s: Location %q, want %q", tc.url, got, tc.want)
		}
	}
}
