# HTTP Server & Router

> **Moved in Phase 3.** This used to document
> `internal/infrastructure/http/server`. That package is gone, along with the
> whole of `internal/infrastructure/`: the server now lives in
> `internal/httpx/server.go`, next to the router, the route tree and the
> middleware it belongs with. Resource teardown moved out of the server
> entirely, into `internal/lifecycle`.

`internal/httpx` owns everything HTTP that is not a business module:

| File | Responsibility |
|---|---|
| `router.go` | `NewRouter` — core middleware stack, plus CORS, rate limiting and metrics when enabled |
| `server.go` | `Server` — listen, serve, drain |
| `routes.go` | `Mount` — operational endpoints and the `/api/v1` group |
| `health.go` | `/healthz` liveness, `/readyz` readiness |
| `cors.go`, `ratelimit.go`, `security_headers.go`, `recovery.go` | the middlewares themselves |
| `problem.go` | RFC 7807 responses, the 404/405 handlers, `EchoRequestID` |

---

## `NewRouter`

```go
func NewRouter(
    srv config.Server,
    sec config.Security,
    log zerolog.Logger,
    m *metrics.Metrics,          // nil disables instrumentation entirely
    mw ...func(http.Handler) http.Handler,
) (*chi.Mux, func() error, error)
```

It also registers `NotFound` and `MethodNotAllowed`, so the router's own
errors are problem+json like everything else.

The core middleware stack, in order:

1. `chi.RequestID` — **first**, ahead of recovery. It used to run second, which meant a recovered panic could not carry a request id even in principle: none had been generated yet
2. `EchoRequestID` — returns it in `X-Request-Id`. chi only puts the id in the request *context*, so without this a client with a successful response has nothing to quote when reporting it later
3. `Recovery(log)` — replaces `chi.Recoverer`. problem+json instead of an empty body, and a structured correlated log line instead of a plain-text stack dump on stderr
4. `SecurityHeaders` — cheap, and applies to everything downstream produces, including 404s, 405s and recovered panics
5. `middleware.RealIP` — resolves the client IP from **trusted proxies only** and puts it in the request context. Empty `trusted_proxies` (the default) means trust nothing and use the socket address. Must run before anything that keys or logs on client IP
6. `chi.Timeout(request_timeout)`
7. `middleware.RequestLogger(log)` — one line per request, and a request-scoped logger in the context for every layer below
8. `metrics.Middleware`, **if metrics are enabled**
9. `CORS`, **if `security.cors.enabled`**
10. `RateLimit`, **if `security.rate_limit.enabled`**
11. any caller-supplied middleware

CORS is registered *before* the rate limiter deliberately: a throttled
cross-origin request must still carry `Access-Control-Allow-Origin`, or the
429 reaches the browser as an opaque network error and the caller never sees
the status it is supposed to back off on. The cost is that a preflight
short-circuits before the limiter and does not consume budget — a few header
writes and no I/O, which is the cheaper side of the trade.

Metrics sit above both for the same reason in reverse: a limiter that
short-circuits *below* the instrumentation makes rejected traffic invisible,
which is exactly the traffic worth seeing. The consequence is that a
short-circuited request never reaches the mux and so has no route pattern; it
is labelled `unmatched`, and the status label still distinguishes it.

**The second return value is a closer.** It releases what the router
constructed that owns a resource — today, the rate limiter's eviction
goroutine. Push it onto the lifecycle stack; it is safe to call when nothing
was constructed.

There is no plugin middleware chain. `plugin.NewMiddlewareChainFromRegistry`
and the registry behind it were deleted in Phase 3; CORS and rate limiting are
constructed directly from typed config. See [PLUGIN_GUIDE.md](PLUGIN_GUIDE.md).

---

## `Server`

```go
func NewServer(cfg config.Server, handler http.Handler, log zerolog.Logger) *Server

func (s *Server) Serve(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
func (s *Server) Draining() bool
```

**`Serve` blocks until `ctx` is cancelled or the listener fails, and returns
the error either way.** It does not call `log.Fatal`. The implementation this
replaced did, inside the serving goroutine — so a port-in-use error called
`os.Exit(1)`: no deferred function ran, the pool was never drained, no
shutdown func fired, and `main` never learned why. `Run` also returned nothing,
so `RunE` returned `nil` and the process exited 0 on a server that never
started.

A cancelled context is a clean stop and returns `nil`; only a real listen
failure is an error. Startup failures now reach `RunE` and give the process a
non-zero exit code, which is what supervisors, CI and container restart
policies read.

**`Serve` does not shut the server down.** That is `Shutdown`, registered once
on the lifecycle stack. Doing it in both places is the double-ownership the
stack exists to remove.

**`Shutdown` flips readiness before draining.** `Draining()` feeds
`httpx.Deps.Draining`, and `/readyz` checks it ahead of the database ping — so
a load balancer removes the instance *while* in-flight requests finish, rather
than after. A reachable database says nothing about whether this process
should still receive traffic.

---

## Teardown: `internal/lifecycle`

```go
stack := lifecycle.New(log)
stack.PushCloser("database", database.Close)
stack.PushCloser("router", closeRouter)
stack.Push("http server", srv.Shutdown)
...
err := stack.Shutdown(shutdownCtx)   // releases in reverse order
```

Rules:

- A resource is pushed **exactly once**, by whoever created it.
- No `defer x.Close()` in `main` for anything on the stack.
- Teardown is strict LIFO — the reverse of construction. Each resource is
  released before the ones it was built on top of.
- A closer that fails does not stop the ones beneath it from running. Errors
  are logged per-resource and joined into one.

`cmd/api/serve()` runs the whole lifecycle linearly and returns errors at every
phase; see "How startup is wired" in [README.md](README.md).

