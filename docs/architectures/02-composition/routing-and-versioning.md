# Routing & API Versioning

**Status:** Implemented — see [ADR-007](../adr/007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1
**Severity of current defect:** None outstanding — all three problems below were
fixed in `4fdc609` (routing) and `40887de` (health split). `/api/v1` is a real
prefix at runtime, `TestRouteTable` (`cmd/api/routes_test.go`) asserts it, and
`internal/infrastructure/http/routes/` no longer exists.

---

## Problem 1 (fixed): the version prefix was a no-op

*Fixed in `4fdc609`. The file quoted below was deleted in the same commit; the
excerpt is preserved because the shape of the bug is the reason `httpx.Mount` is
written the way it is.*

Historical `internal/infrastructure/http/routes/routes.go:25-29`:

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

At review time this was invisible, because the review believed
[the container was empty](dependency-injection.md) and the loop body never ran.
Fixing the container without fixing this would have silently published the entire
API at the wrong paths — so both landed together, the router fix (`4fdc609`)
immediately after the container rewrite (`ef76759`).

The comment on line 106 — `// scoped sub-router for /api/v1 context` — stated an
intent the code did not implement.

## Problem 2 (fixed): auth binding was decided too early and too globally

*Fixed in `ef76759` / `6d0c1b7`. Auth middleware is now owned by the module that
needs it: `modules/identity/module.go` builds its own `AuthRequired` from its
token service, and `cmd/api/container.go:90-98` does the same for the user
module. Both return an error rather than a module with unprotected routes, so
the fail-open path below cannot recur.*

Historical `main.go:99-112` resolved the auth middleware once, then built a single
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

1. **Fail-open.** With `plugins.auth.jwt.enabled: false` (still the default in
   `config/default.yaml:47`), `authMiddleware` was nil and `Protected` became an
   alias for `Public`. Every route a module considered protected was served with
   no authentication and no warning logged.
2. **One policy for everything.** A module could not express "these three routes
   need an admin scope, these two are public" — the split was fixed by the host.

## Problem 3 (fixed): `RouterGroup` split routing across two files

*Fixed in `4fdc609`: `handlers.RouterGroup` and `handlers.RouteRegistrar` were
deleted along with `internal/infrastructure/http/routes/`. Each module now owns
its own path structure via `Module.Routes(chi.Router)`.*

Modules could not see their own path structure. The prefix lived in `routes.go`,
the public/protected split in `main.go`, and the paths in the handler — which is
how the shadowing bug survived review.

---

## Target design — shipped

### Modules receive the group router directly

```go
// internal/httpx/routes.go
func Mount(r *chi.Mux, modules []*module.Module, deps Deps) {
    // Operational endpoints: unversioned, unauthenticated
    r.Get("/healthz", healthzHandler())
    r.Get("/readyz", readyzHandler(deps.Pinger))
    r.Get("/health", healthzHandler())   // deprecated alias; drop once clients migrate
    r.Get("/swagger/*", swaggerHandler())

    // Versioned API
    r.Route("/api/v1", func(v1 chi.Router) {
        for _, m := range modules {
            m.Routes(v1)          // ← the group router is PASSED IN, never shadowed
        }
    })
}
```

`Mount` takes `[]*module.Module` rather than the `*App` this document originally
sketched: `App` lives in `package main`, which `internal/httpx` cannot import
without an import cycle. `cmd/api/main.go:104` passes `app.Modules`.

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

### Health endpoint split — shipped

The old `/health` (`routes.go:17`) checked nothing. It was split in `40887de`
into `internal/httpx/health.go`:

- `/healthz` — liveness. Process is up. No dependency checks.
- `/readyz` — readiness. Pings the pool with a 2s timeout; returns 503 when the
  database is unreachable, and fails closed on a nil pinger, so orchestrators stop
  routing traffic.
- `/health` — retained as a deprecated alias for `/healthz` (`routes.go:48`) so
  existing probes keep working; drop it once clients migrate.

A single endpoint that reports healthy without checking anything is how a broken
composition root goes unnoticed. Covered by `internal/httpx/health_test.go`.

---

## The test that catches all of this

Implemented as `TestRouteTable` in `cmd/api/routes_test.go` (`d92480c`) — it lives
in `cmd/api` rather than `internal/httpx` because it needs `BuildApp`, which is in
`package main`.

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

    // Failed on BOTH counts before 4fdc609: no /api/v1 prefix, and modules
    // mounted onto a discarded router. Passes now.
    require.Contains(t, got, "POST /api/v1/auth/login")
    require.Contains(t, got, "GET /healthz")
}
```

`chi.Walk` is the cheapest available guard against a half-wired composition root,
the shadowed router, and accidental path changes. It requires no HTTP server and no
live database beyond what `BuildApp` needs.

---

## Middleware ordering on the router

Set once in `internal/httpx.NewRouter`; see
[middleware hardening](../04-security/middleware-hardening.md) for the rationale
and the security fixes.

> **Withdrawn.** An earlier revision noted here that `server.NewRouter` registers
> `r.NotFound` and `r.MethodNotAllowed` before its `r.Use` calls, and argued for
> reversing the order. It registers neither, and never did:
> `git show 4fdc609^:internal/infrastructure/http/server/server.go` shows five
> `r.Use` calls and no handler registrations, as does the current file. The claim
> could not be substantiated and has been withdrawn.

The real gap is that 404 and 405 have no handlers at all, so they fall through to
chi's plain-text defaults instead of the `application/problem+json` envelope every
other error path uses. See
[middleware hardening](../04-security/middleware-hardening.md).

---

## Migration path — complete

1. ✅ Add `Module.Routes func(chi.Router)` to the module contract — `05b1051`.
2. ✅ Rewrite `routes.Setup` as `httpx.Mount`, passing `v1` into `m.Routes` — `4fdc609`.
3. ✅ Move the public/protected split into each module's `Routes` — `6d0c1b7`, `ef76759`.
4. ✅ Delete `handlers.RouterGroup` and `handlers.RouteRegistrar` — `4fdc609`.
5. ✅ Split `/api/health` into `/healthz` + `/readyz` — `40887de`.
6. ✅ Add `TestRouteTable` — `d92480c`; it failed before step 2 and passes after.

---

## Related documents

- [ADR-007: API Versioning at the Router Boundary](../adr/007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)
- [Dependency injection](dependency-injection.md)
- [Middleware hardening](../04-security/middleware-hardening.md)
