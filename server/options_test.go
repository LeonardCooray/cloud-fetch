package server

import (
	"testing"

	"github.com/jpillora/opts"
)

// parseOptions parses args the way main does, over the same starting defaults.
func parseOptions(t *testing.T, args ...string) *Server {
	t.Helper()
	s := &Server{Title: "Cloud Fetch", Port: 3000}
	if _, err := opts.New(s).ParseArgsError(append([]string{"cloud-fetch"}, args...)); err != nil {
		t.Fatal(err)
	}
	return s
}

// opts v1.2 reads env names only from opts:"env=NAME"; a separate env:"NAME" tag is
// silently ignored, which left AUTH unset and the server open on localhost only.
func TestOptionsReadEnvironment(t *testing.T) {
	t.Setenv("TITLE", "From Env")
	t.Setenv("PORT", "8123")
	t.Setenv("AUTH", "admin:secret")
	t.Setenv("DOMAIN", "fetch.example.com")

	s := parseOptions(t)
	if s.Title != "From Env" || s.Port != 8123 || s.Auth != "admin:secret" || s.Domain != "fetch.example.com" {
		t.Fatalf("env not applied: title=%q port=%d auth=%q domain=%q", s.Title, s.Port, s.Auth, s.Domain)
	}
}

func TestOptionsFlagsOverrideEnvironment(t *testing.T) {
	t.Setenv("AUTH", "env:env")
	t.Setenv("PORT", "8123")

	s := parseOptions(t, "--auth", "flag:flag", "--port", "9000")
	if s.Auth != "flag:flag" || s.Port != 9000 {
		t.Fatalf("flags should win over env: auth=%q port=%d", s.Auth, s.Port)
	}
}

func TestOptionsDefaultsWithoutEnvironment(t *testing.T) {
	for _, k := range []string{"TITLE", "PORT", "AUTH", "DOMAIN"} {
		t.Setenv(k, "")
	}
	s := parseOptions(t)
	if s.Title != "Cloud Fetch" || s.Port != 3000 || s.Auth != "" || s.Domain != "" {
		t.Fatalf("defaults changed: title=%q port=%d auth=%q domain=%q", s.Title, s.Port, s.Auth, s.Domain)
	}
}
