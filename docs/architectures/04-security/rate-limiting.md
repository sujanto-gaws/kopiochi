# Rate Limiting

**Status:** Proposed
**Date:** 2026-08-02
**Severity:** Critical — the current implementation serialises the entire server.

`ratelimit` is listed in `config/default.yaml` under `plugins.middleware`, so this
code is **active in every deployment**.

---

## Problem 1: the mutex is held across the downstream handler

`internal/plugins/middleware/ratelimit.go:66-113`:

```go
func (p *RateLimiterPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ...
            p.mu.Lock()
            defer p.mu.Unlock()          // ← released only when the request COMPLETES

            now := time.Now()
            client, exists := p.requests[clientIP]

            if !exists || now.Sub(client.windowStart) > p.window {
                p.requests[clientIP] = &clientRate{count: 1, windowStart: now}
                next.ServeHTTP(w, r)     // ← downstream runs INSIDE the critical section
                return
            }

            client.count++
            ...
            next.ServeHTTP(w, r)         // ← and here too
        })
    }
}
```

`defer p.mu.Unlock()` runs when the middleware function returns — which is after
`next.ServeHTTP` has fully completed, including handler execution, all database
queries, and response writing.

**Consequence:** a single global mutex is held for the entire lifetime of every
request. The server processes exactly **one request at a time**, no matter how
many cores or connections are available. A single slow query blocks every other
client. Under load this presents as latency growing linearly with concurrency —
and it will not show up in a single-user smoke test.

This is the highest-impact performance defect in the repository, and it is on by
default.

## Problem 2: the map never evicts

```go
p.requests = make(map[string]*clientRate)    // line 50 — created once
```

Entries are added on first sight of a key and **never removed**. `Close()` nils
the whole map, but that only runs at shutdown. Memory grows with the number of
distinct keys, forever.

## Problem 3: the key is attacker-controlled

```go
clientIP := r.RemoteAddr
if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
    clientIP = forwarded          // whole header, verbatim, from any client
}
```

Two failures at once:

1. **Bypass.** An attacker sends `X-Forwarded-For: <random>` per request. Every
   request is a new key with `count = 1`. The limiter never triggers.
2. **Unbounded memory.** Each forged value creates a permanent map entry.
   Combined with problem 2, this is a trivial remote OOM: a few million requests
   with distinct headers exhaust the heap.

Using the *entire* XFF chain rather than a resolved client address also means
adding a legitimate proxy hop silently changes everyone's key.

## Problem 4: fixed windows allow 2× bursts

A fixed window resets abruptly. A client can send the full budget at the end of
one window and again at the start of the next — 2× the intended rate across the
boundary.

## Problem 5: the counter increments even when rejecting

```go
client.count++
if client.count > p.maxRequests {
    // 429 — but count was already incremented
}
```

A client that keeps hammering after being limited drives the counter up
indefinitely. Harmless with an `int` in practice, but it means the counter no
longer measures "requests served" and complicates any future logging on it.

## Problem 6: rate-limit headers are inconsistent

`X-RateLimit-*` headers are set only on the success path; the 429 response omits
them and sets only `Retry-After`. Clients that read the headers to self-throttle
get nothing exactly when they need it most. The header names are also the legacy
`X-` forms rather than the standardised `RateLimit-*` fields.

---

## Target design

### Token bucket with per-key locking, no lock held downstream

```go
package httpx

type RateLimiter struct {
    mu      sync.Mutex
    buckets map[string]*bucket
    rate    float64        // tokens per second
    burst   float64        // bucket capacity
    ttl     time.Duration  // idle eviction
    now     func() time.Time
}

type bucket struct {
    tokens float64
    last   time.Time
}

// allow decides in O(1) and returns immediately. The lock is NEVER held
// across next.ServeHTTP.
func (rl *RateLimiter) allow(key string) (ok bool, remaining int, retryAfter time.Duration) {
    now := rl.now()

    rl.mu.Lock()
    b, exists := rl.buckets[key]
    if !exists {
        b = &bucket{tokens: rl.burst, last: now}
        rl.buckets[key] = b
    }
    // Refill proportionally to elapsed time.
    b.tokens = math.Min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
    b.last = now

    if b.tokens >= 1 {
        b.tokens--
        remaining = int(b.tokens)
        ok = true
    } else {
        retryAfter = time.Duration((1-b.tokens)/rl.rate*float64(time.Second)) + time.Millisecond
    }
    rl.mu.Unlock()          // ← released before any downstream work

    return ok, remaining, retryAfter
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        key := ClientIPFrom(r.Context())        // resolved once by RealIP middleware

        ok, remaining, retryAfter := rl.allow(key)

        w.Header().Set("RateLimit-Limit", strconv.Itoa(int(rl.burst)))
        w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))

        if !ok {
            w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
            problem.Write(w, http.StatusTooManyRequests, "rate_limit_exceeded",
                "Too Many Requests", "Request rate exceeded; retry later.")
            return
        }

        next.ServeHTTP(w, r)                     // outside the critical section
    })
}
```

