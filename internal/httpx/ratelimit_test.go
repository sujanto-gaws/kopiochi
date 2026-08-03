package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sujanto-gaws/kopiochi/internal/config"
)

// newTestLimiter builds a RateLimiter with a fake, test-controlled clock.
// Callers advance `clock` directly (mutating the closed-over time.Time) to
// simulate the passage of time deterministically -- never time.Sleep, which
// can't reliably cross a refill boundary in a unit test.
func newTestLimiter(t *testing.T, cfg config.RateLimit) (l *RateLimiter, clock *time.Time) {
	t.Helper()

	l, err := NewRateLimiter(cfg)
	if err != nil {
		t.Fatalf("build rate limiter: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	now := time.Unix(1_700_000_000, 0)
	l.setClock(func() time.Time { return now })

	return l, &now
}

// TestRateLimitAllowsConcurrentRequests is task 1.1(c) from the remediation
// plan (docs/architectures/06-quality/testing-strategy.md) — the
// highest-value test in this batch. Two requests dispatched concurrently must
// both be inside the wrapped handler at the same time; a rate limiter should
// gate *how many* requests get through, not serialize them one at a time
// regardless of limit.
//
// This was a documented failure against the pre-Phase-2.1 implementation,
// which held the single mutex for the entire duration of next.ServeHTTP, so a
// second concurrent request could not even start running the handler until
// the first request's ServeHTTP call had completely returned. The token
// bucket releases its lock (inside allow()) before next.ServeHTTP is ever
// called. Phase 3.5 moved the limiter out of the plugin framework into this
// package without touching that property; the test moved with it.
func TestRateLimitAllowsConcurrentRequests(t *testing.T) {
	l, _ := newTestLimiter(t, config.RateLimit{
		Enabled: true, Rate: 100, Burst: 100, TTL: 10 * time.Minute, MaxKeys: 1000,
	})

	release := make(chan struct{})
	entered := make(chan struct{}, 2)

	h := l.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
	}))

	req := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/", nil)
	}

	go h.ServeHTTP(httptest.NewRecorder(), req())
	go h.ServeHTTP(httptest.NewRecorder(), req())

	// Both handlers must be inside simultaneously. A limiter holding its
	// lock across ServeHTTP deadlocks here.
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("handler blocked: rate limiter holds its lock across ServeHTTP")
		}
	}
	close(release)
}

// TestAllow_RefillsTokensOverInjectedClock exercises the token bucket refill
// maths using an injected clock: burst caps the initial allowance, exhausting
// it rejects further requests, and tokens are only replenished once enough
// simulated time has passed -- proportional to the configured rate, never in
// a single fixed-window jump.
func TestAllow_RefillsTokensOverInjectedClock(t *testing.T) {
	l, clock := newTestLimiter(t, config.RateLimit{
		Enabled: true,
		Burst:   2,
		Rate:    60, // 60 requests/min == 1 token/sec
		TTL:     10 * time.Minute,
		MaxKeys: 1000,
	})

	if ok, remaining, _ := l.allow("client"); !ok || remaining != 1 {
		t.Fatalf("1st request: ok=%v remaining=%d, want ok=true remaining=1", ok, remaining)
	}
	if ok, remaining, _ := l.allow("client"); !ok || remaining != 0 {
		t.Fatalf("2nd request: ok=%v remaining=%d, want ok=true remaining=0", ok, remaining)
	}

	ok, remaining, retryAfter := l.allow("client")
	if ok {
		t.Fatalf("3rd request should be rejected: bucket is empty (remaining=%d)", remaining)
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}

	// Advance less than the time needed for one token: still rejected.
	*clock = clock.Add(500 * time.Millisecond)
	if ok, _, _ := l.allow("client"); ok {
		t.Fatal("request after +500ms should still be rejected: only 0.5 tokens refilled")
	}

	// Advance past the 1-second mark: exactly one token has refilled.
	*clock = clock.Add(600 * time.Millisecond)
	if ok, _, _ := l.allow("client"); !ok {
		t.Fatal("request after +1.1s total should be allowed: one token refilled")
	}

	// Immediately after consuming that token, the bucket is empty again.
	if ok, _, _ := l.allow("client"); ok {
		t.Fatal("request immediately after consuming the refilled token should be rejected")
	}

	// Advance well beyond what's needed to fully refill: capped at burst, not
	// unbounded.
	*clock = clock.Add(time.Hour)
	if ok, remaining, _ := l.allow("client"); !ok || remaining != 1 {
		t.Fatalf("after a long idle period: ok=%v remaining=%d, want ok=true remaining=1 (capped at burst-1)", ok, remaining)
	}
}

// TestAllow_DoesNotDrainBelowZeroOnRepeatedRejection guards against Problem 5
// in rate-limiting.md: a rejected request must not consume/mutate bucket
// state beyond what a normal read would -- tokens must not go negative or
// keep draining while a client hammers a limiter that is already rejecting
// it.
func TestAllow_DoesNotDrainBelowZeroOnRepeatedRejection(t *testing.T) {
	l, _ := newTestLimiter(t, config.RateLimit{
		Enabled: true, Burst: 1, Rate: 60, TTL: 10 * time.Minute, MaxKeys: 1000,
	})

	if ok, _, _ := l.allow("client"); !ok {
		t.Fatal("first request should be allowed")
	}

	var firstRetry time.Duration
	for i := 0; i < 5; i++ {
		ok, _, retryAfter := l.allow("client")
		if ok {
			t.Fatalf("request %d should be rejected: bucket is empty", i)
		}
		if i == 0 {
			firstRetry = retryAfter
		} else if retryAfter != firstRetry {
			t.Fatalf("retryAfter changed across repeated rejections at a fixed clock: got %v, want %v (tokens must not keep draining)", retryAfter, firstRetry)
		}
	}
}

