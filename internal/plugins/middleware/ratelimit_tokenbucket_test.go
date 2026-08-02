package middleware

import (
	"testing"
	"time"
)

// newTestLimiter builds an initialized RateLimiterPlugin with a fake,
// test-controlled clock. Callers advance `clock` directly (mutating the
// closed-over time.Time) to simulate the passage of time deterministically
// -- never time.Sleep, which can't reliably cross a window/refill boundary
// in a unit test.
func newTestLimiter(t *testing.T, cfg map[string]interface{}) (p *RateLimiterPlugin, clock *time.Time) {
	t.Helper()

	p = NewRateLimiterPlugin()
	if err := p.Initialize(cfg); err != nil {
		t.Fatalf("initialize rate limiter: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	now := time.Unix(1_700_000_000, 0)
	p.setClock(func() time.Time { return now })

	return p, &now
}

// TestAllow_RefillsTokensOverInjectedClock exercises the token bucket
// refill maths using an injected clock: burst caps the initial allowance,
// exhausting it rejects further requests, and tokens are only replenished
// once enough simulated time has passed -- proportional to the configured
// rate, never in a single fixed-window jump.
func TestAllow_RefillsTokensOverInjectedClock(t *testing.T) {
	p, clock := newTestLimiter(t, map[string]interface{}{
		"burst": float64(2),
		"rate":  float64(60), // 60 requests/min == 1 token/sec
		"ttl":   "10m",
	})

	if ok, remaining, _ := p.allow("client"); !ok || remaining != 1 {
		t.Fatalf("1st request: ok=%v remaining=%d, want ok=true remaining=1", ok, remaining)
	}
	if ok, remaining, _ := p.allow("client"); !ok || remaining != 0 {
		t.Fatalf("2nd request: ok=%v remaining=%d, want ok=true remaining=0", ok, remaining)
	}

	ok, remaining, retryAfter := p.allow("client")
	if ok {
		t.Fatalf("3rd request should be rejected: bucket is empty (remaining=%d)", remaining)
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}

	// Advance less than the time needed for one token: still rejected.
	*clock = clock.Add(500 * time.Millisecond)
	if ok, _, _ := p.allow("client"); ok {
		t.Fatal("request after +500ms should still be rejected: only 0.5 tokens refilled")
	}

	// Advance past the 1-second mark: exactly one token has refilled.
	*clock = clock.Add(600 * time.Millisecond)
	if ok, _, _ := p.allow("client"); !ok {
		t.Fatal("request after +1.1s total should be allowed: one token refilled")
	}

	// Immediately after consuming that token, the bucket is empty again.
	if ok, _, _ := p.allow("client"); ok {
		t.Fatal("request immediately after consuming the refilled token should be rejected")
	}

	// Advance well beyond what's needed to fully refill: capped at burst,
	// not unbounded.
	*clock = clock.Add(time.Hour)
	if ok, remaining, _ := p.allow("client"); !ok || remaining != 1 {
		t.Fatalf("after a long idle period: ok=%v remaining=%d, want ok=true remaining=1 (capped at burst-1)", ok, remaining)
	}
}

// TestAllow_DoesNotIncrementUsageOnRejection guards against Problem 5 in
// rate-limiting.md: a rejected request must not consume/mutate bucket state
// beyond what a normal read would -- tokens must not go negative or keep
// draining while a client hammers a limiter that is already rejecting it.
func TestAllow_DoesNotDrainBelowZeroOnRepeatedRejection(t *testing.T) {
	p, _ := newTestLimiter(t, map[string]interface{}{
		"burst": float64(1),
		"rate":  float64(60),
		"ttl":   "10m",
	})

	if ok, _, _ := p.allow("client"); !ok {
		t.Fatal("first request should be allowed")
	}

	var firstRetry time.Duration
	for i := 0; i < 5; i++ {
		ok, _, retryAfter := p.allow("client")
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
	p, clock := newTestLimiter(t, map[string]interface{}{
		"burst": float64(1),
		"rate":  float64(60),
		"ttl":   "1m",
	})

	if ok, _, _ := p.allow("idle-client"); !ok {
		t.Fatal("first request should be allowed")
	}
	if !bucketExists(p, "idle-client") {
		t.Fatal("bucket should exist immediately after a request")
	}

	// Not idle long enough yet: must survive a sweep.
	*clock = clock.Add(30 * time.Second)
	p.evictExpired()
	if !bucketExists(p, "idle-client") {
		t.Fatal("bucket evicted before its TTL elapsed")
	}

	// Idle well past the TTL, and with enough elapsed time to have fully
	// refilled: must be swept.
	*clock = clock.Add(2 * time.Minute)
	p.evictExpired()
	if bucketExists(p, "idle-client") {
		t.Fatal("bucket should have been evicted: idle past TTL and fully refilled")
	}
}

func bucketExists(p *RateLimiterPlugin, key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.buckets[key]
	return ok
}

// TestMaxKeysCap_RejectsNewKeysOnceTableIsFull is the direct regression test
// for Problem 3's "unbounded memory" half in rate-limiting.md: once the
// tracked-key table reaches max_keys, a brand-new key must be rejected
// rather than growing the map further, and existing keys must be
// unaffected (no evict-oldest that would hand an attacker a way to reset a
// legitimate client's budget for free).
func TestMaxKeysCap_RejectsNewKeysOnceTableIsFull(t *testing.T) {
	p, _ := newTestLimiter(t, map[string]interface{}{
		"burst":    float64(5),
		"rate":     float64(60),
		"ttl":      "10m",
		"max_keys": float64(2),
	})

	if ok, _, _ := p.allow("client-a"); !ok {
		t.Fatal("client-a should be admitted: table has room")
	}
	if ok, _, _ := p.allow("client-b"); !ok {
		t.Fatal("client-b should be admitted: table has room")
	}

	ok, remaining, retryAfter := p.allow("client-c")
	if ok {
		t.Fatal("client-c should be rejected: max_keys capacity reached")
	}
	if remaining != 0 {
		t.Fatalf("remaining = %d, want 0 for a rejected, never-admitted key", remaining)
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}

	if ok, _, _ := p.allow("client-a"); !ok {
		t.Fatal("client-a should still be served: the cap must not evict existing keys")
	}

	if n := bucketCount(p); n != 2 {
		t.Fatalf("len(buckets) = %d, want 2 (max_keys cap enforced, rejected key never added)", n)
	}
}

func bucketCount(p *RateLimiterPlugin) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buckets)
}

// TestInitialize_LegacyRequestsWindowStillWorks guards backward
// compatibility: existing deployments configuring the old
// "requests"/"window" fixed-window shape must still get an equivalent,
// working limiter rather than an error or a silently-ignored config.
func TestInitialize_LegacyRequestsWindowStillWorks(t *testing.T) {
	p := NewRateLimiterPlugin()
	if err := p.Initialize(map[string]interface{}{
		"requests": float64(2),
		"window":   "1m",
	}); err != nil {
		t.Fatalf("initialize with legacy requests/window config: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	if ok, _, _ := p.allow("client"); !ok {
		t.Fatal("1st request under legacy config should be allowed")
	}
	if ok, _, _ := p.allow("client"); !ok {
		t.Fatal("2nd request under legacy config should be allowed (burst == requests == 2)")
	}
	if ok, _, _ := p.allow("client"); ok {
		t.Fatal("3rd request under legacy config should be rejected: burst exhausted")
	}
}
