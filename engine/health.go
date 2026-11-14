package engine

import (
	"errors"
	"time"
)

var errCheckTimeout = errors.New("engine didn't answer in time")

// checkReuse is how long a finished check's result is handed out again. The
// health check needs no login, so this caps how often anyone can make the
// engine take its locks; the 1s torrent poller already takes them as often.
const checkReuse = time.Second

type healthProbe struct {
	done chan struct{}
	err  error     // set before done is closed
	at   time.Time // when it finished, set before done is closed
}

// Check reports whether the torrent client is running and answering: it
// takes the engine lock, then the client's own lock, which is where anacrolix
// lock leaks like #1119 (fixed in v1.62.0) leave things stuck. It gives up
// after timeout, but the probe keeps waiting, and later
// calls share it instead of starting another, so a wedged engine costs one
// stuck goroutine however often it's checked. A result up to checkReuse old
// is reused.
func (e *Engine) Check(timeout time.Duration) error {
	e.probeMu.Lock()
	p := e.probe
	if p == nil || p.stale() {
		p = &healthProbe{done: make(chan struct{})}
		e.probe = p
		go p.run(e)
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

// stale reports whether p finished more than checkReuse ago; one still
// running is shared, never replaced.
func (p *healthProbe) stale() bool {
	select {
	case <-p.done:
		return time.Since(p.at) >= checkReuse
	default:
		return false
	}
}

// forgetCheck drops a finished check's result, so the next one sees a client
// that just closed or started. A running one stays to be shared; it's waiting
// on e.mut and will see the change itself.
func (e *Engine) forgetCheck() {
	e.probeMu.Lock()
	defer e.probeMu.Unlock()
	if e.probe != nil {
		select {
		case <-e.probe.done:
			e.probe = nil
		default:
		}
	}
}

func (p *healthProbe) run(e *Engine) {
	p.err = e.probeClient()
	p.at = time.Now()
	close(p.done)
}

func (e *Engine) probeClient() error {
	// e.mut stays held so Configure can't close this client mid-check;
	// GetTorrents nests the two locks the same way every second
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return errNotRunning
	}
	e.client.Torrents()
	return nil
}
