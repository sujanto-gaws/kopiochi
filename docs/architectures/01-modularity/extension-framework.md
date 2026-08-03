# Extension Framework Consolidation

**Status:** Proposed — see [ADR-004](../adr/004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
**Date:** 2026-08-02

---

## Problem

The codebase contains **two complete, competing extension frameworks**. Both
implement registration, lifecycle, and lookup. Only one is reachable from
`main.go`.

### Framework A — `Manager` (`internal/extension/`, ~600 LOC)

Yii-inspired. Five-pass bootstrap over registered extensions:

```go
// internal/extension/manager.go:80-137
func (m *Manager) Bootstrap() error {
    // 1. EarlyBootstrap for BootstrappableExtension
    // 2. Bootstrap for all
    // 3. Routes for RoutableExtension
    // 4. Services for ServiceProvider
    // 5. Events for EventListener
}
```

Imported by: `examples/extension-demo/main.go` and
`internal/extension/identity/extension.go`. **Not imported by `cmd/`.**

### Framework B — `Registry` (`internal/plugin/` + `internal/plugins/`, ~500 LOC)

Factory registry with interface-assertion accessors:

```go
// internal/plugin/registry.go
func (r *Registry) Register(name string, factory PluginFactory)
func (r *Registry) Initialize(name string, cfg map[string]interface{}) error
func (r *Registry) GetAuth(name string) AuthPlugin
func (r *Registry) GetMiddleware(name string) MiddlewarePlugin
```

Imported by `cmd/api/main.go:59-67`. **This is the live one.**

### What Framework A actually costs

> **Withdrawn.** An earlier revision of this document concluded here that "the
> entire identity extension is written against Framework A, which the server
> never instantiates", and that this was "the direct mechanical reason ~2,000 LOC
> of working auth code is dead". That is not what happened. The live auth stack
> was ordinary DDD code under `internal/{domain,application,infrastructure}/auth`,
> reachable through the container the whole time; it was moved to
> `modules/identity/**` in `5f6edfe`/`6d0c1b7` and is now served under
> `/api/v1/auth/*`. No `extensions/` directory has ever existed in this
> repository. The claim could not be substantiated and has been withdrawn.

What Framework A does cost is a 1,076-LOC parallel identity implementation
written against it:

```
internal/extension/identity/{extension,models,repository,service}.go   1,076 LOC
```

It has no transport layer, it duplicates concepts the live `modules/identity`
owns, and `grep -rn "extension/identity" --include=*.go .` finds no importer. Its
only reason to exist is that `internal/extension/` offered a registration
mechanism; deleting the framework removes the reason to keep the copy. Together
with `internal/extension/` itself (~600 LOC) and `examples/extension-demo/`, that
is the whole of Framework A's footprint.

The defects below are in the framework the server **does** run.

---

## Defects in the live framework

### 1. Config cannot carry dependencies — FIDO2 is unreachable by construction

`internal/plugins/auth/fido2.go:92-100`:

```go
if cfg["user_store"] != nil {
    if store, ok := cfg["user_store"].(UserStore); ok {
        p.userStore = store
    } else {
        return fmt.Errorf("fido2-auth: user_store must implement UserStore interface")
    }
} else {
    return fmt.Errorf("fido2-auth: user_store is required")
}
```

`cfg` originates from Viper (`internal/plugin/initializer.go:40-47`), which
produces only `string`, `float64`, `bool`, `map`, and `[]interface{}`. It can
**never** contain a value implementing `UserStore`.

Therefore `fido2-auth` returns an error on every initialisation attempt. 383 LOC
that cannot run under any configuration.

This is the general failure of the `map[string]interface{}` contract: it cannot
distinguish "configuration" (data, from YAML) from "dependencies" (live objects,
from the composition root).

### 2. Re-initialisation leaks the previous instance

`internal/plugin/registry.go:99-118`:

```go
func (r *Registry) Initialize(name string, cfg map[string]interface{}) error {
    plugin := factory()
    if err := plugin.Initialize(cfg); err != nil { ... }
    r.plugins[name] = plugin   // overwrites without Close()ing the old instance
    return nil
}
```

Any plugin holding a goroutine, ticker, or connection leaks it on re-init.

### 3. Config type mismatches are silent

Every plugin parses config with unchecked type assertions that fall through to a
default:

```go
// ratelimit.go:33-37
if maxReq, ok := cfg["requests"].(float64); ok {
    p.maxRequests = int(maxReq)
} else {
    p.maxRequests = 100      // silently used when YAML says requests: "500"
}
```

A YAML string where a number was expected produces the default with no warning.
Note also that Viper yields `int` for unquoted YAML integers in some paths, so
the `float64` assertion is itself fragile.

### 4. `custom` initialises everything indiscriminately

`internal/plugin/initializer.go:72-81` loops over every `plugins.custom` entry and
initialises it as a plugin. But `custom` is *also* the config source for
middleware plugins (line 18). An entry present in `custom` but absent from
`middleware` gets initialised anyway — configuration and activation are conflated.

### 5. Adapters exist purely to bridge the two interface families

`internal/plugins/adapters.go` wraps every concrete plugin in
`authPluginAdapter` / `middlewarePluginAdapter` so it satisfies
`plugin.Plugin`. That indirection exists only because the interfaces were
designed twice.

---

## Target: one module contract

Delete both frameworks. Replace with a plain constructor per module.

```go
// internal/module/module.go
package module

// Deps carries live collaborators from the composition root.
// It is a struct — not a map — so a missing dependency is a compile error.
type Deps struct {
    DB     bun.IDB
    Logger zerolog.Logger
    Clock  platform.Clock
}

// Module is what a business capability exposes to the host.
type Module struct {
    Name       string
    Routes     func(r chi.Router)   // mounts onto the version group
    Migrations fs.FS                // embedded, owned by the module
    Close      func() error         // optional cleanup
}
```

Each module supplies its own typed config and constructor:

```go
// modules/identity/module.go
package identity

type Config struct {
    AccessTokenTTL  time.Duration `mapstructure:"access_token_ttl"`
    RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`
    PrivateKeyPath  string        `mapstructure:"private_key_path"`
    PublicKeyPath   string        `mapstructure:"public_key_path"`
    Issuer          string        `mapstructure:"issuer"`
    Audience        string        `mapstructure:"audience"`
}

