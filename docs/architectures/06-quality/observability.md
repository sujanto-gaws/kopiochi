# Observability

**Status:** Proposed
**Date:** 2026-08-02

---

## What is already correct

- `internal/logger/logger.go` initialises zerolog with configurable level and
  format (`json` for production, console for development).
- `internal/middleware/zerolog.go` provides structured request logging.
- `internal/middleware/recovery.go` logs panics with request ID, method, path,
  and stack trace, then returns RFC 7807 `application/problem+json`.
- `handlers.NotFound()` / `MethodNotAllowed()` return the same problem shape, so
  error responses are consistent.

The foundation is sound. The gaps below are about coverage and discipline, not
about replacing it.

---

## Problem 1: the global logger is used directly

`main.go:54` assigns `log.Logger = logger.Init(...)`, and packages then call
`log.Info()` on zerolog's package-level global:

```go
// internal/infrastructure/http/server/server.go
import "github.com/rs/zerolog/log"
log.Info().Str("addr", addr).Msg("starting http server")
```

Consequences: no per-request logger with bound fields, no way to inject a test
logger, and no component scoping. Tests that assert on log output are impossible
without capturing a global.

**Target:** pass `zerolog.Logger` explicitly through constructors — it is already
part of `module.Deps`. Derive child loggers with bound context:

```go
log := deps.Logger.With().Str("component", "identity").Logger()
```

## Problem 2: the extension manager prints to stdout

`internal/extension/manager.go:294-311`:

```go
func (l *defaultLogger) Info(msg string, keysAndValues ...interface{}) {
    fmt.Printf("[INFO] %s %v\n", msg, keysAndValues)
}
```

Unstructured `fmt.Printf` bypassing zerolog entirely — inconsistent format, no
level filtering, no JSON. This disappears when the extension framework is deleted
(see [extension framework](../01-modularity/extension-framework.md)), but no new
code may reintroduce the pattern.

## Problem 3: request-scoped context is not propagated

`RequestID` is generated (`server.go:63`) and used by the recovery middleware,
but it is not attached to a logger placed in the request context. Handlers and
services therefore cannot emit correlated logs, and a downstream database error
cannot be tied to the request that caused it.

## Problem 4: no metrics at all

No Prometheus endpoint, no counters, no histograms. There is no way to observe:

- request rate, error rate, latency distribution
- database pool saturation (`pgxpool.Stat()` is available but unexported to
  anything)
- rate-limit rejections
- authentication failures — the signal that matters most for detecting attacks

## Problem 5: no tracing

Multi-step flows (login → token issue → refresh rotation) cannot be followed
across layers. `context.Context` is already threaded correctly, so adding
OpenTelemetry is largely mechanical.

## Problem 6: nothing prevents secrets reaching the logs

`Config.DB.Password` is a plain `string`. A single `log.Debug().Interface("cfg",
cfg)` during troubleshooting prints the database password into the log stream.
The `secret.String` type proposed in
[secret management](../03-configuration/secret-management.md) closes this.

---

## Target design

### Logging

**Levels**

| Level | Use |
|---|---|
| `error` | Operator action required; always with `err` |
| `warn` | Degraded but handled — rate limit hit, auth failure, retry |
| `info` | Lifecycle events — startup, shutdown, module registration, migrations |
| `debug` | Development detail — SQL, decision branches |
| `trace` | Off outside local debugging |

**Standard fields**

| Field | Where |
|---|---|
| `request_id` | Every request-scoped line |
| `component` | Every line, from the child logger |
| `method`, `path`, `status`, `duration_ms` | Access log |
| `client_ip` | Access log — the *resolved* IP, never raw XFF |
| `user_id` | When authenticated |
| `err` | Every error line, via `.Err(err)` |

**Request-scoped logger in context**