Properties:

- The lock covers a handful of arithmetic operations, not a request.
- Token bucket smooths bursts; no fixed-window doubling.
- Headers are emitted on **both** paths, using standardised names.
- 429 uses the same `problem+json` shape as every other error, unlike the current
  ad-hoc `{"error":"..."}` string.

### Eviction

```go
// Sweep runs until ctx is cancelled; registered on the lifecycle stack.
func (rl *RateLimiter) Sweep(ctx context.Context) {
    t := time.NewTicker(rl.ttl)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-t.C:
            cutoff := rl.now().Add(-rl.ttl)
            rl.mu.Lock()
            for k, b := range rl.buckets {
                if b.last.Before(cutoff) && b.tokens >= rl.burst {
                    delete(rl.buckets, k)     // idle and fully refilled: no state to lose
                }
            }
            rl.mu.Unlock()
        }
    }
}
```

Additionally cap `len(rl.buckets)`; when the cap is reached, reject new keys with
429 rather than growing without bound. That converts a memory-exhaustion attack
into a rate-limit rejection.

### Keying

- Key on the client IP resolved by the `RealIP` middleware from **trusted
  proxies only** (see [middleware hardening](middleware-hardening.md)). Never
  read `X-Forwarded-For` here.
- For authenticated requests, prefer keying on the subject: `user:<sub>`. It is
  more accurate than IP for shared NAT.
- Normalise IPv6 to a /64 prefix — a single client typically holds many addresses
  in one /64, so per-address keying is trivially bypassable.

### Per-route limits

A global limit is the wrong granularity for authentication endpoints. Login and
refresh deserve much tighter budgets:

```go
r.Route("/auth", func(r chi.Router) {
    r.With(httpx.RateLimit(cfg.RateLimit.Login)).Post("/login", h.Login)     // e.g. 5/min
    r.With(httpx.RateLimit(cfg.RateLimit.Refresh)).Post("/refresh", h.Refresh)
})
```

This complements — and does not replace — the account lockout already configured
via `auth.max_failed_attempts` and `auth.lock_duration`.

### Distributed deployments

The in-process limiter allows N× the intended rate across N replicas. Acceptable
for a single instance; document the limitation. When horizontal scaling arrives,
move to a shared store (Redis `INCR` with expiry, or a sliding-window script)
behind the same `allow(key)` interface so the middleware does not change.

### Configuration

```yaml
security:
  rate_limit:
    enabled: true
    rate: 100          # sustained requests per minute
    burst: 20          # instantaneous allowance
    ttl: "10m"         # idle bucket eviction
    max_keys: 100000   # hard cap on tracked keys
    login:
      rate: 5
      burst: 3
```

---

## Tests

```go
func TestAllowDoesNotHoldLockDuringHandler(t *testing.T) {
    // Two concurrent requests to a handler that blocks on a channel.
    // Both must enter the handler; with the current code the second never does.
}

func TestBucketRefillsOverTime(t *testing.T)      // inject rl.now for determinism
func TestEvictsIdleBuckets(t *testing.T)
func TestRejectsBeyondMaxKeys(t *testing.T)
func TestXForwardedForIsIgnoredWithoutTrustedProxies(t *testing.T)
func TestHeadersPresentOn429(t *testing.T)
```

`TestAllowDoesNotHoldLockDuringHandler` is the direct regression test for the
critical defect and should be written first — it fails against the current
implementation.

---

## Related documents

- [Middleware hardening](middleware-hardening.md)
- [Extension framework](../01-modularity/extension-framework.md) — why this stops being a "plugin"
- [Observability](../06-quality/observability.md)
