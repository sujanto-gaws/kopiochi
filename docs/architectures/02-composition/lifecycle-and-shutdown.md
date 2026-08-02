# Application Lifecycle & Shutdown

**Status:** Proposed
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1 — every problem below is still live
(Phase 3.9 has not run). Line references updated for the current tree.

---

## Problem

### 1. Every resource is closed twice

`cmd/api/main.go`:

```go
defer pluginRegistry.Close()                      // line 63
defer pool.Close()                                // line 76
...
server.Run(cfg.Server, r,
    server.WithShutdownFunc(server.NewPoolShutdownFunc(pool)),   // closes pool again
    server.WithPluginRegistry(pluginRegistry),                   // closes registry again
)
```

`server.Shutdown` (`server.go:133-153`) invokes the shutdown funcs and
`pluginRegistry.Close()`, then `main`'s deferred calls fire on return.

`pgxpool.Close` and the registry's `Close` are effectively idempotent today, so
nothing breaks — but ownership is genuinely ambiguous, and the next resource
added (a Kafka producer, a cache client, a file handle) will not be so forgiving.

Note that `Registry.Close` deletes entries as it goes
(`registry.go:135-150`), so the second call is a no-op — the safety is
accidental, not designed.

### 2. `log.Fatal` inside the serving goroutine bypasses shutdown

`server.go:110-114`:

```go
go func() {
    if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatal().Err(err).Msg("server failed to start")
    }
}()
```

`log.Fatal` calls `os.Exit(1)`. Deferred functions do not run, the pool is not
drained, and shutdown funcs never fire. A port-in-use error at startup therefore
exits without cleanup — and the parent `main` never learns why.

### 3. `Run` swallows the error

`Run` returns nothing (`server.go:95`), so `main`'s `RunE` returns `nil`
even when the server failed. The process exit code does not reflect the failure —
which matters for supervisors, CI, and container restart policies.

### 4. Startup order does not match teardown order

Plugins initialise (`main.go:56`) *before* the database connects (`main.go:68`). Teardown
runs in an order determined by a mix of `defer` and the shutdown-func slice.
There is no single place expressing "started A then B, so stop B then A".

### 5. Signal handling is incomplete

`signal.Notify` covers SIGINT and SIGTERM (`server.go:107`), which is correct.
But the shutdown context is created *after* the signal arrives and is not tied to
a base context — so nothing else in the process can observe "we are shutting
down". A second signal during a slow drain is ignored rather than forcing exit.

---

## Target design

### One owner per resource, one teardown stack

```go
// internal/lifecycle/stack.go
package lifecycle

type Stack struct {
    entries []entry
    log     zerolog.Logger
}

type entry struct {
    name  string
    close func(context.Context) error
}

// Push registers a closer. Teardown runs in reverse registration order.
func (s *Stack) Push(name string, fn func(context.Context) error) {
    s.entries = append(s.entries, entry{name: name, close: fn})
}

func (s *Stack) Shutdown(ctx context.Context) error {
    var errs []error
    for i := len(s.entries) - 1; i >= 0; i-- {
        e := s.entries[i]
        s.log.Info().Str("resource", e.name).Msg("closing")
        if err := e.close(ctx); err != nil {
            errs = append(errs, fmt.Errorf("%s: %w", e.name, err))
        }
    }
    return errors.Join(errs...)
}
```

Rules:

- A resource is registered on the stack **exactly once**, by whoever created it.
- No `defer x.Close()` in `main` for anything on the stack.
- Teardown is strict LIFO: the reverse of construction.

### `main` becomes linear and returns errors

```go
func run(cmd *cobra.Command) error {
    cfg, err := config.Load(cfgPath)
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }

    log := logger.Init(cfg.Log.Level, cfg.Log.Format)
    stack := lifecycle.New(log)

    // Base context cancelled on the first signal — observable process-wide.
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    bunDB, pool, err := db.New(ctx, cfg.DB)
    if err != nil {
        return fmt.Errorf("init database: %w", err)
    }
    stack.Push("database pool", func(context.Context) error { pool.Close(); return nil })

    app, err := BuildApp(cfg, bunDB, log)
    if err != nil {
        return fmt.Errorf("build application: %w", err)
    }
    for _, m := range app.Modules {
        if m.Close != nil {
            stack.Push("module "+m.Name, func(context.Context) error { return m.Close() })
        }
    }

    r := httpx.NewRouter(cfg.Server, cfg.Security, log)
    httpx.Mount(r, app.Modules, deps)

    srv := server.New(cfg.Server, r, log)
    stack.Push("http server", srv.Shutdown)

    // Blocks until ctx is cancelled or the listener fails.
    if err := srv.Serve(ctx); err != nil {
        log.Error().Err(err).Msg("server stopped with error")
    }

    shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
    defer cancel()
    return stack.Shutdown(shutdownCtx)
}
```

### `Serve` returns instead of exiting

```go
func (s *Server) Serve(ctx context.Context) error {
    errCh := make(chan error, 1)
    go func() {
        if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err            // no log.Fatal — the error travels to the caller
            return
        }
        errCh <- nil
    }()

    select {
    case err := <-errCh:
        return err                  // failed to bind, etc.
    case <-ctx.Done():
        s.log.Info().Msg("shutdown signal received")
        return nil
    }
}
```

Startup failures now propagate to `RunE`, which sets a non-zero exit code.

### Second signal forces exit

```go
go func() {
    <-ctx.Done()                    // first signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh                         // second signal
    log.Warn().Msg("second signal received, forcing exit")
    os.Exit(130)
}()
```

Without this, an operator who wants to abort a stuck 30-second drain has to reach
for `SIGKILL`.

### Readiness reflects shutdown

On the first signal, flip readiness to false **before** draining, so the load
balancer stops sending new requests while in-flight ones finish:

```go
r.Get("/readyz", handlers.Ready(pool, &shuttingDown))
```

---

## Startup order

| Phase | Action | Failure behaviour |
|---|---|---|
| 1 | Load + validate config | Return error, exit non-zero |
| 2 | Initialise logger | Return error |
| 3 | Connect database, `Ping` | Return error |
| 4 | `BuildApp` — construct modules | Return error (includes the empty-module guard) |
| 5 | Build router, mount routes | Return error |
| 6 | Listen | Return error |
| 7 | Serve until signal | — |

Teardown is exactly the reverse via the LIFO stack.

Note that phase 3 precedes phase 4 in the target, whereas today plugins
initialise before the database. Modules may need the DB during construction (to
prepare statements or verify schema), so the database must exist first.

---

## Timeouts

`config/default.yaml` sets sensible values already; keep them and make the
relationship explicit:

| Setting | Value | Constraint |
|---|---|---|
| `read_header_timeout` | 10s | Guards against Slowloris |
| `read_timeout` | 30s | ≥ header timeout |
| `write_timeout` | 30s | ≥ `request_timeout` or long responses are cut off |
| `request_timeout` | 60s | ⚠️ currently **exceeds** `write_timeout` (30s) |
| `idle_timeout` | 120s | Keep-alive reuse |
| `shutdown_timeout` | 30s | ≥ `request_timeout` for a clean drain |

The `request_timeout` (60s) > `write_timeout` (30s) inversion means a request
allowed to run for 60 seconds has its connection torn down at 30. Either lower
`request_timeout` to 25s or raise `write_timeout` above it. Config validation
should reject the inversion at boot rather than leaving it to be discovered in
production.

---

## Related documents

- [Dependency injection](dependency-injection.md)
- [Routing and versioning](routing-and-versioning.md)
- [Configuration model](../03-configuration/configuration-model.md)
- [Observability](../06-quality/observability.md)
