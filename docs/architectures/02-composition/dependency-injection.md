# Dependency Injection & the Composition Root

**Status:** Implemented — see [ADR-006](../adr/006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1
**Severity of current defect:** None outstanding — the target design below shipped
in `ef76759`. `cmd/api/container.go` now exposes `BuildApp`, which wires the
`identity` and `user` modules, refuses to return a zero-module application, and is
covered by `cmd/api/container_test.go`. The routes it produces serve under
`/api/v1` (`internal/httpx.Mount`, fixed in `4fdc609`).

---

## Problem

> **Withdrawn.** An earlier revision of this document presented
> `cmd/api/container/container.go` "in full" as a verbatim excerpt whose
> `registrars` slice was empty, and drew four consequences from it — zero
> registrars iterated, only `/api/health` and `/swagger/*` served, nothing
> indicating an empty application, and `cfg`/`db` unused — before calling it "the
> single highest-impact defect in the repository". That code never existed. Both
> the earliest version of the file (`git show 794d783:cmd/api/container/container.go`)
> and the version current when these documents were written
> (`git show 0fbab20:...`) return
> `registrars: []handlers.RouteRegistrar{authHandler, userHandler}`, and both
> parameters are used. The excerpt and the consequences drawn from it could not be
> substantiated and have been withdrawn.

What survives the withdrawal is the design observation, which does not depend on
the container ever having been empty:

The old container was a **passive list**. Its correctness was a property of a
comment — "To add a new handler: wire it above and append it here" — rather than
of the type system. `New` returned `(*Container, error)` regardless of how many
registrars it appended, so there was no compile error, no boot failure, and no
test that could distinguish a complete container from an unfinished one. A
handler omitted from the slice was simply not served, silently.

That is a *category* problem: the design made "not wired" indistinguishable from
"wired" at both compile time and runtime. The target below closes it by making
emptiness a boot failure rather than a valid state.

---

## Target design — shipped

Landed in `ef76759` as `cmd/api/container.go`. The sketch below is what was
designed; the shipped code differs in three details: `module.Deps` carries only
`DB` and `Logger` (no `Clock`), the identity module reads `cfg.Auth` rather than a
`cfg.Identity` block, and the registered modules are `identity` and `user`.

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

    userMod, err := newUserModule(deps, cfg)
    if err != nil {
        return nil, fmt.Errorf("module user: %w", err)
    }
    mods = append(mods, userMod)

    if len(mods) == 0 {
        return nil, errors.New("no modules registered: refusing to start an empty application")
    }

    log.Info().Strs("modules", names(mods)).Msg("modules registered")
    return &App{Modules: mods}, nil
}
```

Three properties the previous container lacked, all now in place
(`cmd/api/container.go`):

| Property | Mechanism | Where |
|---|---|---|
| Empty application cannot start | Explicit `len(mods) == 0` guard | `container.go:66-68` |
| Misconfiguration fails at boot | Module constructors return `error`, wrapped with `%w` | `container.go:56`, `:62`, `:93` |
| Wiring is observable | Registered module names logged at startup | `container.go:74` |

> The `len(mods) == 0` guard is currently unreachable, because two modules are
> appended unconditionally above it. It is retained deliberately as a guard for
> future edits — but it is not, today, an executable check. See
> [testing strategy](../06-quality/testing-strategy.md).

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
    FindByEmail(ctx context.Context, email string) (*User, error)
    FindByID(ctx context.Context, id string) (*User, error)
    FindByUsername(ctx context.Context, username string) (*User, error)
    Save(ctx context.Context, user *User) error
}
```

`infrastructure/persistence` satisfies it structurally. The domain never imports
the implementation; the composition root is the only place both are visible.

---

## Testability

Both tests below shipped in `d92480c` as `cmd/api/container_test.go`:

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
catches a mis-wired composition root immediately.

---

## Migration path — complete

1. ✅ Add `internal/module` (`Deps`, `Module`) — `05b1051`, additive, broke nothing.
2. ✅ Write `modules/identity/module.go` over the existing identity internals — `5f6edfe`, `6d0c1b7`.
3. ✅ Replace `cmd/api/container/container.go` with `cmd/api/container.go` — `ef76759`.
4. ✅ Update `main.go` to call `BuildApp` and pass `app.Modules` to route setup — `ef76759`, `4fdc609`.
5. ✅ Add the two container tests and the route-table test — `d92480c`.
6. ✅ Delete `handlers.RouteRegistrar` and the `RouterGroup` indirection — `4fdc609`;
   `grep -rn "RouteRegistrar\|RouterGroup" --include=*.go .` now returns nothing.

---

## Related documents

- [ADR-006: Explicit Compile-Time Dependency Injection](../adr/006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
- [Routing and versioning](routing-and-versioning.md)
- [Lifecycle and shutdown](lifecycle-and-shutdown.md)
- [Extension framework](../01-modularity/extension-framework.md)
