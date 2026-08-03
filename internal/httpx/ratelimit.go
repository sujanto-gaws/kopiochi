package httpx

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sujanto-gaws/kopiochi/internal/config"
	corenet "github.com/sujanto-gaws/kopiochi/internal/middleware"
)

// RateLimiter is a per-key token bucket, refilled continuously (not on a
// fixed-window reset). See docs/architectures/04-security/rate-limiting.md
// for the defects it replaces: the original implementation held its mutex
// across the entire downstream handler (serializing the whole server to one
// concurrent request), never evicted idle entries, and keyed on the raw,
// attacker-controlled X-Forwarded-For header.
//
// It is constructed once, at boot, from typed configuration. The plugin
// version could be re-Initialize()d at any time, which both leaked the
// previous instance's sweep goroutine and made every field a moving target
// for in-flight requests; here rate, burst, ttl and maxKeys are written once
// by NewRateLimiter and never again, so only the bucket table needs the lock.
//
// allow is the only method that touches shared state, and it holds the lock
// only for the bucket lookup/refill arithmetic -- never across
// next.ServeHTTP.
type RateLimiter struct {
	rate    float64 // tokens (requests) refilled per second
	burst   float64 // bucket capacity / instantaneous allowance
	ttl     time.Duration
	maxKeys int

	mu      sync.Mutex
	buckets map[string]*bucket
	// now is the injected clock, read under mu. Defaults to time.Now;
	// overridden by white-box tests for deterministic refill and eviction
	// timing without ever sleeping across a window boundary.
	now func() time.Time

	stopOnce sync.Once
	stop     chan struct{}
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxKeysRetryAfter is the Retry-After hint returned when a brand-new key is
// rejected because the tracked-key table is full. There is no per-bucket
// state to compute a precise value from -- the key was never admitted -- so
// this is a conservative fixed hint rather than a fabricated per-key number.
const maxKeysRetryAfter = time.Second

// NewRateLimiter builds a rate limiter from typed configuration and starts
// its background eviction sweep. The caller owns the returned limiter and
// must Close it; Close stops the sweep goroutine.
//
// Every tunable must be positive. There are deliberately no fallback
// defaults here: defaults belong in exactly one place (internal/config's
// SetDefault calls), and silently substituting one for a bad value is the
// plugin-config defect this replaces -- a YAML type error used to produce a
// working-looking limiter with a limit nobody configured.
func NewRateLimiter(cfg config.RateLimit) (*RateLimiter, error) {
	if cfg.Rate <= 0 {
		return nil, fmt.Errorf("ratelimit: rate must be positive, got %v", cfg.Rate)
	}
	if cfg.Burst <= 0 {
		return nil, fmt.Errorf("ratelimit: burst must be positive, got %v", cfg.Burst)
	}
	if cfg.TTL <= 0 {
		return nil, fmt.Errorf("ratelimit: ttl must be positive, got %v", cfg.TTL)
	}
	if cfg.MaxKeys <= 0 {
		return nil, fmt.Errorf("ratelimit: max_keys must be positive, got %d", cfg.MaxKeys)
	}

	l := &RateLimiter{
		rate:    cfg.Rate / 60.0, // configured per minute, applied per second
		burst:   cfg.Burst,
		ttl:     cfg.TTL,
		maxKeys: cfg.MaxKeys,
		buckets: make(map[string]*bucket),
		now:     time.Now,
		stop:    make(chan struct{}),
	}

	go l.sweepLoop()

	return l, nil
}

// Close stops the background eviction sweep and releases tracked state. It is
// idempotent: the composition root may call it on a shutdown path that also
// runs after a partially-failed boot.
func (l *RateLimiter) Close() error {
	l.stopOnce.Do(func() { close(l.stop) })

	l.mu.Lock()
	// Reset rather than nil the map: allow writes to it, and a nil map write
	// panics. A request racing shutdown gets a fresh bucket and is allowed
	// through, which is the right failure mode for a server that is already
	// on its way out.
	l.buckets = make(map[string]*bucket)
	l.mu.Unlock()

	return nil
}

// setClock overrides the injected clock under lock. Unexported: it exists
// purely so white-box tests can install a fake, deterministic clock without
// racing the background eviction sweep.
func (l *RateLimiter) setClock(now func() time.Time) {
	l.mu.Lock()
	l.now = now
	l.mu.Unlock()
}

// sweepLoop periodically evicts idle buckets until Close stops it.
func (l *RateLimiter) sweepLoop() {
	ticker := time.NewTicker(l.ttl)
	defer ticker.Stop()

	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.evictExpired()
		}
	}
}

// evictExpired removes buckets that have been idle for at least the
// configured TTL and would already be fully refilled by now -- there is no
// in-flight state to lose by dropping them; the next request simply starts a
// fresh bucket at full burst, indistinguishable from a key seen for the first
// time. This bounds the memory the original implementation grew without
// limit.
func (l *RateLimiter) evictExpired() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.ttl)

	for key, b := range l.buckets {
		if !b.last.Before(cutoff) {
			continue // still within the idle window
		}
		projected := math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
		if projected >= l.burst {
			delete(l.buckets, key)
		}
	}
}

// allow decides whether the request identified by key may proceed. It is
// O(1) and returns immediately; the lock is held only across this function's
// own bucket lookup/refill/decrement, never across next.ServeHTTP.
func (l *RateLimiter) allow(key string) (ok bool, remaining int, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Read the clock under the lock: setClock writes l.now under the same
	// mutex, so reading it unlocked is a data race on the field itself.
	now := l.now()

	b, exists := l.buckets[key]
	if !exists {
		if len(l.buckets) >= l.maxKeys {
			// The tracked-key table is full. Reject the new key rather than
			// evicting an existing one to make room: evict-oldest would let
			// an attacker who floods fresh keys push a legitimate,
			// actively-throttled client's bucket out and hand it a full
			// budget for free. Existing keys are served normally and
			// unaffected; only brand-new keys are turned away, and only
			// while the table is at capacity -- the TTL sweep continuously
			// reclaims idle slots, so this self-heals.
			return false, 0, maxKeysRetryAfter
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}

	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, int(b.tokens), 0
	}

	retryAfter = time.Duration((1-b.tokens)/l.rate*float64(time.Second)) + time.Millisecond
	return false, 0, retryAfter
}

// Middleware returns the rate-limiting middleware for this limiter.
func (l *RateLimiter) Middleware() func(http.Handler) http.Handler {
	limit := strconv.Itoa(int(l.burst)) // immutable after construction

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, remaining, retryAfter := l.allow(clientKey(r))

			// Headers are set on both the success and rejection paths so a
			// well-behaved client always has enough information to
			// self-throttle, not just when it's already too late.
			w.Header().Set("RateLimit-Limit", limit)
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))

			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r) // outside the critical section: allow() released the lock
		})
	}
}

// clientKey resolves the rate-limit key for a request. It reads the IP
// resolved upstream by internal/middleware.RealIP (trusted-proxy aware) and
// never reads X-Forwarded-For itself -- that header is attacker-controlled
// and reading it directly is exactly the bypass this limiter used to have.
// If RealIP hasn't run (e.g. the limiter exercised directly, outside the full
// middleware chain), it falls back to the raw peer address rather than any
// header.
func clientKey(r *http.Request) string {
	if ip := corenet.ClientIP(r.Context()); ip != "" {
		return ip
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host
}
