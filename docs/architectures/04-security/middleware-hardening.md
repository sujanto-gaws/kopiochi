# HTTP Middleware Hardening

**Status:** Partially implemented
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1 — the auth-binding section is
resolved; CORS, proxy headers, and security response headers are all still open
(Phase 2).

Covers CORS, proxy header handling, security headers, auth binding, and the
middleware stack order. Rate limiting has its own document:
[rate-limiting.md](rate-limiting.md).

---

## CORS

### Problem 1 — permissive default

`internal/plugins/middleware/cors.go:27-35`:

```go
if origins, ok := cfg["allowed_origins"].([]interface{}); ok {
    ...
} else {
    p.allowedOrigins = []string{"*"}     // default: allow everything
}
```

`plugins.custom` is `{}` in `config/default.yaml`, so CORS initialises with **no
config at all** and every deployment starts fully permissive. The safe value must
be the default; permissive must be a deliberate, visible choice.

### Problem 2 — arbitrary origin reflection

`cors.go:88-107`:

```go
allowed := false
for _, allowedOrigin := range p.allowedOrigins {
    if allowedOrigin == "*" || allowedOrigin == origin {
        allowed = true
        break
    }
}
...
if origin != "" {
    w.Header().Set("Access-Control-Allow-Origin", origin)   // echoes the CALLER's origin
}
```

With the `"*"` default, any `Origin` is matched and then **reflected verbatim**.

Today `allowCredentials` defaults to `false`, which keeps the immediate impact
low. But the combination is one config line away from critical: setting
`allow_credentials: true` yields reflected-origin **plus**
`Access-Control-Allow-Credentials: true`, which is a complete bypass of the same
origin policy — any site can read authenticated responses. The code must not
permit that combination at all.

### Problem 3 — no `Vary: Origin`

The response varies by request `Origin`, but no `Vary` header is emitted. Any
shared cache or CDN will store one origin's `Access-Control-Allow-Origin` and
serve it to a different origin — cache poisoning that produces both false
rejections and unintended grants.

### Problem 4 — non-browser requests rejected with 403

`cors.go:96-100` aborts with 403 whenever an `Origin` header is present and not
allowed. CORS is a *browser* enforcement mechanism; the server's job is to
withhold the header, not to reject the request. Any client that happens to send
`Origin` (some proxies, some SDKs) is broken for no security gain — the browser
would have blocked the read anyway.

### Problem 5 — preflight terminates the chain early

`cors.go:118-121` answers `OPTIONS` with 204 for **every** path, including ones
that do not exist. This leaks nothing serious but makes 404 behaviour inconsistent
and bypasses downstream middleware.

### Target

```go
func CORS(cfg config.CORS) func(http.Handler) http.Handler {
    allowed := make(map[string]bool, len(cfg.AllowedOrigins))
    for _, o := range cfg.AllowedOrigins {
        allowed[strings.ToLower(o)] = true
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")

            // Always advertise that the response depends on Origin.
            w.Header().Add("Vary", "Origin")

            if origin != "" && allowed[strings.ToLower(origin)] {
                w.Header().Set("Access-Control-Allow-Origin", origin)
                if cfg.AllowCredentials {
                    w.Header().Set("Access-Control-Allow-Credentials", "true")
                }
            }
            // Not allowed → simply omit the header. Do NOT 403; the browser enforces.

            if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
                w.Header().Add("Vary", "Access-Control-Request-Method")
                w.Header().Add("Vary", "Access-Control-Request-Headers")
                w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
                w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
                w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
                w.WriteHeader(http.StatusNoContent)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

Config validation makes the dangerous combination unrepresentable:

```go
func (c CORS) Validate() error {
    for _, o := range c.AllowedOrigins {
        if o == "*" && c.AllowCredentials {
            return errors.New(`security.cors: allowed_origins "*" cannot be combined with allow_credentials`)
        }
        if o != "*" && !strings.HasPrefix(o, "http://") && !strings.HasPrefix(o, "https://") {
            return fmt.Errorf("security.cors: origin %q must include a scheme", o)
        }
    }
    if len(c.AllowedOrigins) == 0 && c.Enabled {
        return errors.New("security.cors: enabled with an empty allowlist; disable it or list origins")
    }
    return nil
}
```

Defaults become: `enabled: false`, empty allowlist, credentials off.

---

## Proxy headers / client IP

### Problem

Two independent consumers disagree, and both are unsafe behind an untrusted edge:

1. `server.go:59` uses chi's `middleware.RealIP`, which trusts `X-Forwarded-For`
   and `X-Real-IP` from **any** peer.
2. `ratelimit.go:76-78` separately reads `X-Forwarded-For` and uses the **entire
   header value** as the rate-limit key — so an attacker sends a different value
   per request and never hits the limit. Detail in
   [rate limiting](rate-limiting.md).

If the service is exposed directly, a client can claim any IP: rate limits are
bypassed, and access logs and audit records are falsified.

### Target

Resolve the client IP once, from trusted proxies only, and make it the single
source of truth:

```go
type Security struct {
    TrustedProxies []string `mapstructure:"trusted_proxies"` // CIDRs, e.g. 10.0.0.0/8
}

