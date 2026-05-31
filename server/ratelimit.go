package server

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter is a small per-key token-bucket limiter. It bounds how
// fast a single client (keyed by source IP) can hit the credential
// endpoints. The auth token has 192 bits of entropy, so this is not
// what stops a brute force — it stops connection/guess floods and the
// log/CPU noise they create.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64 // tokens refilled per second
	burst   float64 // max tokens (and initial fill)
	stop    chan struct{}
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter builds a limiter allowing `burst` requests immediately
// and refilling at `rate` per second thereafter, per key.
func NewRateLimiter(rate, burst float64) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    rate,
		burst:   burst,
		stop:    make(chan struct{}),
	}
	go rl.sweepLoop()
	return rl
}

// Allow consumes one token for key, returning false when the bucket is
// empty (i.e. the caller is over the limit).
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &tokenBucket{tokens: rl.burst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// Shutdown stops the background sweeper. Idempotent.
func (rl *RateLimiter) Shutdown() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	select {
	case <-rl.stop:
	default:
		close(rl.stop)
	}
}

// sweepLoop drops idle buckets so the map can't grow without bound under
// IP churn.
func (rl *RateLimiter) sweepLoop() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case <-t.C:
			cutoff := time.Now().Add(-5 * time.Minute)
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if b.last.Before(cutoff) {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// RateLimit wraps a handler, rejecting requests from a source IP that
// has exceeded the limiter with 429. The IP is derived via clientIP so
// it honors TrustProxyHeaders consistently with the rest of the app.
func RateLimit(cfg *Config, rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(clientIP(cfg, r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
