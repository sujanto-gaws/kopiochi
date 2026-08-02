# HTTP Middleware Hardening

**Status:** Accepted — partially implemented
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2 — auth binding (Phase 1), CORS
(2.3, `87381d2`), proxy headers (2.2, `333968c`), and security response headers
(2.8, `0968aae`) are all resolved. Panic recovery, the 404/405 problem+json
handlers, and the `Recoverer`/`RequestID` ordering defect are **still open**.

Covers CORS, proxy header handling, security headers, auth binding, and the
middleware stack order. Rate limiting has its own document:
[rate-limiting.md](rate-limiting.md).

---

## CORS — fixed (Phase 2.3, `87381d2`)

All five problems below are resolved in
`internal/plugins/middleware/cors.go`, with `*`-plus-credentials rejected one
level up in `internal/config.Config.Validate`. Seven tests in
`internal/plugins/middleware/cors_test.go` cover them. The plugin's config
surface is still `map[string]interface{}` — converting it to typed config and
direct construction is Phase 3.5.

### Problem 1 (fixed) — permissive default

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

*Fixed. `Initialize` builds an empty allowlist and leaves `allowAll` false when
`allowed_origins` is absent, so the shipped `plugins.custom: {}` now means "no
origin is ever granted access". A wildcard is only ever set when `"*"` appears
explicitly in the list. `config/default.yaml` documents how to opt in.
Test: `TestCORS_DefaultDenyAllowsNoOrigin`.*

### Problem 2 (fixed) — arbitrary origin reflection

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

*Fixed, on both levels. The `Origin` header is echoed back **only** after the
exact value has been found in the allowlist (`cors.go:143-149`); an unlisted
origin gets no header at all. The wildcard branch emits a literal `*` and never
`Access-Control-Allow-Credentials`, as defence in depth. The dangerous
combination itself is now unrepresentable: `Config.Validate` rejects `"*"` plus
`allow_credentials: true` at config load, so the process refuses to start
(`config.go:248-255`). Tests: `TestCORS_NeverReflectsArbitraryOrigin`,
`TestCORS_ExplicitWildcardNeverSetsCredentials`,
`TestLoad_RejectsWildcardCORSOriginWithCredentials`.*

### Problem 3 (fixed) — no `Vary: Origin`

The response varies by request `Origin`, but no `Vary` header is emitted. Any
shared cache or CDN will store one origin's `Access-Control-Allow-Origin` and
serve it to a different origin — cache poisoning that produces both false
rejections and unintended grants.

*Fixed. `w.Header().Add("Vary", "Origin")` is the first thing the middleware
does, before it has even decided whether this is a CORS request — so it is
present on allowed, denied, and no-`Origin` responses alike. Preflights add
`Vary: Access-Control-Request-Method` and `Vary: Access-Control-Request-Headers`
on top. Test: `TestCORS_AlwaysSetsVaryOrigin`.*

### Problem 4 (fixed) — non-browser requests rejected with 403

`cors.go:96-100` aborts with 403 whenever an `Origin` header is present and not
allowed. CORS is a *browser* enforcement mechanism; the server's job is to
withhold the header, not to reject the request. Any client that happens to send
`Origin` (some proxies, some SDKs) is broken for no security gain — the browser
would have blocked the read anyway.

*Fixed. There is no 403 path left. An `Origin` that is not allowed simply gets
no `Access-Control-Allow-Origin` header and the request continues to the router.
A request with no `Origin` at all is not a CORS request and passes through
untouched — not even the preflight branch is considered. Tests:
`TestCORS_DisallowedOriginGetsNoHeaderAndIsNot403`,
`TestCORS_NoOriginRequestPassesThroughUntouched`.*

### Problem 5 (fixed) — preflight terminates the chain early

`cors.go:118-121` answers `OPTIONS` with 204 for **every** path, including ones
that do not exist. This leaks nothing serious but makes 404 behaviour inconsistent
and bypasses downstream middleware.

*Fixed. Only an actual preflight — `OPTIONS` **and** a non-empty
`Access-Control-Request-Method` — gets the 204. Every other `OPTIONS` request
falls through to the router like any other method, so 404/405 behaviour stays
consistent. Test: `TestCORS_OnlyActualPreflightGetsNoContent`.*

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