A second SIGINT/SIGTERM during a drain forces `exit(130)`, so an operator
aborting a stuck shutdown does not have to reach for `SIGKILL`.

---

## Configuration

All server parameters come from `config.Server` (`internal/config/config.go`):

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_header_timeout: "10s"
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  shutdown_timeout: "30s"
  request_timeout: "25s"     # must not exceed write_timeout — validated at boot
  trusted_proxies: []        # CIDRs whose X-Forwarded-For is honoured
  enable_hsts: false
```

`Config.Validate` rejects `request_timeout > write_timeout` and
`shutdown_timeout < request_timeout` at startup, so a request allowed to run
for longer than the write deadline cannot have its connection severed
mid-response. Values are Go duration strings and can be overridden with the
`APP_` prefix and `.` → `_`:

```
APP_SERVER_READ_TIMEOUT=45s
APP_SERVER_SHUTDOWN_TIMEOUT=60s
```

---

## Route mounting

`NewRouter` knows nothing about application routes. The caller hands the
`*chi.Mux` to `Mount`:

```go
func Mount(r *chi.Mux, modules []*module.Module, deps Deps)

type Deps struct {
    Pinger   Pinger        // /readyz pings this; nil = not ready (fail closed)
    Draining func() bool   // nil = no shutdown signal to report
}
```

`Mount` registers the unversioned operational endpoints — `GET /healthz`
(liveness, never touches a dependency), `GET /readyz` (draining check, then a
short-timeout ping), `GET /health` (deprecated alias) and `GET /swagger/*` —
then opens a **single** `/api/v1` group and calls every module's
`Routes(chi.Router)` against it. Only one router is ever in scope for that
group, so routes cannot silently register on the wrong one — the bug that
mounted every module at the root instead of under `/api/v1`.

> The former `routes.Setup` and the `internal/infrastructure/http/routes`
> package no longer exist; `handlers.RouterGroup` and `handlers.RouteRegistrar`
> were removed with them. Each module declares its own routes and its own auth
> middleware, so a module cannot be mounted unprotected by accident.

---

## Security notes

| Concern | Mitigation |
|---|---|
| Slowloris | `read_header_timeout: 10s` caps slow header delivery |
| Long-running bodies | `read_timeout: 30s` caps total request read time |
| Response flooding | `write_timeout: 30s` caps time to write responses |
| Spoofed client IP | `RealIP` honours forwarded headers only from configured trusted-proxy CIDRs; empty means trust nothing |
| Request flooding | Token-bucket rate limiter keyed on the *resolved* IP, with TTL eviction and a `max_keys` cap |
| Cross-origin access | Allowlist-only CORS; `"*"` with credentials is rejected at config load |
| Partial shutdown | The lifecycle stack collects every error; no resource is silently skipped |
| Stuck drain | A second signal forces `exit(130)` |

---

## Error responses

Every error this service emits is RFC 7807 `application/problem+json`, produced
by `httpx.WriteProblem`:

```json
{
  "type": "not_found",
  "title": "Not Found",
  "status": 404,
  "detail": "No route matches this path.",
  "instance": "/api/v1/nope",
  "request_id": "kopiochi-000123"
}
```

`request_id` is an extension member, not part of RFC 7807. It is there so a
user can quote the id from a failed response and have it match a log line
exactly, rather than anyone guessing from timestamps. The same id is on every
response in the `X-Request-Id` header, including successful ones.

`detail` is shown to the client, so it never carries internal state — no panic
values, no stack traces, no driver errors. Anything an operator needs is in the
log line under the same `request_id`.

**404 and 405 use this shape too.** chi's defaults are `text/plain`, which
meant a client that content-negotiated JSON got an unparseable body for the two
statuses it is most likely to hit while integrating.

> **Registering a custom 405 handler with chi silently drops the `Allow`
> header.** `Mux.MethodNotAllowedHandler` returns a registered handler
> *instead of* chi's default, and only the default sets `Allow` — which
> RFC 9110 requires a 405 to carry, and which is the only thing telling a
> client what to try instead. `httpx.MethodNotAllowed` therefore takes the
> router and recomputes the set by probing the routing tree, because chi keeps
> it in an unexported context field.

---

## Metrics

Off by default. When `metrics.enabled` is set, a second `httpx.Server` listens
on `metrics.addr` and serves `metrics.path`:

```yaml
metrics:
  enabled: false
  addr: "127.0.0.1:9090"   # loopback by default
  path: "/metrics"
```

It is a separate listener on purpose. `/metrics` exposes the route table,
latency distributions, database pool saturation and the process's memory and
file-descriptor counts — an inventory of the service's internals with no
business being reachable from the internet. `Config.Validate` **rejects a
`metrics.addr` equal to the API's own address**, because serving it on the
public listener is the one outcome a separate admin port exists to prevent.

The admin server goes on the same lifecycle stack, pushed *after* the API so
LIFO stops it first — a scrape landing mid-drain would otherwise report a
process that is already going away. A failure to bind it is logged, not fatal:
losing observability is bad, losing the service is worse.

Requests are labelled with the chi **route pattern** (`/api/v1/users/{id}`),
never the raw path. A raw path makes one time series per distinct URL, so a
crawler walking `/api/v1/users/1` … `/9999999` creates millions of series and
takes the Prometheus server down — an availability incident caused by the
monitoring, from traffic nobody had to authenticate to send. Unmatched paths
collapse to a single `unmatched` label for the same reason.

Database pool statistics are collected at scrape time rather than on a ticker,
so there is no goroutine to own and no staleness in a saturation signal.
`kopiochi_db_pool_empty_acquires_total` is the one to alert on: a nonzero rate
means requests are queueing for a connection, which is saturation showing up
before latency does.
