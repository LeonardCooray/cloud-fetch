package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jpillora/scraper/scraper"
)

func newSearchServer(t *testing.T, url string) *Server {
	t.Helper()
	s := &Server{SearchConfigURL: url, scraper: &scraper.Handler{}}
	if err := s.scraper.LoadConfig(builtinSearchConfig()); err != nil {
		t.Fatalf("built-in search config doesn't load: %v", err)
	}
	return s
}

// the built-in list is the repo's search-config.json, so a bad edit to that
// file must fail here rather than at startup on the VPS
func TestBuiltInSearchConfigHasRepoProviders(t *testing.T) {
	s := newSearchServer(t, "")
	for _, id := range []string{"nyaa", "lt", "lt/item", "abb", "abb/item"} {
		if _, ok := s.scraper.Config[id]; !ok {
			t.Errorf("built-in providers missing %q", id)
		}
	}
	if _, ok := s.scraper.Config["rbg"]; ok {
		t.Error("built-in providers still carry the old hardcoded list")
	}
}

func TestSearchConfigFetchedFromConfiguredURL(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"mine":{"name":"Mine","url":"https://example.test/?q={{query}}","list":"tr","result":{"name":"td"}}}`))
	}))
	defer remote.Close()
	s := newSearchServer(t, remote.URL)

	if err := s.fetchSearchConfig(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.scraper.Config["mine"]; !ok {
		t.Fatal("providers from the configured URL were not loaded")
	}
}

func TestBrokenRemoteSearchConfigKeepsCurrentProviders(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// valid JSON on an error page would otherwise replace every provider with none
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("{}"))
	}))
	defer remote.Close()
	s := newSearchServer(t, remote.URL)

	if err := s.fetchSearchConfig(); err == nil {
		t.Fatal("a 404 page was accepted as a provider list")
	}
	if _, ok := s.scraper.Config["nyaa"]; !ok {
		t.Fatal("a failed fetch dropped the built-in providers")
	}
}

func TestEmptySearchConfigURLDisablesRemoteFetch(t *testing.T) {
	s := newSearchServer(t, "")
	done := make(chan struct{})
	go func() { s.fetchSearchConfigLoop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch loop kept running with no URL configured")
	}
}
