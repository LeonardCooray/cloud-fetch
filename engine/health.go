package engine

import (
	"errors"
	"time"
)

var errCheckTimeout = errors.New("engine didn't answer in time")

type healthProbe struct {
	done chan struct{}
	err  error // set before done is closed
}

// Check reports whether the torrent client is running and answering: it
// takes the engine lock, then the client's own lock (the one anacrolix #1119
// leaks). It gives up after timeout, but the probe keeps waiting, and later
// calls share it instead of starting another, so a wedged engine costs one
// stuck goroutine however often it's checked.
func (e *Engine) Check(timeout time.Duration) error {
	e.probeMu.Lock()
	p := e.probe
	if p == nil {
		p = &healthProbe{done: make(chan struct{})}
		e.probe = p
		go e.runProbe(p)
	}
	e.probeMu.Unlock()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-p.done:
		return p.err
	case <-t.C:
		return errCheckTimeout
	}
}

func (e *Engine) runProbe(p *healthProbe) {
	p.err = e.probeClient()
	e.probeMu.Lock()
	e.probe = nil
	e.probeMu.Unlock()
	close(p.done)
}

func (e *Engine) probeClient() error {
	e.mut.Lock()
	client := e.client
	e.mut.Unlock()
	if client == nil {
		return errNotRunning
	}
	// a closed client still answers, but closing only happens under e.mut
	// and leaves e.client nil, which the next check sees
	client.Torrents()
	return nil
}
