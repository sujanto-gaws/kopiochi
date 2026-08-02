# Dependency Injection & the Composition Root

**Status:** Proposed — see [ADR-006](../adr/006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
**Date:** 2026-08-02
**Severity of current defect:** Critical — the application serves no business routes.

---

## Problem

`cmd/api/container/container.go` in full:

```go
// New builds the full dependency graph and returns a ready Container.
func New(cfg *config.Config, db bun.IDB) (*Container, error) {
	// ── Shared infrastructure ────────────────────────────────────────────────

	// ── Auth ─────────────────────────────────────────────────────────────────

	// ── User ─────────────────────────────────────────────────────────────────

	// ── Register all handlers ─────────────────────────────────────────────────
	// To add a new handler: wire it above and append it here.
	return &Container{
		registrars: []handlers.RouteRegistrar{
			// Auth
		},
	}, nil
}
```

The function returns an empty slice and `nil` error. Consequences:

- `routes.Setup` iterates zero registrars.
- The server exposes only `/api/health` and `/swagger/*`.
- Startup logs `"application starting"`, the health check reports healthy, and
  **nothing indicates the application is empty**.
- Both parameters, `cfg` and `db`, are unused — the database connects, is
  verified with `Ping`, and is then never used by anything.

This is the single highest-impact defect in the repository. It is also a
*category* problem: the design makes "not wired" indistinguishable from "wired"
at both compile time and runtime.

---

## Why it happened

The container is a **passive list**. Its correctness is a property of a comment
("To add a new handler: wire it above and append it here") rather than of the
type system. There is no signal — no compile error, no boot failure, no test —
distinguishing an intentionally empty container from an unfinished one.

Combined with the extension-framework split (identity was written against
`internal/extension`, which `main.go` never instantiates), nothing ever connected
the ~2,000 LOC of working auth code to the server.

---

## Target design

### 1. The container returns an application, and refuses to be empty

```go
// cmd/api/container.go
package main

type App struct {
    Modules []*module.Module
}

func BuildApp(cfg *config.Config, db bun.IDB, log zerolog.Logger) (*App, error) {
    deps := module.Deps{
        DB:     db,
        Logger: log,
        Clock:  platform.SystemClock{},
    }

    var mods []*module.Module

    identityMod, err := identity.New(deps, cfg.Identity)
    if err != nil {
        return nil, fmt.Errorf("module identity: %w", err)
    }
    mods = append(mods, identityMod)

    aquaMod, err := aquaculture.New(deps, cfg.Aquaculture)
    if err != nil {
        return nil, fmt.Errorf("module aquaculture: %w", err)
    }
    mods = append(mods, aquaMod)

    if len(mods) == 0 {
        return nil, errors.New("no modules registered: refusing to start an empty application")
    }

    log.Info().Strs("modules", names(mods)).Msg("modules registered")
    return &App{Modules: mods}, nil
}
```

Three properties the current code lacks:

| Property | Mechanism |
|---|---|
| Empty application cannot start | Explicit `len(mods) == 0` guard |
| Misconfiguration fails at boot | Module constructors return `error`, wrapped with `%w` |
| Wiring is observable | Registered module names logged at startup |

### 2. Constructor injection, no service locator

Dependencies are constructor parameters. There is no runtime lookup by string
name — no `GetService("user_repo")`, no `map[string]interface{}`.

```go
users   := persistence.NewUserRepository(deps.DB)        // implements domain.UserRepository
tokens  := token.NewJWTService(cfg.PrivateKeyPath, ...)  // implements domain.TokenIssuer
authSvc := application.NewAuthService(users, tokens, deps.Clock)
handler := transport.NewAuthHandler(authSvc, deps.Logger)
```

Any missing or mistyped dependency is a compile error. Contrast with
`internal/extension/manager.go:198-211`, where services are registered and
retrieved by string with an `interface{}` value and an `ok` boolean — every
lookup a potential runtime failure.

Note also `Manager.RegisterService` **eagerly invokes the factory** at
registration time (`m.services[name] = factory()`), so a "lazy factory" API is
not lazy at all. Constructor injection removes the question entirely.

### 3. No DI framework

Explicit Go code is preferred over `wire`, `dig`, or `fx`:

- The graph is small (two modules, a handful of nodes each).
- Explicit wiring is greppable and debuggable; a stack trace points at real code.
- No code generation step, no reflection-based container to learn.

Revisit only if the module count grows past roughly a dozen. Recorded in
[ADR-006](../adr/006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md).

### 4. Interfaces declared by consumers

```go
// modules/identity/domain/repository.go — the consumer declares what it needs
type UserRepository interface {
    ByEmail(ctx context.Context, email string) (*AppUser, error)
    Create(ctx context.Context, u *AppUser) error
}
```

`infrastructure/persistence` satisfies it structurally. The domain never imports
the implementation; the composition root is the only place both are visible.

---

## Testability

The current container cannot be tested — there is nothing to assert. The target
supports:

```go
func TestBuildApp_RegistersModules(t *testing.T) {
    app, err := BuildApp(testConfig(t), testDB(t), zerolog.Nop())
    require.NoError(t, err)
    require.NotEmpty(t, app.Modules, "application must register at least one module")
}

func TestBuildApp_FailsOnInvalidConfig(t *testing.T) {
    cfg := testConfig(t)
    cfg.Identity.Issuer = ""                     // Validate() must reject this
    _, err := BuildApp(cfg, testDB(t), zerolog.Nop())
    require.Error(t, err)
}
```

Plus the route-table test in
[routing and versioning](routing-and-versioning.md), which is the check that
would have caught the empty container immediately.

---

## Migration path

1. Add `internal/module` (`Deps`, `Module`) — additive, breaks nothing.
2. Write `modules/identity/module.go` over the existing identity internals.
3. Replace `cmd/api/container/container.go` with `cmd/api/container.go` as above.
4. Update `main.go` to call `BuildApp` and pass `app.Modules` to route setup.
5. Add the two container tests and the route-table test.
6. Delete `handlers.RouteRegistrar` and the `RouterGroup` indirection once no
   caller remains.

Steps 1–4 restore a functioning application; step 5 prevents the regression.

---

## Related documents

- [ADR-006: Explicit Compile-Time Dependency Injection](../adr/006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
- [Routing and versioning](routing-and-versioning.md)
- [Lifecycle and shutdown](lifecycle-and-shutdown.md)
- [Extension framework](../01-modularity/extension-framework.md)