// RealIP walks X-Forwarded-For right-to-left, skipping trusted hops,
// and returns the first untrusted address. Falls back to RemoteAddr.
func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler
```

- Empty `trusted_proxies` ⇒ **ignore all forwarding headers** and use
  `RemoteAddr`. Safe default for direct exposure.
- The resolved IP goes into the request context; rate limiting, logging, and
  auditing all read from there rather than re-parsing headers.

---

## Security response headers

None are currently set. Add a small middleware:

| Header | Value | Purpose |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | Stop MIME sniffing |
| `X-Frame-Options` | `DENY` | Clickjacking (API responses are never framed) |
| `Referrer-Policy` | `no-referrer` | No URL leakage |
| `Cache-Control` | `no-store` on authenticated responses | Keep tokens/PII out of caches |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | HTTPS only; **only when TLS-terminated** |
| `Content-Security-Policy` | `default-src 'none'` | For JSON APIs; relax for the swagger UI route |

HSTS must be conditional on the deployment terminating TLS — emitting it over
plain HTTP in local development breaks `http://localhost` in a way that persists
in the browser.

---

## Auth middleware binding — fixed

*Resolved in `6d0c1b7` / `ef76759`.*

Historically `main.go:99-112` resolved auth once and **failed open**:

```go
if authMiddleware != nil {
    protected = v1.With(authMiddleware)
} else {
    protected = v1          // every "protected" route is now public, silently
}
```

With `plugins.auth.jwt.enabled: false` — still the shipped default
(`config/default.yaml:47`) — every protected route was served unauthenticated,
with no warning logged.

Target, per [routing and versioning](../02-composition/routing-and-versioning.md)
— **all three now implemented**:

- ✅ Modules declare their own protected groups.
- ✅ The auth middleware is a constructor dependency, never nil: if a module needs
  authentication and the verifier cannot be built, `New()` returns an error and
  the process does not start. `modules/identity/module.go` derives its
  `AuthRequired` from the token service it just built; `cmd/api/container.go:90-98`
  does the same for the user module.
- ✅ Fail closed, always. `main.go` no longer derives auth middleware from the
  jwt-auth plugin at all (see the comment at `main.go:96-103`).

The defects in what the middleware then *does* with a token — no `iss`/`aud`
validation, one `Validate()` for three token classes — are unchanged; see
[token architecture](token-architecture.md).

---

## Panic recovery — a gap, not a solved problem

> **Withdrawn — and the premise inverts.** An earlier revision stated that
> `internal/middleware/recovery.go` is "**correct** and should be kept", recovering
> with request ID and stack and emitting `application/problem+json`, and asked
> only for two refinements. That file appears in no commit of this repository
> (`git log --all --diff-filter=A -- internal/middleware/recovery.go` returns
> nothing); `internal/middleware/` contains a single file, `zerolog.go`. The claim
> could not be substantiated and has been withdrawn. Recovery is therefore not a
> solved problem here — it is an open one.

Panic recovery is chi's `middleware.Recoverer`, registered first in the core
stack (`internal/infrastructure/http/server/server.go:57`). It catches the panic
and returns 500, which is the important part. What it does **not** do:

- It does not emit `application/problem+json`. Every other error path in the
  service does (`modules/identity/transport/helpers.go:71`,
  `internal/infrastructure/http/handlers/helpers.go:69`), so a panic is the one
  response shape a client cannot parse uniformly.