*Shipped in `87381d2`, against the plugin's `map[string]interface{}` surface
rather than a typed `config.CORS`, so the sketch above is the shape it takes at
Phase 3.5, not the shape in the tree. Two of the sketched validations did **not**
ship: the scheme check (`origin %q must include a scheme`) and the
"enabled with an empty allowlist" error. The first is a config-hygiene check with
no security consequence — an origin without a scheme can never match a real
browser `Origin` header, so it fails closed. The second does not apply while
"enabled" means "listed in `plugins.middleware`": an empty allowlist is the
shipped default and is the safe state, not an error. Only the `*`-plus-
credentials rejection was security-load-bearing, and that shipped.*

The effective defaults are as described: empty allowlist, credentials off, and
the middleware only runs at all when `ratelimit`/`cors` are listed under
`plugins.middleware`.

---

## Proxy headers / client IP — fixed (Phase 2.2, `333968c`)

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

*Shipped as `internal/middleware/clientip.go` (`RealIP`, `ClientIP`,
`ParseTrustedProxies`) — the package the request logger already lives in, not
`internal/httpx`. Both disagreeing consumers are gone: `server.NewRouter` now
registers `zlog.RealIP(zlog.ParseTrustedProxies(cfg.TrustedProxies))` in place of
chi's `middleware.RealIP` (`server.go:71`), and the rate limiter reads
`corenet.ClientIP(r.Context())` instead of parsing `X-Forwarded-For` itself. The
zerolog request logger logs the same resolved value.*

*`server.trusted_proxies` is a CIDR list defaulting to `[]`. Forwarded headers
are consulted **only** when the immediate TCP peer falls inside one of those
ranges; `X-Forwarded-For` is then walked right-to-left, skipping trusted hops, to
find the first untrusted address, with `X-Real-Ip` as a fallback. An empty, nil,
or entirely-invalid list all mean the same thing — trust nothing, use the socket
address. Invalid CIDR entries are logged and skipped rather than aborting
startup; full config validation of this field is still outstanding.*

*Six tests in `internal/middleware/clientip_test.go` cover it, including
`TestRealIP_IgnoresForwardedHeaderFromUntrustedPeerEvenIfProxiesConfigured` —
the case where proxies are configured but the caller is not one of them — and
`TestRealIP_WalksMultipleTrustedHopsToFindRealClient`.*

---

## Security response headers — added (Phase 2.8, `0968aae`)

None were set. `internal/httpx/security_headers.go` now sets them on every
response, registered in `server.NewRouter` (`server.go:64`):

| Header | Value | Shipped |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | ✅ |
| `X-Frame-Options` | `DENY` | ✅ |
| `Referrer-Policy` | `no-referrer` | ✅ |
| `Content-Security-Policy` | `default-src 'none'`, relaxed on `/swagger/*` | ✅ |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | ✅ gated on `server.enable_hsts`, **default false** |
| `Cache-Control: no-store` on authenticated responses | — | ⏳ not shipped; it belongs on the handlers that emit tokens/PII, not on a blanket middleware |

Two details worth recording:

- **The CSP is route-scoped, not global.** `default-src 'none'` is right for a
  JSON API and fatal for the bundled Swagger UI, whose `index.html` uses an
  inline `<style>` and an inline `<script>` to boot `SwaggerUIBundle`. Rather
  than loosening the policy for every response, `/swagger/` gets its own
  `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'
  'unsafe-inline'; img-src 'self' data:` — everything that page loads is
  same-origin, so nothing there reaches a third party.
  `TestSecurityHeaders_SwaggerCSPIsRelaxed` asserts it against the real mounted
  route.
- **HSTS must stay off by default.** This server always listens plain HTTP, so
  `enable_hsts` is only correct where TLS is terminated in front of it. Emitting
  it unconditionally would, at best, be ignored and, at worst, be cached by a
  browser against `http://localhost` and lock a developer out of it.
  `TestSecurityHeaders_HSTSGatedByConfig` covers both settings.

Because the middleware is registered with `chi.Mux.Use`, it wraps the router's
own not-found handling too — `TestSecurityHeadersPresent_On404` proves the
headers are present on responses no handler produced.

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

With `plugins.auth.jwt.enabled: false` — the shipped default at the time — every
protected route was served unauthenticated, with no warning logged. *That
configuration key no longer exists: Phase 2.6 (`0cf07d9`) deleted the HS256
plugin along with its `plugins.auth.jwt` block, so `config/default.yaml` now
ships `plugins.auth: {}` and there is nothing left to accidentally disable.*

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

