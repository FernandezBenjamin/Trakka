package handlers

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Authentication rate limiting.
//
// Before this existed, POST /auth/login and POST /auth/register accepted an
// unlimited number of attempts: a self-hosted Trakka reachable from the
// internet could be password-sprayed at whatever rate the network allowed,
// with bcrypt's own cost as the only brake. There is no external dependency
// to lean on here (see CLAUDE.md's stdlib-first convention) and no shared
// store to coordinate through — a single process owns the one SQLite file —
// so a small in-process fixed-window counter is both sufficient and the
// right shape for this deployment model.
//
// Two independent buckets are checked for every attempt:
//
//   - per client IP, which blunts spraying many accounts from one host;
//   - per submitted email, which blunts guessing one account's password from
//     many hosts, and which is the bucket that actually does the work when
//     Trakka sits behind a reverse proxy that makes every request look like
//     it came from the proxy's own address.
//
// A successful login clears the email bucket, so an ordinary user who
// mistypes their password a few times and then gets it right is never left
// locked out; only sustained failure accumulates.
const (
	authRateWindow      = 15 * time.Minute
	authRateMaxPerIP    = 30
	authRateMaxPerEmail = 8
)

// rateLimiter is a fixed-window counter keyed by an arbitrary string. Entries
// are swept lazily on write, so an idle process never holds memory for keys
// nobody is using and there is no background goroutine to shut down.
type rateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]*rateEntry
	lastGC  time.Time
}

type rateEntry struct {
	count       int
	windowStart time.Time
}

func newRateLimiter(window time.Duration) *rateLimiter {
	return &rateLimiter{window: window, entries: make(map[string]*rateEntry), lastGC: time.Now()}
}

// allow records an attempt against key and reports whether it stays within
// max for the current window.
func (rl *rateLimiter) allow(key string, max int) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.gcLocked(now)

	e, ok := rl.entries[key]
	if !ok || now.Sub(e.windowStart) >= rl.window {
		rl.entries[key] = &rateEntry{count: 1, windowStart: now}
		return true
	}
	e.count++
	return e.count <= max
}

// reset forgets a key entirely — called for the email bucket after a
// successful authentication.
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, key)
}

// gcLocked drops expired windows. Called from allow (already holding the
// lock) at most once per window, which is often enough to keep the map
// proportional to recent activity rather than to all activity ever.
func (rl *rateLimiter) gcLocked(now time.Time) {
	if now.Sub(rl.lastGC) < rl.window {
		return
	}
	for k, e := range rl.entries {
		if now.Sub(e.windowStart) >= rl.window {
			delete(rl.entries, k)
		}
	}
	rl.lastGC = now
}

// clientIP is the address the connection actually came from. X-Forwarded-For
// is deliberately NOT consulted: Trakka has no configuration describing which
// proxies to trust, and an unvalidated forwarded header is attacker-controlled
// — honoring it would let a single host defeat the per-IP bucket entirely by
// varying one request header. Behind a reverse proxy this therefore collapses
// to one bucket for everyone, which is exactly why the per-email bucket above
// exists alongside it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