- It does not log the request ID with the stack. `Recoverer` is registered
  *before* `middleware.RequestID` (`server.go:58`), so no request ID exists in the
  context when the panic is caught even if it were logged. A panic is
  uncorrelatable with the access-log line for the request that caused it.
- It writes to stderr in chi's own format, bypassing zerolog entirely — see
  [observability](../06-quality/observability.md).

### Target

Replace it with `httpx.Recovery(log)`, registered **after** `RequestID`:

1. Recover, log at `error` through the injected zerolog logger with
   `request_id`, method, path, and stack.
2. Emit RFC 7807 `application/problem+json` with a fixed, non-revealing detail
   ("An unexpected error occurred."). Never the panic value or stack.
3. Do not attempt to write a body if the handler already wrote a status — wrap
   the `ResponseWriter` and track it, otherwise the 500 is appended to a partial
   200 response.
4. Re-panic on `http.ErrAbortHandler`, which is the documented way to signal an
   intentional abort and should not be logged as a crash.

Points 3 and 4 were the only two items in the previous revision; they remain
correct, but they are refinements on top of work that has not been done rather
than on top of an existing implementation.

## Not-found and method-not-allowed responses

The same gap applies at the router level. `server.NewRouter` sets neither
`r.NotFound` nor `r.MethodNotAllowed`, so 404 and 405 fall through to chi's
plain-text defaults and do not match the problem+json shape used everywhere else.
Add both handlers alongside `httpx.Recovery`, emitting the same envelope.

> *An earlier revision described `handlers.NotFound()` and
> `handlers.MethodNotAllowed()` as existing and returning "the same problem
> shape". No such functions appear in any commit; the claim has been withdrawn.*

---

## Stack order

```go
r.Use(middleware.RequestID)              // 1. correlation id for everything below
r.Use(httpx.RealIP(cfg.TrustedProxies))  // 2. resolve client IP once
r.Use(httpx.Recovery(log))               // 3. catch panics from everything below
r.Use(httpx.RequestLogger(log))          // 4. log with id + resolved IP + status
r.Use(httpx.SecurityHeaders(cfg))        // 5. cheap, applies to all responses
r.Use(middleware.Timeout(cfg.RequestTimeout))
r.Use(httpx.CORS(cfg.CORS))              // 7. before rate limiting: preflights are cheap
r.Use(httpx.RateLimit(cfg.RateLimit))    // 8. needs the resolved IP from (2)
// auth is applied per route group, inside modules
```

Ordering constraints worth stating explicitly:

- `RequestID` first so every log line and error response can be correlated.
- `RealIP` before `RateLimit`, which depends on it.
- `Recovery` above the application middleware so their panics are caught, and
  below `RequestID` so the panic log carries the id.
- `CORS` before `RateLimit` so preflight `OPTIONS` requests are not counted
  against a user's budget.

One correction to a claim made in an earlier revision: the current
`server.NewRouter` does **not** register `NotFound`/`MethodNotAllowed` before
`r.Use`. It registers neither, at any point — `git show 4fdc609^:internal/infrastructure/http/server/server.go`
shows the same five `r.Use` calls and nothing else. The ordering concern
described there was withdrawn; the real gap is the missing handlers, above.

The one ordering defect that *is* present today: `middleware.Recoverer` is
registered before `middleware.RequestID` (`server.go:57-58`), so a recovered
panic has no request ID available to log. The stack above reverses that.

---

## Tests

```go
func TestCORS_DisallowedOriginGetsNoHeader(t *testing.T)  // no ACAO, and NOT a 403
func TestCORS_AlwaysSetsVaryOrigin(t *testing.T)
func TestCORS_WildcardWithCredentialsRejectedAtConfigLoad(t *testing.T)
func TestRealIP_IgnoresXFFWhenNoTrustedProxies(t *testing.T)
func TestSecurityHeadersPresent(t *testing.T)
func TestProtectedRouteRequiresToken(t *testing.T)        // guards against fail-open
func TestRecovery_EmitsProblemJSONWithRequestID(t *testing.T)
func TestNotFound_EmitsProblemJSON(t *testing.T)
```

---

## Related documents

- [Rate limiting](rate-limiting.md)
- [Token architecture](token-architecture.md)
- [Routing and versioning](../02-composition/routing-and-versioning.md)
- [Configuration model](../03-configuration/configuration-model.md)
