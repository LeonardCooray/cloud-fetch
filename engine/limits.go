package engine

import (
	"sync"

	"golang.org/x/time/rate"
)

// maxRateKBps keeps a cap in bytes inside a 32-bit int, which is what
// rate.Limiter's burst is on the 32-bit release targets.
const maxRateKBps = 2000000

// minBurst is the smallest download bucket. anacrolix trims each peer read
// to the download burst, so it must stay well above a 16 KiB chunk.
const minBurst = 256 << 10

// maxPeerRequest is the largest peer request anacrolix accepts (set as its
// MaxAllocPeerRequestDataPerConn) and the smallest upload bucket. While
// uploads are unlimited anacrolix accepts requests up to that size, and it
// panics if one is ever bigger than the upload burst, which a cap switched
// on later would otherwise make it.
const maxPeerRequest = 1 << 20

// limiters are the client's download and upload rate limiters. They outlive
// any one client and are changed in place, because anacrolix keeps the
// pointers it was given and has no way to swap them on a running client.
// Both always exist: a nil download limiter could never be turned on later.
type limiters struct {
	// rate.Limiter is safe for concurrent use, but a limit and burst from
	// two Configure calls at once mustn't mix
	mu       sync.Mutex
	down, up *rate.Limiter
}

func newLimiters() *limiters {
	return &limiters{
		down: rate.NewLimiter(rate.Inf, minBurst),
		up:   rate.NewLimiter(rate.Inf, maxPeerRequest),
	}
}

// set applies c's caps; 0 is unlimited.
func (l *limiters) set(c Config) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.apply(l.down, c.MaxDownloadKBps, minBurst)
	l.apply(l.up, c.MaxUploadKBps, maxPeerRequest)
}

// apply gives rl a bucket of one second at the cap, and at least floor.
func (l *limiters) apply(rl *rate.Limiter, kbps, floor int) {
	limit, burst := rate.Inf, floor
	if kbps > 0 {
		b := kbps * 1000
		limit, burst = rate.Limit(b), max(b, floor)
	}
	rl.SetBurst(burst)
	rl.SetLimit(limit)
}