```go
func RequestLogger(base zerolog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            reqLog := base.With().
                Str("request_id", middleware.GetReqID(r.Context())).
                Str("client_ip", ClientIPFrom(r.Context())).
                Logger()

            ctx := reqLog.WithContext(r.Context())
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

            next.ServeHTTP(ww, r.WithContext(ctx))

            evt := reqLog.Info()
            if ww.Status() >= 500 {
                evt = reqLog.Error()
            } else if ww.Status() >= 400 {
                evt = reqLog.Warn()
            }
            evt.Str("method", r.Method).
                Str("path", r.URL.Path).
                Int("status", ww.Status()).
                Int("bytes", ww.BytesWritten()).
                Dur("duration", time.Since(start)).
                Msg("request completed")
        })
    }
}
```

Any layer then does `zerolog.Ctx(ctx).Info()` and gets correlation for free.

**What must never be logged:** passwords, tokens (access, refresh, or MFA), token
hashes, private keys, full `Authorization` headers, session cookies, or complete
request bodies on auth endpoints. Log a token's `jti`, never the token.

**Log at boundaries** — per `CLAUDE.md`. Errors are wrapped with `%w` as they
propagate and logged **once**, at the HTTP boundary. Logging at every level
produces one incident as five unrelated-looking error lines.

### Metrics

Prometheus on a separate port (not the public API):

```go
var (
    httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kopiochi_http_requests_total",
    }, []string{"method", "route", "status"})

    httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "kopiochi_http_request_duration_seconds",
        Buckets: prometheus.DefBuckets,
    }, []string{"method", "route"})

    authFailures = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kopiochi_auth_failures_total",
    }, []string{"reason"})     // bad_password, expired_token, wrong_class, locked

    rateLimitRejections = promauto.NewCounter(prometheus.CounterOpts{
        Name: "kopiochi_rate_limit_rejections_total",
    })
)
```

Label with the **chi route pattern** (`/api/v1/users/{id}`), never the raw path —
raw paths create unbounded cardinality.

Database pool metrics from `pool.Stat()`, collected periodically:
`acquired_conns`, `idle_conns`, `total_conns`, `acquire_duration`. Pool
saturation is the usual first symptom of the connection-churn problem described
in [persistence and pooling](../05-data/persistence-and-pooling.md).

### Tracing

OpenTelemetry with spans at HTTP handler, application use case, and repository
boundaries. Propagate W3C `traceparent`. Include `trace_id` in every log line so
logs and traces cross-reference. Sample at ~1% in production, 100% locally.

### Error responses

Keep RFC 7807 — already implemented. Standardise the `type` values:

```json
{
  "type": "validation_error",
  "title": "Validation Failed",
  "status": 400,
  "detail": "email must be a valid address",
  "instance": "/api/v1/auth/register",
  "request_id": "a1b2c3d4"
}
```

Adding `request_id` to the response body lets a user quote it in a support
request and have it match a log line directly.

Internal errors never leak: the recovery middleware's fixed "An unexpected error
occurred." is correct. Do not include the panic value or stack in the response.

### Audit events

Security-relevant events go to a separate, structured, retained stream — not
mixed with request logs:

| Event | Fields |
|---|---|
| `auth.login.success` / `auth.login.failure` | user_id (or attempted email), client_ip, reason |
| `auth.token.refresh_reuse_detected` | user_id, token family — high severity |
| `auth.mfa.enrolled` / `auth.mfa.failed` | user_id |
| `user.role.granted` / `revoked` | actor_id, target_id, role |
| `user.deleted` | actor_id, target_id |

The domain entities already carry `CreatedBy`/`UpdatedBy`/`DeletedBy`, so the
actor is available.

---

## Sequencing

1. Replace global logger use with injected loggers (mechanical, low risk).
2. Add the request-scoped logger to context; adopt `zerolog.Ctx(ctx)` in handlers.
3. Add `secret.String` so config can never be logged in the clear.
4. Add Prometheus metrics + `/metrics` on the admin port.
5. Add pool metrics.
6. Add audit events with the identity module.
7. Add OpenTelemetry once the module structure has settled.

---

## Related documents

- [Testing strategy](testing-strategy.md)
- [Secret management](../03-configuration/secret-management.md)
- [Lifecycle and shutdown](../02-composition/lifecycle-and-shutdown.md)
- [Rate limiting](../04-security/rate-limiting.md)
