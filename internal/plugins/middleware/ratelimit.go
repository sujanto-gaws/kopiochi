package middleware

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	corenet "github.com/sujanto-gaws/kopiochi/internal/middleware"
)

// RateLimiterPlugin implements Plugin for request rate limiting using a
// token bucket per key, refilled continuously (not on a fixed-window
// reset). See docs/architectures/04-security/rate-limiting.md for the
// defects this replaces: the previous implementation held its mutex across
// the entire downstream handler (serializing the whole server to one
// concurrent request), never evicted idle entries, and keyed on the raw,
// attacker-controlled X-Forwarded-For header.
//
// allow() is the only method that touches shared state, and it holds the
// lock only for the bucket lookup/refill arithmetic -- never across
// next.ServeHTTP.
type RateLimiterPlugin struct {
	initialized bool

	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens (requests) refilled per second
	burst   float64 // bucket capacity / instantaneous allowance
	ttl     time.Duration
	maxKeys int

	// now is the injected clock. Defaults to time.Now in Initialize;
	// overridden directly by white-box tests for deterministic refill and
	// eviction timing without ever sleeping across a window boundary.
	now func() time.Time

	stopSweep chan struct{}
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxKeysRetryAfter is the Retry-After hint returned when a brand-new key
// is rejected because the tracked-key table is full. There is no
// per-bucket state to compute a precise value from -- the key was never
// admitted -- so this is a conservative fixed hint rather than a fabricated
// per-key number.
const maxKeysRetryAfter = time.Second

// Name returns the plugin name.
func (p *RateLimiterPlugin) Name() string {
	return "ratelimit"
}

// Initialize sets up the rate limiter with configuration.
//
// Two config shapes are accepted:
//
//   - "rate" (requests per minute) and "burst" (bucket capacity): the
//     token-bucket parameters directly.
//   - Legacy "requests" + "window" (a fixed-window count and duration):
//     translated into an equivalent token bucket (burst = requests,
//     rate = requests / window) for backward compatibility with existing
//     deployments' config.
//
// "ttl" (idle bucket eviction, default 10m) and "max_keys" (hard cap on
// tracked keys, default 100000) round out the tunables.
func (p *RateLimiterPlugin) Initialize(cfg map[string]interface{}) error {
	requests := 100.0
	if v, ok := cfg["requests"].(float64); ok {
		requests = v
	}

	window := time.Minute
	if s, ok := cfg["window"].(string); ok && s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("ratelimit: invalid window duration: %w", err)
		}
		window = d
	}

	burst := requests
	if v, ok := cfg["burst"].(float64); ok {
		burst = v
	}
	if burst <= 0 {
		return fmt.Errorf("ratelimit: burst must be positive, got %v", burst)
	}

	rate := requests / window.Seconds()
	if v, ok := cfg["rate"].(float64); ok {
		rate = v / 60.0 // configured as requests per minute
	}
	if rate <= 0 {
		return fmt.Errorf("ratelimit: rate must be positive, got %v", rate)
	}

	ttl := 10 * time.Minute
	if s, ok := cfg["ttl"].(string); ok && s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("ratelimit: invalid ttl duration: %w", err)
		}
		ttl = d
	}
	if ttl <= 0 {
		return fmt.Errorf("ratelimit: ttl must be positive, got %v", ttl)
	}

	maxKeys := 100000
	if v, ok := cfg["max_keys"].(float64); ok {
		maxKeys = int(v)
	}
	if maxKeys <= 0 {
		return fmt.Errorf("ratelimit: max_keys must be positive, got %d", maxKeys)
	}

	p.mu.Lock()
	p.rate = rate
	p.burst = burst
	p.ttl = ttl
	p.maxKeys = maxKeys
	p.buckets = make(map[string]*bucket)
	if p.now == nil {
		p.now = time.Now
	}
	p.stopSweep = make(chan struct{})
	p.initialized = true
	p.mu.Unlock()

	go p.sweepLoop()

	return nil
}

// setClock overrides the injected clock under lock. Unexported: it exists
// purely so white-box tests can install a fake, deterministic clock without
// racing the background eviction sweep started by Initialize.
func (p *RateLimiterPlugin) setClock(now func() time.Time) {
	p.mu.Lock()
	p.now = now
	p.mu.Unlock()
}