func (c Config) Validate() error {
    if c.Issuer == "" {
        return errors.New("identity: issuer is required")
    }
    if c.AccessTokenTTL <= 0 {
        return errors.New("identity: access_token_ttl must be positive")
    }
    return nil
}

func New(deps module.Deps, cfg Config) (*module.Module, error) {
    if err := cfg.Validate(); err != nil {
        return nil, err
    }

    tokens, err := token.NewJWTService(cfg.PrivateKeyPath, cfg.PublicKeyPath, cfg.Issuer, cfg.Audience)
    if err != nil {
        return nil, fmt.Errorf("identity: token service: %w", err)
    }

    users := persistence.NewUserRepository(deps.DB)
    authSvc := application.NewAuthService(users, tokens, deps.Clock)
    handler := transport.NewAuthHandler(authSvc, deps.Logger)

    return &module.Module{
        Name:       "identity",
        Routes:     handler.Routes,
        Migrations: migrationsFS,
    }, nil
}
```

### How this fixes each defect

| Defect | Resolution |
|---|---|
| FIDO2 unreachable | `UserStore` arrives via `Deps`/constructor argument — a typed parameter, not a config key. Compile-time. |
| Re-init leak | No re-init. Modules are constructed once at boot; `Close` is called once at shutdown. |
| Silent config defaults | `mapstructure` decoding with `ErrorUnused`/`WeaklyTypedInput` disabled surfaces type errors; `Validate()` rejects bad values at boot. |
| `custom` conflation | Activation is explicit code in the container. Config no longer decides what exists. |
| Adapter indirection | One interface, no adapters. `internal/plugins/adapters.go` deleted. |

---

## What about genuinely optional middleware?

CORS and rate limiting are cross-cutting HTTP concerns, not business modules.
They do not need a plugin framework at all — they need config-driven construction
in the HTTP layer:

```go
// internal/httpx/router.go
func NewRouter(cfg config.Server, sec config.Security, log zerolog.Logger) *chi.Mux {
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(realip.FromTrustedProxies(sec.TrustedProxies))
    r.Use(httpx.Recovery(log))
    r.Use(httpx.RequestLogger(log))
    r.Use(middleware.Timeout(cfg.RequestTimeout))

    if sec.CORS.Enabled {
        r.Use(httpx.CORS(sec.CORS))     // typed config, allowlist required
    }
    if sec.RateLimit.Enabled {
        r.Use(httpx.RateLimit(sec.RateLimit))
    }
    return r
}
```

Roughly 1,100 LOC of framework collapses into an `if` per middleware. Hardening
details in [`../04-security/middleware-hardening.md`](../04-security/middleware-hardening.md).

---

## Migration path

1. ✅ Introduce `internal/module` with the `Deps`/`Module` types — `05b1051`.
2. ✅ Add `modules/identity/module.go` implementing `New()` over the **existing**
   live auth internals, moved rather than rewritten, from
   `internal/{domain,application,infrastructure}/auth/**` — `5f6edfe`, `6d0c1b7`.
3. ✅ Wire it in `cmd/api/container.go`; verify routes respond — `ef76759`,
   `4fdc609`.
4. ✅ Convert CORS and rate limiting to direct construction in `internal/httpx`
   — `de7e242`. `internal/httpx.CORS` and `internal/httpx.NewRateLimiter` take
   typed `config.CORS`/`config.RateLimit`, and `internal/httpx.NewRouter`
   applies each behind an `if`.
5. ✅ Delete `internal/extension/` (including `internal/extension/identity/`),
   `internal/plugin/`, `internal/plugins/`, `internal/plugins/adapters.go`, and
   `examples/extension-demo/` — `de7e242`, **4,023 lines**.
6. ✅ Drop `go-webauthn`, `go-tpm` and `cbor` from `go.mod` — `52464f6`, with
   their transitive `msgp`/`fwd`/`float16`. **`otp` is kept** (TOTP is live in
   `modules/identity/infrastructure/mfa`) and **`barcode` therefore cannot be
   dropped**: `go mod why` reaches it only through `pquerna/otp/totp`.

All six steps have landed. Steps 4–6 shipped as one commit for 4 and 5,
because `internal/plugin/initializer.go` took a `*config.Plugins` — removing
the plugin config surface and removing the frameworks could not be separated
without leaving the tree unbuildable in between.

The config surface went with them: `plugins.middleware`, `plugins.auth`,
`plugins.cache` and `plugins.custom` are replaced by `security.cors` and
`security.rate_limit`. `.env.example` previously advertised `APP_RATELIMIT_*`
and `APP_CORS_*`, which mapped to no Viper key at all and were silently
ignored; the real `APP_SECURITY_*` names replace them, covered by
`TestLoad_SecurityEnvOverrides`.

---

## Related documents

- [ADR-004: Consolidate on a Single Extension Framework](../adr/004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
- [Module layout](module-layout.md)
- [Dependency rules](dependency-rules.md)
- [Dependency injection](../02-composition/dependency-injection.md)
