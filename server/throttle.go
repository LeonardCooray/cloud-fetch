package server

import (
	"context"
	"crypto/sha256"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	loginMaxFails = 5
	loginWindow   = 15 * time.Minute
	loginBlock    = 15 * time.Minute
	// the map is swept of stale entries once it grows past this
	loginSweepAt = 1024
	// how many distinct working Authorization headers to remember
	loginKnownMax = 8
	// cookieauth's default cookie name
	authCookie = "cookieauth"
)

// loginThrottle blocks a client IP that fails to log in loginMaxFails times
// within loginWindow, answering 429 to everything from it for loginBlock.
// cookieauth itself has no limit, so without this a password can be guessed
// as fast as the server answers.
type loginThrottle struct {
	mu    sync.Mutex
	ips   map[string]*loginFails
	known map[[32]byte]bool // Authorization headers that have worked
	now   func() time.Time
}

type loginFails struct {
	times   []time.Time // failures inside the window, oldest first
	pending int         // unproven Basic logins still being checked
	until   time.Time   // blocked until
}

type authedKey struct{}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{ips: map[string]*loginFails{}, known: map[[32]byte]bool{}, now: time.Now}
}

// wrap puts the throttle around auth, an authenticating handler that calls
// inner (and only inner) once a request's credentials check out.
func (lt *loginThrottle) wrap(auth func(inner http.Handler) http.Handler, inner http.Handler) http.Handler {
	// Marking success from inside, instead of watching for a 401 on the way
	// out, leaves the ResponseWriter unwrapped: velox and downloads need its
	// Flusher and Hijacker.
	mark := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, _ := r.Context().Value(authedKey{}).(*bool); ok != nil {
			*ok = true
		}
		inner.ServeHTTP(w, r)
	})
	h := auth(mark)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		basic := r.Header.Get("Authorization")
		// A Basic header that hasn't worked before is a possible guess, and
		// counts against the limit while it's checked, so a burst of
		// simultaneous guesses can't all slip in before the first one fails.
		// Cookies aren't: they hold a scrypt hash, not a guessable password.
		guess := basic != "" && !lt.isKnown(basic)
		if wait := lt.begin(ip, guess); wait > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds()+0.5)))
			http.Error(w, "Too many failed logins, try again later", http.StatusTooManyRequests)
			return
		}
		authed := false
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authedKey{}, &authed)))
		// a request without credentials is the browser asking for the login
		// prompt, not a guess
		lt.end(ip, guess, authed, basic, hasCredentials(r))
	})
}

// begin returns how long ip must wait, or zero to go ahead.
func (lt *loginThrottle) begin(ip string, guess bool) time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	now := lt.now()
	f := lt.ips[ip]
	if f != nil && now.Before(f.until) {
		return f.until.Sub(now)
	}
	if !guess {
		return 0
	}
	if f == nil {
		if len(lt.ips) >= loginSweepAt {
			lt.sweep(now)
		}
		f = &loginFails{}
		lt.ips[ip] = f
	}
	f.times = trimBefore(f.times, now.Add(-loginWindow))
	if len(f.times)+f.pending >= loginMaxFails {
		// too many guesses at once; working ones become known and pass
		return time.Second
	}
	f.pending++
	return 0
}

func (lt *loginThrottle) end(ip string, guess, authed bool, basic string, creds bool) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	now := lt.now()
	f := lt.ips[ip]
	if guess && f != nil {
		f.pending--
	}
	if authed {
		if basic != "" {
			if len(lt.known) >= loginKnownMax {
				clear(lt.known)
			}
			lt.known[sha256.Sum256([]byte(basic))] = true
		}
		if f != nil {
			f.times = nil
			if f.pending == 0 && !now.Before(f.until) {
				delete(lt.ips, ip)
			}
		}
		return
	}
	if !creds {
		return
	}
	if f == nil {
		if len(lt.ips) >= loginSweepAt {
			lt.sweep(now)
		}
		f = &loginFails{}
		lt.ips[ip] = f
	}
	f.times = append(trimBefore(f.times, now.Add(-loginWindow)), now)
	if len(f.times) >= loginMaxFails && !now.Before(f.until) {
		f.times = nil
		f.until = now.Add(loginBlock)
		log.Printf("Blocked %s for %s after %d failed logins", ip, loginBlock, loginMaxFails)
	}
}

func (lt *loginThrottle) isKnown(basic string) bool {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.known[sha256.Sum256([]byte(basic))]
}

func (lt *loginThrottle) sweep(now time.Time) {
	for ip, f := range lt.ips {
		f.times = trimBefore(f.times, now.Add(-loginWindow))
		if len(f.times) == 0 && f.pending == 0 && !now.Before(f.until) {
			delete(lt.ips, ip)
		}
	}
}

func trimBefore(times []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(times) && times[i].Before(cutoff) {
		i++
	}
	return times[i:]
}

func hasCredentials(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return true
	}
	_, err := r.Cookie(authCookie)
	return err == nil
}

// clientIP is the connection's address, or its /64 for IPv6, since one host
// usually holds a whole /64 and could rotate through it. X-Forwarded-For is
// ignored on purpose: with no proxy in front, a guesser would set it to dodge
// the block.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		return host
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
