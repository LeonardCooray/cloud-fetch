package server

import (
	"log"
	"net/http"
	"time"
)

// healthTimeout bounds how long /healthz waits for the engine; uptime
// monitors commonly give up after 5 to 10 seconds.
const healthTimeout = 2 * time.Second

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
	err := s.engine.Check(healthTimeout)
	s.noteHealth(err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unavailable\n"))
		return
	}
	w.Write([]byte("ok\n"))
}

// noteHealth logs when the result, or the reason for failing, changes.
func (s *Server) noteHealth(err error) {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	prev := s.healthErr
	s.healthErr = err
	switch {
	case err != nil && (prev == nil || prev.Error() != err.Error()):
		log.Printf("Health check failing: %s", err)
	case err == nil && prev != nil:
		log.Printf("Health check passing again")
	}
}
