# Target Architecture

**Status:** Partially implemented
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1

This document describes the architecture Kopiochi should converge on. It is the
destination; [`../07-roadmap/remediation-plan.md`](../07-roadmap/remediation-plan.md)
is the route.

Parts of it are no longer aspirational. The composition root (`cmd/api/`
`container.go`), the module contract (`internal/module`), the `modules/identity`
tree, and the HTTP composition below all exist as described. The middleware
stack, the configuration section's fail-closed behaviour, and the data-access
targets do not yet. Sections are annotated where they have shipped.

---

## Shape in one picture

```
cmd/
  api/            server entrypoint — cobra command, composition root
    main.go         parse flags, load config, build app, run
    container.go    THE wiring: every module registered explicitly
  migrate/        migration runner
  generator/      scaffolding tool (dev-only, excluded from server binary)

internal/         private to this binary — no external importers
  config/           typed config, precedence, validation
  db/               pool construction, health, tx helpers
  httpx/            router construction, middleware stack, problem+json
  observability/    logger, request logging, metrics
  platform/         shared kernel used by modules (see dependency rules)

modules/          business capabilities — ONE layout, no exceptions
  identity/
    domain/           entities, value objects, repository interfaces
    application/      use cases, DTOs, orchestration
    infrastructure/   bun repositories, token service, hasher
    transport/        HTTP handlers + route registration
    migrations/       schema owned by this module
    module.go         Register(deps) → Module
  aquaculture/
    (identical structure)

migrations/       global/shared schema only (extensions, roles, audit)
```

Two changes carry most of the value:

1. **One place where modules exist** (`modules/`), one layout inside each.
2. **One place where wiring happens** (`cmd/api/container.go`), and it is
   impossible to leave empty by accident.

---

## Layer contract inside a module

```
transport ──▶ application ──▶ domain
     │             │             ▲
     └─────────────┴─────────────┘
              infrastructure
     (implements domain interfaces, injected at the composition root)
```

- **domain** — pure Go. No bun, no chi, no viper, no zerolog. Depends on nothing
  but the standard library and `internal/platform`.
- **application** — use cases. Depends on `domain` interfaces only. Never imports
  bun or chi.
- **infrastructure** — implements `domain` interfaces using bun/pgx. Imports
  `domain`, never `transport`.
- **transport** — HTTP handlers. Imports `application`, never `infrastructure`.

Enforcement is mechanical, not cultural — see
[`../01-modularity/dependency-rules.md`](../01-modularity/dependency-rules.md).

---

## The module contract

Every module exposes exactly one exported symbol:

```go
// modules/identity/module.go
package identity

type Deps struct {
    DB     bun.IDB
    Config Config
    Logger zerolog.Logger
    Clock  platform.Clock
}

type Module struct {
    Routes     func(chi.Router)   // mounts under the version group
    Migrations fs.FS              // embedded, owned by this module
    Name       string
}

func New(deps Deps) (*Module, error) { ... }
```

This replaces both the `Extension`/`Manager` interface pair and the
`Plugin`/`Registry` pair. Rationale in
[ADR-004](../adr/004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md).

Key properties:

- **Typed dependencies.** `Deps` is a struct, not `map[string]interface{}`. The
  FIDO2 failure mode — requiring a live Go interface that YAML cannot express —
  becomes a compile error rather than a runtime one.
- **Typed config.** Each module declares its own `Config` struct with
  `mapstructure` tags, decoded and validated once at startup.
- **Constructor returns an error.** Misconfiguration fails at boot, not on the
  first request.

---

## Composition root

`cmd/api/container.go` is the only file that knows about every module:

```go
func Build(cfg *config.Config, db bun.IDB, log zerolog.Logger) (*App, error) {
    var mods []*module.Module

    identityMod, err := identity.New(identity.Deps{DB: db, Config: cfg.Identity, Logger: log})
    if err != nil {
        return nil, fmt.Errorf("identity module: %w", err)
    }
    mods = append(mods, identityMod)

    aquaMod, err := aquaculture.New(aquaculture.Deps{DB: db, Config: cfg.Aquaculture, Logger: log})
    if err != nil {
        return nil, fmt.Errorf("aquaculture module: %w", err)
    }
    mods = append(mods, aquaMod)

    if len(mods) == 0 {
        return nil, errors.New("no modules registered")   // the empty-container bug, caught at boot
    }
    return &App{Modules: mods}, nil
}
```

No reflection, no service locator, no `map[string]interface{}`. Wiring errors are
compile errors. Detail:
[`../02-composition/dependency-injection.md`](../02-composition/dependency-injection.md).

---

## HTTP composition — shipped

