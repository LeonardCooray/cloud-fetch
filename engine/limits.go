package engine

import (
	"sync"

	"golang.org/x/time/rate"
)

// maxRateKBps keeps a cap in bytes inside a 32-bit int, which is what
// rate.Limiter's burst is on the 32-bit release targets.
const maxRateKBps = 2000000

// minBurst is the smallest bucket a limiter gets. anacrolix trims each peer
// read to the download burst and rejects peer requests bigger than the
// upload burst, so it must stay well above a 16 KiB chunk.
const minBurst = 256 << 10

// limiters are the client's download and upload rate limiters. They outlive
// any one client and are changed in place, because anacrolix keeps the
// pointers it was given and has no way to swap them on a running client.
// Both always exist: a nil download limiter could never be turned on later.
type limiters struct {
	mu       sync.Mutex
	down, up *rate.Limiter
}

func newLimiters() *limiters {
	return &limiters{
		down: rate.NewLimiter(rate.Inf, minBurst),
		up:   rate.NewLimiter(rate.Inf, minBurst),
	}
}

// set applies c's caps; 0 is unlimited.
func (l *limiters) set(c Config) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.apply(l.down, c.MaxDownloadKBps)
	l.apply(l.up, c.MaxUploadKBps)
}

func (l *limiters) apply(rl *rate.Limiter, kbps int) {
	limit, burst := rate.Inf, minBurst
	if kbps > 0 {
		b := kbps * 1000
		limit, burst = rate.Limit(b), max(b, minBurst)
	}
	if rl.Limit() == limit && rl.Burst() == burst {
		return
	}
	rl.SetBurst(burst)
	rl.SetLimit(limit)
}
