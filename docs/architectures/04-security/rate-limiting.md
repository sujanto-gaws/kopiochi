# Rate Limiting

**Status:** Accepted — partially implemented (Phase 2.1, `dcc6e5d` + `d130519`)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2 — problems 1–6 below are all
resolved. What remains unshipped from the target design is listed under
[What did not ship](#what-did-not-ship).

> **The `Severity: Critical` header no longer applies and has been removed.**
> It read *"Critical — the current implementation serialises the entire
> server."* `dcc6e5d` replaced the fixed-window counter with a token bucket
> whose lock is released inside `allow()` before `next.ServeHTTP` is ever
> reached, so the server no longer serialises.
> `TestRateLimitAllowsConcurrentRequests` — task 1.1(c), previously `t.Skip`ped
> because it failed by design — now runs unconditionally in the default suite
> and passes.

`ratelimit` is listed in `config/default.yaml` under `plugins.middleware`, so this
code is **active in every deployment**. It is still a plugin; converting it to
direct construction in `internal/httpx` is Phase 3.5.

---

## Problem 1 (fixed): the mutex is held across the downstream handler

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

This was the highest-impact performance defect in the repository, and it was on
by default.

*Fixed in `dcc6e5d`. `allow()` (`ratelimit.go:233-270`) takes the mutex, does the
bucket lookup, refill, and decrement, and returns; `Middleware` calls
`next.ServeHTTP` only after `allow()` has returned, outside any critical
section. Regression test: `TestRateLimitAllowsConcurrentRequests`
(`internal/plugins/middleware/ratelimit_test.go`), which now runs by default.*

## Problem 2 (fixed): the map never evicts

```go
p.requests = make(map[string]*clientRate)    // line 50 — created once
```

Entries are added on first sight of a key and **never removed**. `Close()` nils
the whole map, but that only runs at shutdown. Memory grows with the number of
distinct keys, forever.

*Fixed in `dcc6e5d` two ways. `Initialize` starts a `sweepLoop` goroutine that
ticks every `ttl` (default 10m) and calls `evictExpired`
(`ratelimit.go:207-227`), which drops any bucket idle past the cutoff **and**
already projected back to full burst — a bucket with no state left to lose.
`Close` stops the loop. Independently, `max_keys` (default 100,000) caps the
table; see [the `max_keys` behavioural note](#the-max_keys-rejection-path)
below. Tests: `TestEvictExpired_RemovesIdleFullyRefilledBuckets`,
`TestMaxKeysCap_RejectsNewKeysOnceTableIsFull`.*

## Problem 3 (fixed): the key is attacker-controlled

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

*Fixed in `dcc6e5d` + `333968c`. This file no longer reads `X-Forwarded-For` at
all. `clientKey` (`ratelimit.go:315-325`) reads the address resolved upstream by
`internal/middleware.RealIP`, which honours forwarded headers only from
configured trusted-proxy CIDRs and defaults to trusting none; if `RealIP` has
not run it falls back to the raw peer address, never to a header. See
[middleware hardening](middleware-hardening.md).*

## Problem 4 (fixed): fixed windows allow 2× bursts

A fixed window resets abruptly. A client can send the full budget at the end of
one window and again at the start of the next — 2× the intended rate across the
boundary.

*Fixed in `dcc6e5d`: there is no window any more. Tokens refill continuously in
proportion to elapsed time, capped at `burst`
(`b.tokens = math.Min(p.burst, b.tokens+elapsed*p.rate)`), so there is no
boundary to double up across. `TestAllow_RefillsTokensOverInjectedClock` proves
the refill maths against an injected clock rather than a sleep.*

## Problem 5 (fixed): the counter increments even when rejecting

```go
client.count++
if client.count > p.maxRequests {
    // 429 — but count was already incremented
}
```

A client that keeps hammering after being limited drives the counter up
indefinitely. Harmless with an `int` in practice, but it means the counter no
longer measures "requests served" and complicates any future logging on it.

*Fixed in `dcc6e5d`: `allow()` decrements only on the accept path
(`if b.tokens >= 1 { b.tokens-- }`). A rejected request leaves the bucket
untouched, so a client hammering a limit does not push its own recovery further
away. `TestAllow_DoesNotDrainBelowZeroOnRepeatedRejection` covers it.*

## Problem 6 (fixed): rate-limit headers are inconsistent

`X-RateLimit-*` headers are set only on the success path; the 429 response omits
them and sets only `Retry-After`. Clients that read the headers to self-throttle
get nothing exactly when they need it most. The header names are also the legacy
`X-` forms rather than the standardised `RateLimit-*` fields.

*Fixed in `dcc6e5d`: `RateLimit-Limit` and `RateLimit-Remaining` are set before
the accept/reject branch, so both paths carry them, and the names are the
standardised forms. `Retry-After` is added on the 429. The 429 **body** is still
the ad-hoc `{"error":"rate limit exceeded"}` rather than `problem+json` — see
[What did not ship](#what-did-not-ship).*

---

## Target design

*Shipped in `dcc6e5d`, with three deliberate deviations from the sketch below:
the type is still `RateLimiterPlugin` in
`internal/plugins/middleware/ratelimit.go` rather than a `RateLimiter` in
`internal/httpx` (the move is Phase 3.5); eviction runs on a goroutine owned by
`Initialize`/`Close` rather than on a lifecycle stack that does not exist yet
(Phase 3.9); and the 429 body is unchanged. Everything else — bucket, clock
injection, TTL eviction, `max_keys`, header handling — matches.*

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

### The `max_keys` rejection path

Shipped as written, and the choice is worth stating explicitly because the
obvious alternative is worse. When the table is full, `allow()` **rejects the
new key** (`ratelimit.go:243-253`); it does not evict an existing bucket to make
room. Evict-oldest would be gameable: an attacker who floods fresh keys could
push a legitimate, actively-throttled client's bucket out of the table, and that
client's next request would start a brand-new bucket at full burst — the flood
would hand the throttled client its budget back. Rejecting instead leaves every
admitted key served normally, and the TTL sweep continuously reclaims idle slots,
so the state self-heals rather than needing intervention.

One behavioural consequence to carry, because it is visible to clients and is
not what the headers usually mean:

> A request rejected by the `max_keys` cap gets `RateLimit-Remaining: 0` and a
> `Retry-After` of 1 second (`maxKeysRetryAfter`) for a key that was **never
> admitted** and therefore has no bucket. Both values are synthetic. There is no
> per-key state to derive a real `Retry-After` from, so the constant is a
> deliberately conservative hint rather than a fabricated per-key number — but a
> client reading `Remaining: 0` here is not being told "you exhausted your
> budget", it is being told "the server is not tracking you at all right now".
> Anything that reports on limiter rejections should distinguish the two, or the
> saturation signal reads as ordinary throttling.

### Keying

- Key on the client IP resolved by the `RealIP` middleware from **trusted
  proxies only** (see [middleware hardening](middleware-hardening.md)). Never
  read `X-Forwarded-For` here. ✅ Shipped — `clientKey`, `dcc6e5d` + `333968c`.
- For authenticated requests, prefer keying on the subject: `user:<sub>`. It is
  more accurate than IP for shared NAT. ⏳ Not shipped.
- Normalise IPv6 to a /64 prefix — a single client typically holds many addresses
  in one /64, so per-address keying is trivially bypassable. ⏳ Not shipped;
  `clientKey` returns the full address.

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

The `security.rate_limit` block below is the target shape and is **not** what
ships. Config still reaches the limiter through the generic plugin map, so the
tunables live under `plugins.custom.ratelimit` and are parsed by
`Initialize` (`ratelimit.go:74-141`):

| Key | Meaning | Default |
|---|---|---|
| `rate` | sustained requests per minute | derived from `requests`/`window` |
| `burst` | bucket capacity / instantaneous allowance | `requests` |
| `ttl` | idle bucket eviction interval | `10m` |
| `max_keys` | hard cap on tracked keys | `100000` |
| `requests` + `window` | legacy fixed-window pair, translated to `burst` = `requests`, `rate` = `requests`/`window` | `100`, `1m` |

The legacy pair is accepted so existing deployments' config keeps working across
the rewrite (`TestInitialize_LegacyRequestsWindowStillWorks`). `config/default.yaml`
ships `plugins.custom: {}`, so the shipped defaults are burst 100, rate 100/min,
ttl 10m, max_keys 100,000. Invalid values — non-positive `rate`, `burst`, `ttl`,
or `max_keys`, or an unparseable duration — are errors from `Initialize` rather
than silent fallbacks.

Target shape, for when Phase 3.5 lifts this out of the plugin map:

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

## What did not ship

Phase 2.1 closed every defect above. These items from the target design are
still outstanding, and none of them is a security regression on the old code:

| Item | Where it goes |
|---|---|
| 429 body as `problem+json` instead of `{"error":"rate limit exceeded"}` | Needs the shared `problem` writer that [middleware hardening](middleware-hardening.md) proposes alongside `httpx.Recovery`; neither exists yet |
| Per-route limits (tighter budgets on `/auth/login`, `/auth/refresh`) | Needs direct construction — Phase 3.5 |
| Keying on `user:<sub>` for authenticated requests | Phase 3.5 |
| IPv6 /64 normalisation | Phase 3.5 |
| `security.rate_limit` typed config replacing the plugin map | Phase 3.5 / [ADR-008](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md) decision 8 |
| Shared store for multi-replica deployments | Unchanged: the in-process limiter still allows N× the rate across N replicas |

---

## A race the fix introduced, and what that says about coverage

`dcc6e5d` was a concurrency fix, and it shipped with a data race of its own:
`p.now` (written by `setClock`) and `p.initialized` / `p.burst` (written by
`Initialize` and `Close`) were read without the mutex that guards their writes.
A request served concurrently with shutdown races on `initialized` and `burst`.

It was **found by inspection, not by tooling**, and fixed in `d130519`:
`snapshot()` reads `initialized` and `burst` together under the lock, `allow()`
takes the clock inside its own critical section, and `evictExpired` already held
the lock.

That is exactly the class of bug `go test -race` exists to catch, and `-race`
**cannot run in this development environment** — see
[testing strategy](../06-quality/testing-strategy.md#race-detection-is-outstanding).
Concurrency correctness here currently rests on review. Phase 4.4's CI, running
on Linux, is what turns that back into a machine check.

---

## Tests

Shipped in `dcc6e5d`, in `internal/plugins/middleware/`:

| Test | File | Covers |
|---|---|---|
| `TestRateLimitAllowsConcurrentRequests` | `ratelimit_test.go` | Problem 1 — no longer skipped, runs by default |
| `TestAllow_RefillsTokensOverInjectedClock` | `ratelimit_tokenbucket_test.go` | Refill maths, injected clock, no sleeps |
| `TestAllow_DoesNotDrainBelowZeroOnRepeatedRejection` | `ratelimit_tokenbucket_test.go` | Problem 5 |
| `TestEvictExpired_RemovesIdleFullyRefilledBuckets` | `ratelimit_tokenbucket_test.go` | Problem 2 |
| `TestMaxKeysCap_RejectsNewKeysOnceTableIsFull` | `ratelimit_tokenbucket_test.go` | The cap, including the rejection semantics above |
| `TestInitialize_LegacyRequestsWindowStillWorks` | `ratelimit_tokenbucket_test.go` | Config back-compat |

The originally sketched `TestXForwardedForIsIgnoredWithoutTrustedProxies` landed
next to the code it actually exercises, as
`TestRealIP_IgnoresForwardedHeadersWithoutTrustedProxies` in
`internal/middleware/clientip_test.go`. `TestHeadersPresentOn429` is not written;
the header behaviour is asserted incidentally by the cap test rather than
directly.

---

## Related documents

- [Middleware hardening](middleware-hardening.md)
- [Extension framework](../01-modularity/extension-framework.md) — why this stops being a "plugin"
- [Observability](../06-quality/observability.md)
