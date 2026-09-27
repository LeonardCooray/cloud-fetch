package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jpillora/backoff"
)

// DefaultSearchConfigURL serves server/search-config.json from this repo, so
// providers are updated by pushing a commit rather than releasing a binary.
const DefaultSearchConfigURL = "https://raw.githubusercontent.com/LeonardCooray/cloud-fetch/master/server/search-config.json"

// builtinSearchConfig returns a fresh copy: the scraper appears to write into
// the slice it parses, which corrupted the shared original on a second load.
func builtinSearchConfig() []byte {
	return append([]byte(nil), defaultSearchConfig...)
}

func (s *Server) fetchSearchConfigLoop() {
	if s.SearchConfigURL == "" {
		return
	}
	b := backoff.Backoff{Max: 30 * time.Minute}
	for {
		if err := s.fetchSearchConfig(); err != nil {
			//ignore error
			time.Sleep(b.Duration())
		} else {
			//no errror - check again in half hour
			time.Sleep(30 * time.Minute)
			b.Reset()
		}
	}
}

var fetches = 0
var currentConfig, _ = normalize(builtinSearchConfig())

func (s *Server) fetchSearchConfig() error {
	client := &http.Client{Timeout: remoteFetchTimeout}
	resp, err := client.Get(s.SearchConfigURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search config: %s", resp.Status)
	}
	newConfig, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentBytes))
	if err != nil {
		return err
	}
	newConfig, err = normalize(newConfig)
	if err != nil {
		return err
	}
	fetches++
	if bytes.Equal(currentConfig, newConfig) {
		return nil //skip
	}
	if err := s.scraper.LoadConfig(newConfig); err != nil {
		return err
	}
	s.state.SearchProviders = s.scraper.Config
	s.state.Push()
	currentConfig = newConfig
	log.Printf("Loaded new search providers")
	return nil
}

func normalize(input []byte) ([]byte, error) {
	output := bytes.Buffer{}
	if err := json.Indent(&output, input, "", "  "); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// see github.com/jpillora/scraper for config specification
// cloud fetch uses "<id>/item" handlers
//
//go:embed search-config.json
var defaultSearchConfig []byte