*Live as `internal/httpx.Mount` (`4fdc609`, `40887de`). Two differences from the
sketch: `Mount` takes `[]*module.Module` rather than reaching into `app`, because
`App` is in `package main` and would create an import cycle; and `/health`
survives as a deprecated alias for `/healthz`.*

```go
r := httpx.NewRouter(cfg.Server, log)      // core middleware stack

r.Get("/healthz", handlers.Live())          // no auth, no version
r.Get("/readyz", handlers.Ready(db))
r.Mount("/swagger", swaggerHandler())

r.Route("/api/v1", func(v1 chi.Router) {    // the version group is REAL
    for _, m := range app.Modules {
        m.Routes(v1)                         // modules receive the group router
    }
})
```

The original bug — a `/api/v1` block whose inner router was shadowed and
discarded — is structurally impossible here, because `Routes` takes the group
router as a parameter instead of closing over a router built elsewhere. Fixed in
`4fdc609`; guarded by `TestRouteTable`. See
[`../02-composition/routing-and-versioning.md`](../02-composition/routing-and-versioning.md)
and [ADR-007](../adr/007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md).

---

## Middleware stack (fixed order)

| # | Middleware | Purpose |
|---|-----------|---------|
| 1 | `RequestID` | Correlation id, first so everything downstream can log it |
| 2 | `RealIP` | Resolve client IP from trusted proxies only |
| 3 | `Recovery` | Panic → `problem+json` 500, logs stack |
| 4 | `RequestLogger` | Structured access log with status + duration |
| 5 | `Timeout` | Per-request deadline from config |
| 6 | `CORS` | Allowlist only, emits `Vary: Origin` |
| 7 | `RateLimit` | Keyed on resolved real IP, no lock held downstream |
| 8 | *(route group)* `Auth` | Applied to protected groups only |

Rationale for the ordering and each hardening change:
[`../04-security/middleware-hardening.md`](../04-security/middleware-hardening.md).

---

## Configuration

- One typed struct per concern, assembled into `config.Config`.
- Precedence: **defaults → file → environment → flags** (later wins).
- Every key has an explicit default registration so environment binding actually
  works — the silent failure of `APP_DB_PASSWORD` was a direct consequence of
  omitting this. *Partly done: `b74b358` added explicit `BindEnv` calls for
  `db.password` and the JWT secret (`internal/config/config.go:88-93`). `db.user`
  and `db.name` still have neither a default nor a binding, so `APP_DB_USER` and
  `APP_DB_NAME` remain inert.*
- Secrets never appear in `config/*.yaml`. *Done for the working tree
  (`b74b358`).* They arrive via environment or a secret store, and the config
  loader **fails closed** if a required secret is missing or is a known
  placeholder. *Not done — nothing rejects a placeholder, and `.env.example`
  ships `CHANGEME_*` values that would load happily. Phase 2.9.*

Detail: [`../03-configuration/configuration-model.md`](../03-configuration/configuration-model.md)
and [`../03-configuration/secret-management.md`](../03-configuration/secret-management.md).

---

## Data access

- `pgxpool` is the single pool; the `*sql.DB` wrapper for bun is configured to
  match it (`SetMaxOpenConns` = pool max, idle sized deliberately).
- DSN built with `net/url` so credentials containing `@ : / # ?` are escaped.
- Repositories return domain types; bun models never leave `infrastructure`.
- All queries use the bun query builder. No string-built SQL. *(Verified: the
  codebase currently complies with this.)*

Detail: [`../05-data/persistence-and-pooling.md`](../05-data/persistence-and-pooling.md).

---

## Migrations

Each module embeds and owns its migrations; the runner composes them in a defined
order. Global objects (extensions, shared enums, audit tables) live in the
top-level `migrations/`.

Detail: [`../05-data/migration-strategy.md`](../05-data/migration-strategy.md),
[ADR-010](../adr/010%20-%20Module-Owned%20Database%20Migrations.md).

---

## Binary composition

`cmd/api` links only what it serves. The scaffolding generator, the swagger
generation step, and any optional auth mechanism live behind build tags or in
separate binaries. Target: **drop the server binary well below its current 38 MB**
by not linking `webauthn`, `go-tpm`, `otp`, `barcode`, and `swag` into it.

Detail: [`../06-quality/repository-hygiene.md`](../06-quality/repository-hygiene.md).

---

## What this fixes

| Root cause (from current-state) | Mechanism in the target |
|---|---|
| No enforced dependency rules | One layout + import linter in CI |
| Composition is optional | `Build()` errors on zero modules; typed `Deps` |
| Config carries live objects | Typed `Deps` struct separates wiring from config |
| No feedback loop | Test strategy + CI gate + `.gitattributes` |

---

## Related documents

- [Current state](current-state.md)
- [Design principles](design-principles.md)
- [Remediation plan](../07-roadmap/remediation-plan.md)
