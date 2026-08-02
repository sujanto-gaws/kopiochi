# Routing & API Versioning

**Status:** Proposed — see [ADR-007](../adr/007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)
**Date:** 2026-08-02
**Severity of current defect:** Critical — the `/api/v1` prefix does not exist at runtime.

---

## Problem 1: the version prefix is a no-op

`internal/infrastructure/http/routes/routes.go:25-29`:

```go
// API v1 routes
r.Route("/api/v1", func(r chi.Router) {     // ← inner `r` shadows the outer *chi.Mux
    for _, reg := range registrars {
        reg.RegisterRoutes(g)                // ← registers onto `g`, NOT onto the inner `r`
    }
})
```

The inner `r` — the only router actually mounted at `/api/v1` — is **shadowed and
never used**. Registrars receive `g`, a `handlers.RouterGroup` built in
`cmd/api/main.go:106-113`:

```go
v1 := r.With()                       // inline sub-router of the ROOT mux — no path prefix
var protected chi.Router
if authMiddleware != nil {
    protected = v1.With(authMiddleware)
} else {
    protected = v1
}
g := handlers.RouterGroup{Public: v1, Protected: protected}
```

`chi.Mux.With()` returns an *inline* router that shares the parent's routing tree
and carries **no path prefix**. So the moment a registrar is added, its routes
mount at `/` — `/login`, not `/api/v1/login`.

The `r.Route("/api/v1", ...)` block mounts an empty sub-router and contributes
nothing.

This is currently invisible because
[the container is empty](dependency-injection.md) and the loop body never
executes. Fixing the container without fixing this would silently publish the
entire API at the wrong paths.

The comment on line 106 — `// scoped sub-router for /api/v1 context` — states an
intent the code does not implement.

## Problem 2: auth binding is decided too early and too globally

`main.go:99-112` resolves the auth middleware once, then builds a single
`Protected` router for **all** modules:

```go
var authMiddleware func(http.Handler) http.Handler
if authPlugin := pluginRegistry.GetAuth("jwt-auth"); authPlugin != nil {
    authMiddleware = authPlugin.AuthMiddleware()
}
...
if authMiddleware != nil {
    protected = v1.With(authMiddleware)
} else {
    protected = v1          // ← silently unprotected
}
```

Two problems:

1. **Fail-open.** With `plugins.auth.jwt.enabled: false` (the default in
   `config/default.yaml`), `authMiddleware` is nil and `Protected` becomes an
   alias for `Public`. Every route a module considers protected is served with no
   authentication and no warning logged.
2. **One policy for everything.** A module cannot express "these three routes need
   an admin scope, these two are public" — the split is fixed by the host.

## Problem 3: `RouterGroup` splits routing across two files

Modules cannot see their own path structure. The prefix lives in `routes.go`, the
public/protected split in `main.go`, and the paths in the handler — which is how
the shadowing bug survived review.

---

## Target design

### Modules receive the group router directly

```go
// internal/httpx/routes.go
func Mount(r *chi.Mux, app *App, deps Deps) {
    // Operational endpoints: unversioned, unauthenticated
    r.Get("/healthz", handlers.Live())
    r.Get("/readyz", handlers.Ready(deps.DB))
    r.Mount("/swagger", swaggerHandler())

    // Versioned API
    r.Route("/api/v1", func(v1 chi.Router) {
        for _, m := range app.Modules {
            m.Routes(v1)          // ← the group router is PASSED IN, never shadowed
        }
    })
}
```

`Module.Routes` has signature `func(chi.Router)`. There is no second router in
scope to accidentally register against — the bug becomes unrepresentable.

### Modules declare their own protection

```go
// modules/identity/transport/routes.go
func (h *Handler) Routes(r chi.Router) {
    r.Route("/auth", func(r chi.Router) {
        // public
        r.Post("/login", h.Login)
        r.Post("/refresh", h.Refresh)

        // authenticated
        r.Group(func(r chi.Router) {
            r.Use(h.auth.RequireUser)
            r.Post("/logout", h.Logout)
            r.Get("/me", h.Me)
        })

        // admin only
        r.Group(func(r chi.Router) {
            r.Use(h.auth.RequireUser, h.auth.RequireScope("admin"))
            r.Get("/users", h.ListUsers)
        })
    })
}
```

`h.auth` is a typed middleware provider injected through the constructor. It is
**never nil** — if the module needs authentication, its constructor fails at boot
when the token verifier cannot be built. Fail-closed replaces fail-open.

### Versioning policy

| Rule | Detail |
|---|---|
| Version in the path | `/api/v1/...`. Not headers — path versioning is greppable and cache-friendly. |
| One major version per breaking change | Additive fields do not bump the version. |
| Versions coexist | `r.Route("/api/v2", ...)` mounts a second module set during migration. |
| Operational endpoints are unversioned | `/healthz`, `/readyz`, `/metrics` |
| Deprecation is announced | `Deprecation` and `Sunset` headers on the old version. |

### Health endpoint split

`/api/health` (`routes.go:17`) is currently versioned-adjacent and does not check
the database. Replace with:

- `/healthz` — liveness. Process is up. No dependency checks.
- `/readyz` — readiness. Pings the pool; returns 503 when the database is
  unreachable so orchestrators stop routing traffic.

The current single endpoint reports healthy even when the application serves
nothing, which is exactly how the empty container went unnoticed.

---

## The test that catches all of this

```go
func TestRouteTable(t *testing.T) {
    app, err := BuildApp(testConfig(t), testDB(t), zerolog.Nop())
    require.NoError(t, err)

    r := httpx.NewRouter(testServerConfig(), zerolog.Nop())
    httpx.Mount(r, app, testDeps(t))

    var got []string
    err = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
        got = append(got, method+" "+route)
        return nil
    })
    require.NoError(t, err)

    // Fails today on BOTH counts: no routes at all, and no /api/v1 prefix.
    require.Contains(t, got, "POST /api/v1/auth/login")
    require.Contains(t, got, "GET /healthz")
}
```

`chi.Walk` is the cheapest available guard against the empty container, the
shadowed router, and accidental path changes. It requires no HTTP server and no
live database beyond what `BuildApp` needs.

---

## Middleware ordering on the router

Set once in `internal/httpx.NewRouter`; see
[middleware hardening](../04-security/middleware-hardening.md) for the rationale
and the security fixes.

One ordering note about the current code: `server.NewRouter` registers
`r.NotFound` and `r.MethodNotAllowed` (lines 58-59) *before* the `r.Use` calls
(lines 62-66). This is legal in chi today — `NotFound` does not build the route
tree, so `Use` does not panic — but it reads as fragile. Register middleware
first, then handlers, so the ordering constraint is visually obvious.

---

## Migration path

1. Add `Module.Routes func(chi.Router)` to the module contract.
2. Rewrite `routes.Setup` as `httpx.Mount`, passing `v1` into `m.Routes`.
3. Move the public/protected split into each module's `Routes`.
4. Delete `handlers.RouterGroup` and `handlers.RouteRegistrar`.
5. Split `/api/health` into `/healthz` + `/readyz`.
6. Add `TestRouteTable` — it must fail before step 2 and pass after.

---

## Related documents

- [ADR-007: API Versioning at the Router Boundary](../adr/007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)
- [Dependency injection](dependency-injection.md)
- [Middleware hardening](../04-security/middleware-hardening.md)
