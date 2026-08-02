# HTTP Middleware Hardening

**Status:** Proposed
**Date:** 2026-08-02

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

1. `server.go:64` uses chi's `middleware.RealIP`, which trusts `X-Forwarded-For`
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

## Auth middleware binding

`main.go:99-112` resolves auth once and **fails open**:

```go
if authMiddleware != nil {
    protected = v1.With(authMiddleware)
} else {
    protected = v1          // every "protected" route is now public, silently
}
```

With `plugins.auth.jwt.enabled: false` — the shipped default — every protected
route is served unauthenticated. No warning is logged.

Target, per [routing and versioning](../02-composition/routing-and-versioning.md):

- Modules declare their own protected groups.
- The auth middleware is a constructor dependency, never nil: if a module needs
  authentication and the verifier cannot be built, `New()` returns an error and
  the process does not start.
- Fail closed, always.

---

## Panic recovery

`internal/middleware/recovery.go` is **correct** and should be kept: it recovers,
logs with request ID and stack, and emits `application/problem+json`. Two small
improvements:

1. Do not attempt to write a body if the handler already wrote a status —
   wrap the `ResponseWriter` and track it, otherwise the 500 is appended to a
   partial 200 response.
2. Re-panic on `http.ErrAbortHandler`, which is the documented way to signal an
   intentional abort and should not be logged as a crash.

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

Current code registers `NotFound`/`MethodNotAllowed` before `Use`
(`server.go:58-66`). It works in chi today, but reversing it makes the dependency
obvious.

---

## Tests

```go
func TestCORS_DisallowedOriginGetsNoHeader(t *testing.T)  // no ACAO, and NOT a 403
func TestCORS_AlwaysSetsVaryOrigin(t *testing.T)
func TestCORS_WildcardWithCredentialsRejectedAtConfigLoad(t *testing.T)
func TestRealIP_IgnoresXFFWhenNoTrustedProxies(t *testing.T)
func TestSecurityHeadersPresent(t *testing.T)
func TestProtectedRouteRequiresToken(t *testing.T)        // guards against fail-open
```

---

## Related documents

- [Rate limiting](rate-limiting.md)
- [Token architecture](token-architecture.md)
- [Routing and versioning](../02-composition/routing-and-versioning.md)
- [Configuration model](../03-configuration/configuration-model.md)