// Close performs cleanup: stops the background eviction sweep and releases
// tracked state.
func (p *RateLimiterPlugin) Close() error {
	p.mu.Lock()
	stopSweep := p.stopSweep
	p.stopSweep = nil
	p.buckets = nil
	p.initialized = false
	p.mu.Unlock()

	if stopSweep != nil {
		close(stopSweep)
	}
	return nil
}

// sweepLoop periodically evicts idle buckets until Close stops it.
func (p *RateLimiterPlugin) sweepLoop() {
	p.mu.Lock()
	stopSweep := p.stopSweep
	interval := p.ttl
	p.mu.Unlock()
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopSweep:
			return
		case <-ticker.C:
			p.evictExpired()
		}
	}
}

// evictExpired removes buckets that have been idle for at least the
// configured TTL and would already be fully refilled by now -- there is no
// in-flight state to lose by dropping them; the next request simply starts
// a fresh bucket at full burst, indistinguishable from a key seen for the
// first time. This bounds the memory the previous implementation grew
// without limit.
func (p *RateLimiterPlugin) evictExpired() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.buckets == nil {
		return
	}

	now := p.now()
	cutoff := now.Add(-p.ttl)

	for key, b := range p.buckets {
		if !b.last.Before(cutoff) {
			continue // still within the idle window
		}
		projected := math.Min(p.burst, b.tokens+now.Sub(b.last).Seconds()*p.rate)
		if projected >= p.burst {
			delete(p.buckets, key)
		}
	}
}

// allow decides whether the request identified by key may proceed. It is
// O(1) and returns immediately; the lock is held only across this
// function's own bucket lookup/refill/decrement, never across
// next.ServeHTTP.
func (p *RateLimiterPlugin) allow(key string) (ok bool, remaining int, retryAfter time.Duration) {
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	b, exists := p.buckets[key]
	if !exists {
		if len(p.buckets) >= p.maxKeys {
			// The tracked-key table is full. Reject the new key rather
			// than evicting an existing one to make room: evict-oldest
			// would let an attacker who floods fresh keys push a
			// legitimate, actively-throttled client's bucket out and hand
			// it a full budget for free. Existing keys are served
			// normally and unaffected; only brand-new keys are turned
			// away, and only while the table is at capacity -- the TTL
			// sweep continuously reclaims idle slots, so this self-heals.
			return false, 0, maxKeysRetryAfter
		}
		b = &bucket{tokens: p.burst, last: now}
		p.buckets[key] = b
	}

	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(p.burst, b.tokens+elapsed*p.rate)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, int(b.tokens), 0
	}

	retryAfter = time.Duration((1-b.tokens)/p.rate*float64(time.Second)) + time.Millisecond
	return false, 0, retryAfter
}

// Middleware returns the rate limiting middleware.
func (p *RateLimiterPlugin) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !p.initialized {
				next.ServeHTTP(w, r)
				return
			}

			key := clientKey(r)

			ok, remaining, retryAfter := p.allow(key)

			// Headers are set on both the success and rejection paths so a
			// well-behaved client always has enough information to
			// self-throttle, not just when it's already too late.
			w.Header().Set("RateLimit-Limit", strconv.Itoa(int(p.burst)))
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))

			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			next.ServeHTTP(w, r) // outside the critical section: lock was released in allow()
		})
	}
}

// clientKey resolves the rate-limit key for a request. It reads the IP
// resolved upstream by internal/middleware.RealIP (trusted-proxy aware) and
// never reads X-Forwarded-For itself -- that header is attacker-controlled
// and reading it directly is exactly the bypass this plugin used to have.
// If RealIP hasn't run (e.g. the plugin exercised directly, outside the
// full middleware chain), it falls back to the raw peer address rather than
// any header.
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

// Provider returns the plugin instance.
func (p *RateLimiterPlugin) Provider() interface{} {
	return p
}

// NewRateLimiterPlugin creates a new rate limiter plugin instance.
func NewRateLimiterPlugin() *RateLimiterPlugin {
	return &RateLimiterPlugin{}
}
