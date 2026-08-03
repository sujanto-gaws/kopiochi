# Adding Custom Plugins to Kopiochi — removed

> ## ⛔ There is no plugin system to add plugins to.
>
> `internal/plugin/`, `internal/plugins/` and `internal/extension/` were
> deleted in Phase 3 of the
> [remediation plan](docs/architectures/07-roadmap/remediation-plan.md)
> (commit `de7e242`). Every path this guide told you to create files in, and
> every interface it told you to implement, is gone.
>
> Kept as a tombstone so links from older commits, issues and forks land
> somewhere that explains what happened.
> [PLUGIN_GUIDE.md](PLUGIN_GUIDE.md) has the full reasoning.

## What to do instead

Whatever you were going to build is one of two things.

### A business capability → write a module

Anything that owns data, routes, or domain rules. Create
`modules/<name>/module.go` with a constructor:

```go
package yours

// Config is your module's own typed settings. No map[string]interface{},
// so a YAML type error fails startup instead of silently becoming a default.
type Config struct {
    SomeSetting time.Duration `mapstructure:"some_setting"`
}

func (c Config) Validate() error {
    if c.SomeSetting <= 0 {
        return errors.New("yours: some_setting must be positive")
    }
    return nil
}

func New(deps module.Deps, cfg Config) (*module.Module, error) {
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("yours: invalid config: %w", err)
    }

    repo := repository.New(deps.DB)   // deps.DB is a typed bun.IDB
    svc := application.NewService(repo)

    return &module.Module{
        Name:   "yours",
        Routes: transport.New(svc).Routes,  // mounted under /api/v1
    }, nil
}
```

Then build it in `cmd/api/container.go`'s `BuildApp` and append it to `mods`.
That is the entire registration mechanism: a function call in a file you can
read top to bottom.

The layout inside a module mirrors `modules/identity` and `modules/user`:

```
modules/yours/
├── module.go          # New(deps, cfg) — the only thing the host sees
├── domain/            # entities + repository interfaces; no bun, no chi
├── application/       # use cases over those interfaces
├── infrastructure/    # bun models, repositories, external clients
└── transport/         # HTTP handlers + Routes(chi.Router)
```

**Return an error rather than degrading.** `user.New` refuses to construct
without an auth middleware, because a module that mounts its routes
unprotected looks identical from the outside — same route table, no warning.

**Four rules are enforced mechanically.** `tools/archtest` and `.golangci.yml`
fail the build, not the review:

| Rule | Meaning |
|---|---|
| R1 | `domain` may import the standard library and `internal/platform` only — not bun, chi, viper, zerolog or pgx. `application` may not import the ORM, the router, or its own `infrastructure` |
| R2 | Modules must not import each other. Declare the interface you need in your own package and satisfy it in `cmd/api/container.go` |
| R3 | `internal/**` must not import `modules/**`. Only `cmd/**` and `tools/**` know about every module |
| R4 | No package named `utils`, `util`, `common`, `shared`, `helpers` or `misc` |

Run them with `make arch`. See
[dependency-rules.md](docs/architectures/01-modularity/dependency-rules.md).

### Cross-cutting HTTP behaviour → add it to `internal/httpx`

A header, a limiter, a tracer — something that applies to every request and
owns no domain. Put it in `internal/httpx`, give it a typed config struct in
`internal/config`, and register it in `NewRouter` behind an `if`:

```go
if sec.YourThing.Enabled {
    r.Use(YourThing(sec.YourThing))
}
```

That is how CORS and rate limiting work now. If your middleware owns a
resource — a goroutine, a ticker, a connection — return a `Close` and add it
to the router's closer list, so the lifecycle stack releases it in order. The
rate limiter's eviction sweep is the worked example.

Do not build a registry. The last two are what Phase 3 removed.
