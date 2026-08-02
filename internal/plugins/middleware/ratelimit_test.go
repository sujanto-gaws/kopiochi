package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestRateLimitAllowsConcurrentRequests is task 1.1(c) from the remediation
// plan (docs/architectures/06-quality/testing-strategy.md:86-105) — the
// highest-value test in this batch. Two requests dispatched concurrently
// must both be inside the wrapped handler at the same time; a rate limiter
// should gate *how many* requests get through, not serialize them one at a
// time regardless of limit.
//
// RateLimiterPlugin.Middleware (ratelimit.go) currently does:
//
//	p.mu.Lock()
//	defer p.mu.Unlock()
//	...
//	next.ServeHTTP(w, r)
//
// holding the single mutex for the *entire* duration of next.ServeHTTP. A
// second concurrent request cannot even start running the handler — it
// blocks on p.mu.Lock() — until the first request's handler (and its
// ServeHTTP call) has completely returned. That means this test is EXPECTED
// TO FAIL against the current implementation: the second goroutine never
// reaches `entered <- struct{}{}` until `release` is closed, but this test
// only closes `release` after observing both entries, so it deadlocks and
// hits the 2s timeout instead.
//
// This is deliberately a documented failure, not a bug being papered over.
// Fixing the limiter is Phase 2.1 — not this task. To avoid a permanently
// red default test run being mistaken for a broken build, the test is
// skipped unless RUN_KNOWN_FAILING=1 is set, so it is skipped-with-reason
// rather than silently absent, and there is still a concrete way to run it
// and see the red:
//
//	RUN_KNOWN_FAILING=1 go test ./internal/plugins/middleware/... -run TestRateLimitAllowsConcurrentRequests -v
func TestRateLimitAllowsConcurrentRequests(t *testing.T) {
	if os.Getenv("RUN_KNOWN_FAILING") == "" {
		t.Skip("known-failing until Phase 2.1: rate limiter holds its mutex across ServeHTTP, serializing concurrent requests instead of allowing them to run simultaneously. Set RUN_KNOWN_FAILING=1 to run this test and observe the failure.")
	}

	p := NewRateLimiterPlugin()
	if err := p.Initialize(map[string]interface{}{
		"requests": float64(100),
		"window":   "1m",
	}); err != nil {
		t.Fatalf("initialize rate limiter: %v", err)
	}

	release := make(chan struct{})
	entered := make(chan struct{}, 2)

	h := p.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
	}))

	req := func() *http.Request {
		return httptest.NewRequest(http.MethodGet, "/", nil)
	}

	go h.ServeHTTP(httptest.NewRecorder(), req())
	go h.ServeHTTP(httptest.NewRecorder(), req())

	// Both handlers must be inside simultaneously. Current code deadlocks
	// here: the second ServeHTTP call is still blocked on the limiter's
	// mutex, held by the first call across its own ServeHTTP.
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("handler blocked: rate limiter holds its lock across ServeHTTP")
		}
	}
	close(release)
}
