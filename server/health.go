package server

import (
	"log"
	"net/http"
	"time"
)

const (
	// bounds how long /healthz waits for the engine; uptime monitors
	// commonly give up after 5 to 10 seconds
	healthTimeout = 2 * time.Second
	// /healthz needs no login, so this caps how often anyone can make the
	// engine take its locks; the 1s torrent poller already takes them as often
	healthCacheFor = time.Second
)

// serveHealth answers /healthz for uptime monitors and cloud-fetch-update.
// It runs before auth and the login throttle, so it says only "ok" or
// "unavailable": no version, paths, torrents or disk figures. Why it's
// unavailable goes to the log, once per change.
func (s *Server) serveHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	err := s.health()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unavailable\n"))
		return
	}
	w.Write([]byte("ok\n"))
}

func (s *Server) health() error {
	s.healthMu.Lock()
	if !s.healthAt.IsZero() && time.Since(s.healthAt) < healthCacheFor {
		err := s.healthErr
		s.healthMu.Unlock()
		return err
	}
	s.healthMu.Unlock()
	// concurrent callers share the engine's probe
	err := s.engine.Check(healthTimeout)
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	wasBad := s.healthErr != nil
	s.healthAt, s.healthErr = time.Now(), err
	if err != nil && !wasBad {
		log.Printf("Health check failing: %s", err)
	} else if err == nil && wasBad {
		log.Printf("Health check passing again")
	}
	return err
}