The defects in what the middleware then *did* with a token — no `iss`/`aud`
validation, one `Validate()` for three token classes — are **also fixed now**, in
Phase 2.4/2.5 (`e0da81e`, `946c1c8`). `modules/identity/transport/middleware.go`
calls `Validate(tokenStr, domain.ClassAccess)`, so an MFA token presented to an
API route is rejected at the validation boundary rather than by a caller
remembering to inspect `scope`. See [token architecture](token-architecture.md).

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

### Where the stack actually stands after Phase 2

`server.NewRouter` (`internal/infrastructure/http/server/server.go:54-81`) now
registers, in order:

```go
r.Use(middleware.Recoverer)                                  // chi's, still first
r.Use(middleware.RequestID)
r.Use(httpx.SecurityHeaders(...{EnableHSTS: cfg.EnableHSTS})) // 2.8
r.Use(zlog.RealIP(zlog.ParseTrustedProxies(cfg.TrustedProxies)))  // 2.2
r.Use(middleware.Timeout(cfg.RequestTimeout))
r.Use(zlog.ZerologRequestLogger)
// then, from main.go: the plugin middleware chain — cors, ratelimit
```

Two ordering constraints from the target are satisfied: `RealIP` precedes
everything that keys or logs on the client IP (the request logger and, via the
plugin chain, the rate limiter), and `CORS` precedes `RateLimit` within the
plugin chain because `plugins.middleware` lists them in that order.

Two are **not**, and both are unchanged from before Phase 2 because nothing in
Phase 2 touched them:

- `Recoverer` is still registered before `RequestID`, so a recovered panic still
  has no request ID to log. Fixing this is bound up with replacing `Recoverer`
  with `httpx.Recovery(log)`, which has not been written.
- `SecurityHeaders` is registered before `RealIP` rather than after the logger.
  That is harmless — it only writes response headers and depends on nothing —
  but it is not the order sketched above.

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

Sketched here, and where each one ended up after Phase 2:

| Sketched | State |
|---|---|
| `TestCORS_DisallowedOriginGetsNoHeader` | ✅ `TestCORS_DisallowedOriginGetsNoHeaderAndIsNot403` — `cors_test.go` |
| `TestCORS_AlwaysSetsVaryOrigin` | ✅ same name — `cors_test.go` |
| `TestCORS_WildcardWithCredentialsRejectedAtConfigLoad` | ✅ `TestLoad_RejectsWildcardCORSOriginWithCredentials` — `internal/config/config_test.go`, since the rejection lives in `Config.Validate` |
| `TestRealIP_IgnoresXFFWhenNoTrustedProxies` | ✅ `TestRealIP_IgnoresForwardedHeadersWithoutTrustedProxies` — `internal/middleware/clientip_test.go` |
| `TestSecurityHeadersPresent` | ✅ same name — `internal/httpx/security_headers_test.go` |
| `TestProtectedRouteRequiresToken` | ⏳ not written. The fail-open path it guards is gone by construction (Phase 1), and `TestValidate_RejectsMFATokenAsAccessToken` covers the escalation case at the unit level, but no test drives a protected route without a token |
| `TestRecovery_EmitsProblemJSONWithRequestID` | ⏳ not written — `httpx.Recovery` does not exist |
| `TestNotFound_EmitsProblemJSON` | ⏳ not written — the handler does not exist |

Beyond the sketch, `cors_test.go` adds `TestCORS_NeverReflectsArbitraryOrigin`,
`TestCORS_NoOriginRequestPassesThroughUntouched`,
`TestCORS_OnlyActualPreflightGetsNoContent`,
`TestCORS_DefaultDenyAllowsNoOrigin`, and
`TestCORS_ExplicitWildcardNeverSetsCredentials`; `security_headers_test.go` adds
the 404, HSTS-gating, and Swagger-CSP cases; `clientip_test.go` adds the
trusted-peer, untrusted-peer, multi-hop, and CIDR-parsing cases.

None of these run under `-race` in this environment — see
[testing strategy](../06-quality/testing-strategy.md#race-detection-is-outstanding).

---

## Related documents

- [Rate limiting](rate-limiting.md)
- [Token architecture](token-architecture.md)
- [Routing and versioning](../02-composition/routing-and-versioning.md)
- [Configuration model](../03-configuration/configuration-model.md)
