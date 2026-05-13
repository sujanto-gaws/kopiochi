# HTTP Server Package

Package `server` provides a production-ready HTTP server built on [chi](https://github.com/go-chi/chi) with graceful shutdown, structured logging, and a plugin lifecycle.

---

## Types

### `ShutdownFunc`

```go
type ShutdownFunc func() error
```

A cleanup callback executed during server shutdown. Use it to close resources such as database pools, cache connections, or file handles.

---

### `Server`

```go
type Server struct { ... }
```

Wraps `*http.Server` with:

| Field             | Type               | Purpose                                          |
|-------------------|--------------------|--------------------------------------------------|
| `httpServer`      | `*http.Server`     | Underlying Go HTTP server                        |
| `shutdownFuncs`   | `[]ShutdownFunc`   | Ordered list of cleanup callbacks                |
| `shutdownTimeout` | `time.Duration`    | Max time allowed for graceful shutdown           |
| `pluginRegistry`  | `*plugin.Registry` | Plugin registry closed during shutdown           |

---

### `ServerOption`

```go
type ServerOption func(*Server)
```

Functional option applied to a `Server` after construction.

| Option                        | Description                                          |
|-------------------------------|------------------------------------------------------|
| `WithShutdownFunc(fn)`        | Appends a `ShutdownFunc` to the cleanup chain        |
| `WithPluginRegistry(registry)`| Registers the plugin registry for lifecycle shutdown |

---

## Functions

### `NewRouter`

```go
func NewRouter(cfg config.Server, mw ...func(http.Handler) http.Handler) *chi.Mux
```

Creates a chi router with a fixed core middleware stack, followed by any caller-supplied middlewares.

**Core middleware stack (applied in order):**

| Middleware              | Purpose                                                     |
|-------------------------|-------------------------------------------------------------|
| `middleware.Recoverer`  | Recovers from panics and returns 500                        |
| `middleware.RequestID`  | Injects a unique `X-Request-Id` header per request          |
| `middleware.RealIP`     | Extracts real client IP from `X-Forwarded-For` / `X-Real-IP`|
| `middleware.Timeout`    | Cancels the request context after `cfg.RequestTimeout`      |
| `ZerologRequestLogger`  | Structured request/response logging via zerolog             |

**Parameters:**

| Parameter | Type            | Description                                       |
|-----------|-----------------|---------------------------------------------------|
| `cfg`     | `config.Server` | Server config; `RequestTimeout` controls chi timeout |
| `mw`      | variadic        | Optional middlewares appended after the core stack |

---

### `NewServer`

```go
func NewServer(cfg config.Server, router *chi.Mux, opts ...ServerOption) *Server
```

Constructs a `Server` from `config.Server`. All `http.Server` timeouts are sourced from `cfg`.

**`http.Server` timeout mapping:**

| `http.Server` field  | `config.Server` field  | Default | Purpose                                        |
|----------------------|------------------------|---------|------------------------------------------------|
| `ReadHeaderTimeout`  | `ReadHeaderTimeout`    | `10s`   | Guards against Slowloris header attacks        |
| `ReadTimeout`        | `ReadTimeout`          | `30s`   | Max time to read the full request              |
| `WriteTimeout`       | `WriteTimeout`         | `30s`   | Max time to write the full response            |
| `IdleTimeout`        | `IdleTimeout`          | `120s`  | Keep-alive idle connection timeout             |
| shutdown wait        | `ShutdownTimeout`      | `30s`   | Grace period for in-flight requests to finish  |

> `ReadHeaderTimeout` is intentionally shorter than `ReadTimeout` — it only needs to cover HTTP header delivery, not the request body.

---

### `Run` (package-level)

```go
func Run(cfg config.Server, router *chi.Mux, opts ...ServerOption)
```

Convenience wrapper: constructs a `Server` via `NewServer` and calls `srv.Run()`. Suitable for simple entry points.

---

### `(s *Server) Run`

```go
func (s *Server) Run()
```

Starts the HTTP server and blocks until `SIGINT` or `SIGTERM` is received.

**Shutdown sequence:**

```
SIGINT / SIGTERM received
        │
        ▼
signal.Stop(quit)              ← release signal channel
        │
        ▼
http.Server.Shutdown(ctx)      ← drain in-flight requests (ShutdownTimeout)
        │
        ▼
ShutdownFuncs[0..n]            ← run cleanup callbacks in registration order
        │
        ▼
plugin.Registry.Close()        ← close plugin registry (if set)
        │
        ▼
log "server exited properly"
```

All steps run even if earlier ones fail — errors are logged but do not abort the sequence.

---

### `(s *Server) Shutdown`

```go
func (s *Server) Shutdown(ctx context.Context) error
```

Programmatic shutdown for testing or embedded use. Mirrors the `Run` shutdown sequence.

- Runs all phases: HTTP shutdown → cleanup funcs → plugin registry.
- Collects **all** errors across every phase (does not stop on first failure).
- Returns a joined error via `errors.Join`, or `nil` if all succeeded.

---

### `NewPoolShutdownFunc`

```go
func NewPoolShutdownFunc(pool *pgxpool.Pool) ShutdownFunc
```

Factory that returns a `ShutdownFunc` which safely closes a `pgxpool.Pool`. Safe to call with a `nil` pool.

---

## Configuration

All server parameters come from `config.Server` (defined in `internal/config/config.go`):

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_header_timeout: "10s"
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  shutdown_timeout: "30s"
  request_timeout: "60s"
```

Values support Go duration strings (`"10s"`, `"1m30s"`, etc.) and can be overridden at runtime via environment variables with the `APP_` prefix and `.` → `_` substitution, e.g.:

```
APP_SERVER_READ_TIMEOUT=45s
APP_SERVER_SHUTDOWN_TIMEOUT=60s
```

---

## Usage Example

```go
// Build router — plugin middleware injected after core stack
r := server.NewRouter(cfg.Server,
    func(next http.Handler) http.Handler {
        return middlewareChain.Build(next)
    },
)

routes.Setup(r, authMiddleware, userHandler)

// Start — blocks until SIGINT/SIGTERM
server.Run(
    cfg.Server,
    r,
    server.WithShutdownFunc(server.NewPoolShutdownFunc(pool)),
    server.WithPluginRegistry(pluginRegistry),
)
```

---

## Security Notes

| Concern              | Mitigation                                                            |
|----------------------|-----------------------------------------------------------------------|
| Slowloris attacks    | `ReadHeaderTimeout: 10s` limits slow header delivery                 |
| Long-running bodies  | `ReadTimeout: 30s` caps total request read time                       |
| Response flooding    | `WriteTimeout: 30s` limits time to write responses                    |
| Goroutine leak       | `signal.Stop(quit)` releases the signal channel after receipt         |
| Partial shutdown     | `Shutdown` collects all errors; no phase is silently skipped          |
