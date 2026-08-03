# Plugin System Guide — removed

> ## ⛔ The plugin system this guide described no longer exists.
>
> `internal/plugin/`, `internal/plugins/` and `internal/extension/` were
> deleted in Phase 3 of the
> [remediation plan](docs/architectures/07-roadmap/remediation-plan.md)
> (commit `de7e242`, 4,023 lines). Nothing registers, initialises or looks up
> a plugin any more, and the `plugins:` block is gone from
> `config/default.yaml`.
>
> This file is kept as a tombstone rather than deleted, so links to it from
> older commits, issues and forks land somewhere that explains what happened.

## Why it was removed

The repository contained **two complete, competing extension frameworks** —
`Manager` (`internal/extension/`, Yii-inspired, five-pass bootstrap) and
`Registry` (`internal/plugin/` + `internal/plugins/`, a factory registry).
Both implemented registration, lifecycle and lookup. Only one was reachable
from `main.go`, and the unreachable one carried a 1,076-line parallel identity
implementation that nothing imported.

The live one's defects were structural rather than incidental, because they
all followed from configuring plugins with `map[string]interface{}`:

| Defect | Consequence |
|---|---|
| Config cannot carry dependencies | `fido2-auth` needed a live `UserStore` in its config map. Viper only ever produces strings, numbers, bools, slices and maps, so those 383 lines **could not run under any configuration** |
| Re-initialisation leaked the previous instance | Any plugin holding a goroutine, ticker or connection leaked it |
| Config type mismatches were silent | `requests: "500"` fell through an unchecked type assertion to the default, with no warning |
| `plugins.custom` initialised everything indiscriminately | Configuration and activation were conflated |
| Adapters bridged the two interface families | Indirection that existed only because the interfaces were designed twice |

Full analysis:
[extension-framework.md](docs/architectures/01-modularity/extension-framework.md)
and [ADR-004](<docs/architectures/adr/004 - Consolidate on a Single Extension Framework.md>).

## What replaced it

The distinction the plugin system never made — between a **business
capability** and a **cross-cutting HTTP concern** — is now the whole design.

**Business capabilities are modules.** A plain constructor returning a
`*module.Module`, built explicitly in `cmd/api/container.go`. Dependencies
arrive as typed struct fields, so a missing one is a compile error rather than
a runtime assertion failure. See the "Modules & cross-cutting middleware"
section of [README.md](README.md) and
[module-layout.md](docs/architectures/01-modularity/module-layout.md).

**CORS and rate limiting are neither modules nor plugins.** They are built
directly in `internal/httpx.NewRouter` from typed config, behind one `if`
each. Roughly 1,100 lines of registration framework collapsed into those two
conditionals.

| Was | Is now |
|---|---|
| `plugins.middleware: [cors, ratelimit]` | `security.cors.enabled`, `security.rate_limit.enabled` |
| `plugins.custom.cors.allowed_origins` | `security.cors.allowed_origins` |
| `plugins.custom.ratelimit.requests` / `.window` | `security.rate_limit.rate` (per minute) / `.burst` |
| `plugins.auth.jwt` (HS256) | `auth:` — RS256, issued by `modules/identity` |
| `plugins.auth.fido2` | deleted; it never worked |
| `plugins.cache.redis` | deleted; no cache plugin was ever registered |
| `APP_RATELIMIT_*`, `APP_CORS_*` | `APP_SECURITY_RATE_LIMIT_*`, `APP_SECURITY_CORS_*` |

That last row deserves a specific warning: the old `APP_RATELIMIT_*` and
`APP_CORS_*` variables appeared in `.env.example` but mapped to no Viper key
at all, so setting them did nothing and always had.
`TestLoad_SecurityEnvOverrides` now pins the replacements to real fields.

## If you were about to write a plugin

Write a module instead. It is less code, and the compiler checks it:

```go
// modules/yours/module.go
package yours

func New(deps module.Deps, cfg Config) (*module.Module, error) {
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("yours: invalid config: %w", err)
    }

    repo := repository.New(deps.DB)
    svc := application.NewService(repo)

    return &module.Module{
        Name:   "yours",
        Routes: transport.New(svc).Routes,
    }, nil
}
```

Then build it in `cmd/api/container.go`. Validate your `Config` and return an
error rather than degrading — `user.New` refuses to construct without an auth
middleware, because a module that mounts its routes unprotected looks
identical from the outside.

Two boundaries are enforced mechanically; `tools/archtest` and `.golangci.yml`
fail the build, not the review:

- Modules must not import each other. Declare the interface you need in your
  own package and satisfy it at the composition root.
- `internal/**` must not import `modules/**`.

See [dependency-rules.md](docs/architectures/01-modularity/dependency-rules.md).

If what you are adding is genuinely cross-cutting HTTP behaviour rather than a
capability — a header, a limiter, a tracer — put it in `internal/httpx` and
register it in `NewRouter` behind a typed config flag. Do not build a registry
for it.
