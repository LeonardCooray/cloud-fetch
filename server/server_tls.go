package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const letsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"

// certManager issues and renews certificates for --domain. Renewal happens in
// the background while the process runs; nothing needs scheduling.
func (s *Server) certManager() (*autocert.Manager, error) {
	if s.CertPath != "" || s.KeyPath != "" {
		return nil, errors.New("use either --domain or --cert-path/--key-path, not both")
	}
	if s.ACMEStaging && s.ACMEDirectory != "" {
		return nil, errors.New("use either --acme-staging or --acme-directory, not both")
	}
	cache := s.CertCache
	if cache == "" {
		// must survive restarts, or every restart spends Let's Encrypt rate limit
		cache = filepath.Join(filepath.Dir(s.ConfigPath), "certs")
	}
	allow := autocert.HostWhitelist(s.Domain)
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		// HTTPHandler passes r.Host verbatim, port included
		HostPolicy: func(ctx context.Context, host string) error {
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			return allow(ctx, host)
		},
		Cache: autocert.DirCache(cache),
	}
	switch {
	case s.ACMEStaging:
		m.Client = &acme.Client{DirectoryURL: letsEncryptStaging}
	case s.ACMEDirectory != "":
		m.Client = &acme.Client{DirectoryURL: s.ACMEDirectory}
	}
	return m, nil
}

// tlsConfigFor drops h2 from autocert's defaults: the server turns HTTP/2 off
// for velox, and a client that negotiated h2 would then get HTTP/1 bytes.
func tlsConfigFor(m *autocert.Manager) *tls.Config {
	c := m.TLSConfig()
	var protos []string
	for _, p := range c.NextProtos {
		if p != "h2" {
			protos = append(protos, p)
		}
	}
	c.NextProtos = protos
	return c
}

// httpsRedirect targets the configured domain, never the request's Host
// header, so the plain HTTP port can't be used as an open redirect.
func httpsRedirect(domain string, port int) http.Handler {
	base := "https://" + domain
	if port != 443 {
		base = fmt.Sprintf("https://%s:%d", domain, port)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}