// TestEvictExpired_RemovesIdleFullyRefilledBuckets is the direct regression
// test for Problem 2 in rate-limiting.md: the bucket map must not grow
// without bound. A bucket that has been idle for at least the TTL and would
// already be fully refilled is swept; one that hasn't reached either
// condition yet is left alone.
func TestEvictExpired_RemovesIdleFullyRefilledBuckets(t *testing.T) {
	l, clock := newTestLimiter(t, config.RateLimit{
		Enabled: true, Burst: 1, Rate: 60, TTL: time.Minute, MaxKeys: 1000,
	})

	if ok, _, _ := l.allow("idle-client"); !ok {
		t.Fatal("first request should be allowed")
	}
	if !bucketExists(l, "idle-client") {
		t.Fatal("bucket should exist immediately after a request")
	}

	// Not idle long enough yet: must survive a sweep.
	*clock = clock.Add(30 * time.Second)
	l.evictExpired()
	if !bucketExists(l, "idle-client") {
		t.Fatal("bucket evicted before its TTL elapsed")
	}

	// Idle well past the TTL, and with enough elapsed time to have fully
	// refilled: must be swept.
	*clock = clock.Add(2 * time.Minute)
	l.evictExpired()
	if bucketExists(l, "idle-client") {
		t.Fatal("bucket should have been evicted: idle past TTL and fully refilled")
	}
}

func bucketExists(l *RateLimiter, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.buckets[key]
	return ok
}

// TestMaxKeysCap_RejectsNewKeysOnceTableIsFull is the direct regression test
// for Problem 3's "unbounded memory" half in rate-limiting.md: once the
// tracked-key table reaches max_keys, a brand-new key must be rejected rather
// than growing the map further, and existing keys must be unaffected (no
// evict-oldest that would hand an attacker a way to reset a legitimate
// client's budget for free).
func TestMaxKeysCap_RejectsNewKeysOnceTableIsFull(t *testing.T) {
	l, _ := newTestLimiter(t, config.RateLimit{
		Enabled: true, Burst: 5, Rate: 60, TTL: 10 * time.Minute, MaxKeys: 2,
	})

	if ok, _, _ := l.allow("client-a"); !ok {
		t.Fatal("client-a should be admitted: table has room")
	}
	if ok, _, _ := l.allow("client-b"); !ok {
		t.Fatal("client-b should be admitted: table has room")
	}

	ok, remaining, retryAfter := l.allow("client-c")
	if ok {
		t.Fatal("client-c should be rejected: max_keys capacity reached")
	}
	if remaining != 0 {
		t.Fatalf("remaining = %d, want 0 for a rejected, never-admitted key", remaining)
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}

	if ok, _, _ := l.allow("client-a"); !ok {
		t.Fatal("client-a should still be served: the cap must not evict existing keys")
	}

	if n := bucketCount(l); n != 2 {
		t.Fatalf("len(buckets) = %d, want 2 (max_keys cap enforced, rejected key never added)", n)
	}
}

func bucketCount(l *RateLimiter) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// TestNewRateLimiter_RejectsNonPositiveTunables is the successor to the
// plugin era's TestInitialize_LegacyRequestsWindowStillWorks. The legacy
// "requests"/"window" config shape is gone with the plugin map, and with it
// the reason that test existed. What replaces it is the stronger guarantee:
// a bad tunable is a construction error, never a silently-substituted
// default. That silent substitution was defect 3 of the plugin config
// contract -- a YAML type error produced a working-looking limiter with a
// limit nobody chose.
func TestNewRateLimiter_RejectsNonPositiveTunables(t *testing.T) {
	valid := config.RateLimit{Enabled: true, Rate: 60, Burst: 10, TTL: time.Minute, MaxKeys: 100}

	cases := map[string]func(*config.RateLimit){
		"zero rate":         func(c *config.RateLimit) { c.Rate = 0 },
		"negative rate":     func(c *config.RateLimit) { c.Rate = -1 },
		"zero burst":        func(c *config.RateLimit) { c.Burst = 0 },
		"zero ttl":          func(c *config.RateLimit) { c.TTL = 0 },
		"zero max_keys":     func(c *config.RateLimit) { c.MaxKeys = 0 },
		"negative max_keys": func(c *config.RateLimit) { c.MaxKeys = -1 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			mutate(&cfg)

			l, err := NewRateLimiter(cfg)
			if err == nil {
				_ = l.Close()
				t.Fatalf("NewRateLimiter accepted %s", name)
			}
		})
	}
}

// TestRateLimiter_CloseIsIdempotent proves shutdown can run twice without
// panicking on a double channel close. The composition root closes the
// router's resources on a path that can also run after a partially-failed
// boot, so this is a real sequence, not a hypothetical one.
func TestRateLimiter_CloseIsIdempotent(t *testing.T) {
	l, err := NewRateLimiter(config.RateLimit{
		Enabled: true, Rate: 60, Burst: 10, TTL: time.Minute, MaxKeys: 100,
	})
	if err != nil {
		t.Fatalf("build rate limiter: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
