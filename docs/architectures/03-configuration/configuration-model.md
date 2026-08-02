# Configuration Model

**Status:** Accepted — partially implemented — see [ADR-008](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2 — problems 1 and 2 are resolved
(`acc057d`); problems 3 and 4 are unchanged and belong to Phase 3.5.

---

## Problem 1 (fixed): environment variables silently do not work

*Fixed for the two secrets in `b74b358`: `internal/config/config.go` calls
`BindEnv` explicitly for `db.password` and `plugins.auth.jwt.config.secret`, so
`APP_DB_PASSWORD` and `APP_JWT_SECRET` reach `Unmarshal`. Regression-tested in
`internal/config/config_test.go`.*

***Closed in Phase 2.9 (`acc057d`): `db.user` and `db.name` now have explicit
`BindEnv` calls too (`config.go:115-120`), so `APP_DB_USER` and `APP_DB_NAME`
work. Test: `TestLoad_DBUserAndNameEnvFallback`. The
`plugins.auth.jwt.config.secret` binding was removed in `0cf07d9` along with the
HS256 plugin — there is no such key any more.*** The analysis below is kept as
the explanation of *why* `BindEnv` is required, since it applies to every future
key.

`internal/config/config.go:77-123` sets up Viper:

```go
v.SetEnvPrefix("APP")
v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
v.AutomaticEnv()
```

then unmarshals into a struct:

```go
var cfg Config
if err := v.Unmarshal(&cfg); err != nil {
    return nil, err
}
```

**`AutomaticEnv` does not participate in `Unmarshal`.** Viper's `Unmarshal` builds
its result from `AllSettings()`, which enumerates keys known from defaults, the
config file, and explicit `BindEnv` calls. `AutomaticEnv` only affects direct
`v.Get("key")` lookups — it cannot enumerate the environment, so keys that exist
*only* in the environment are invisible to `Unmarshal`.

Now compare the registered defaults with the `DB` struct:

```go
v.SetDefault("db.host", "localhost")     // ✓ has default
v.SetDefault("db.port", 5432)            // ✓
v.SetDefault("db.sslmode", "disable")    // ✓
v.SetDefault("db.max_conns", 10)         // ✓
v.SetDefault("db.min_conns", 2)          // ✓
//           db.user      — NO DEFAULT
//           db.password  — NO DEFAULT
//           db.name      — NO DEFAULT
```

`APP_DB_PASSWORD` was therefore **silently ignored** unless `db.password` also
appeared in the YAML file. This is very likely *why* the password was hardcoded
in `config/default.yaml:14` — the environment path appeared not to work, because
it did not.

The same applied to every plugin secret: `plugins.auth.jwt.config.secret` has no
default, so it could only come from the file.

*Both secrets were bound explicitly in `b74b358`, which is what allowed it to
strip them from `config/default.yaml`. `acc057d` extended the same treatment to
`db.user` and `db.name`, so all four `APP_DB_*` variables now reach `Unmarshal`.*

## Problem 2 (fixed): no validation

`Load` returns as soon as `Unmarshal` succeeds. Nothing checks that:

- `db.name` is non-empty (an empty database name yields a confusing DSN error later)
- `server.request_timeout` ≤ `server.write_timeout` — **at the time violated**:
  60s vs 30s, so long requests got their connection cut at 30s
- `auth.access_token_ttl` is shorter than `auth.refresh_token_ttl`
- key files at `auth.private_key_path` exist and are readable
- the JWT secret is not the committed placeholder

*Fixed in `acc057d`. `Config.Validate()` exists (`config.go:238-312`) and `Load`
calls it, wrapping the result as `invalid config: %w`, so the process refuses to
start rather than failing on the first request that touches a bad value. Errors
are collected with `errors.Join` rather than returned one at a time, so a
misconfigured deployment sees every problem in one boot attempt.*

*What it checks: `db.name` and `db.user` non-empty; `db.min_conns` ≤
`db.max_conns`; `db.password` not empty and not a known placeholder;
`server.port` in range; `request_timeout` ≤ `write_timeout`; `shutdown_timeout`
≥ `request_timeout`; `auth.issuer` non-empty; the access/MFA/refresh TTLs
positive and access < refresh; `max_failed_attempts` positive;
`token_leeway` non-negative; and CORS `"*"` not combined with
`allow_credentials`. Thirteen tests in `internal/config/config_test.go` cover
these, including `TestLoad_DefaultYAMLIsValid`, which asserts the shipped
`config/default.yaml` passes.*

*The **timeout inversion itself is fixed**, not merely detectable:
`server.request_timeout` went 60s → 25s in both the default registered in
`Load` and `config/default.yaml`, against an unchanged 30s `write_timeout`. See
[lifecycle and shutdown](../02-composition/lifecycle-and-shutdown.md#timeouts).*

*Two checks from the list above did **not** ship: key files at
`auth.private_key_path` are not verified to exist or be readable at config load
(they are opened later, by `NewJWTService`, which does return an error — so this
still fails at boot, just further along and with a less direct message), and
there is no JWT-secret placeholder check because there is no JWT secret any more
(`0cf07d9`). The placeholder machinery that remains guards `db.password`.*

## Problem 3: config decides what code exists — still open

`plugins.middleware: [cors, ratelimit]` and `plugins.custom` drive both
*activation* and *configuration* through `map[string]interface{}`. A typo in a
plugin name produces a runtime error at startup rather than a compile error, and
`initializer.go:72-81` initialises every `custom` entry as a plugin even when it
was only meant to supply config for a middleware. See
[extension framework](../01-modularity/extension-framework.md).

*Unchanged after Phase 2 — this is Phase 3.5's job. Phase 2.3 did add a
narrow exception: `Config.Validate` reaches into `plugins.custom["cors"]` to
extract `allowed_origins`/`allow_credentials` and reject the wildcard-plus-
credentials combination (`corsAllowedOriginsAndCredentials`, `config.go:216-232`).
That is typed validation bolted onto an untyped surface, and it is explicitly a
stopgap: it exists because the combination is dangerous enough to be worth
catching before the surface is fixed properly.*

## Problem 4: unchecked type assertions swallow mistakes — still open

```go
// ratelimit.go:33
if maxReq, ok := cfg["requests"].(float64); ok {
    p.maxRequests = int(maxReq)
} else {
    p.maxRequests = 100     // used silently when the YAML value is a string
}
```

Every plugin repeats this pattern. A `requests: "500"` in YAML yields 100 with no
diagnostic. Viper also returns `int` (not `float64`) for plain YAML integers on
some paths, making the assertion itself unreliable.

*The quoted line no longer exists — `dcc6e5d` rewrote the limiter and there is no
`p.maxRequests` — but **the pattern is unchanged**: `Initialize` still does
`cfg["requests"].(float64)` and falls back silently on a type mismatch. What did
improve is that a value which parses but is nonsensical is now an error rather
than a silent default: non-positive `rate`, `burst`, `ttl`, or `max_keys`, and
unparseable `window`/`ttl` durations, all fail `Initialize` and therefore fail
startup. A wrongly-*typed* value still slips through to the default. Both CORS
and the rate limiter keep the pattern until Phase 3.5 removes the untyped
surface.*

---

## Target design

### Explicit binding for every key

Register a default for **every** field, even if the default is the zero value.
This makes the key known to Viper so environment binding works, and it documents
the full surface in one place.

```go
func setDefaults(v *viper.Viper) {
    // Server
    v.SetDefault("server.host", "0.0.0.0")
    v.SetDefault("server.port", 8080)
    v.SetDefault("server.request_timeout", "25s")

    // Database — every key registered, including the ones with no sensible default
    v.SetDefault("db.host", "localhost")
    v.SetDefault("db.port", 5432)
    v.SetDefault("db.user", "")        // required; validated below
    v.SetDefault("db.password", "")    // required; from env or secret store
    v.SetDefault("db.name", "")        // required
    v.SetDefault("db.sslmode", "require")
}
```

For belt-and-braces on secrets, bind them explicitly as well — `BindEnv` works
with `Unmarshal` regardless of defaults. *Implemented in `b74b358`, extended in
`acc057d`, and trimmed in `0cf07d9`. The live keys are the ones the config struct
actually uses, and the errors are checked rather than discarded:*

```go
// internal/config/config.go:108-120
if err := v.BindEnv("db.password", "APP_DB_PASSWORD"); err != nil {
    return nil, fmt.Errorf("bind db.password env: %w", err)
}
if err := v.BindEnv("db.user", "APP_DB_USER"); err != nil {
    return nil, fmt.Errorf("bind db.user env: %w", err)
}
if err := v.BindEnv("db.name", "APP_DB_NAME"); err != nil {
    return nil, fmt.Errorf("bind db.name env: %w", err)
}
```

*The `plugins.auth.jwt.config.secret` / `APP_JWT_SECRET` binding that used to sit
alongside these was deleted in `0cf07d9` with the HS256 plugin.*

The per-key `SetDefault` sweep above — ADR-008 decision 2, "every key gets an
explicit default registration" — is **still outstanding**. `BindEnv` was used
instead for the three keys that needed it, which fixes those three; it does not
make the next added key work automatically, which is what the sweep buys.

### Precedence

```
flags  >  environment  >  config file  >  defaults
```

Documented, tested, and unsurprising. Flags are bound with
`v.BindPFlags(cmd.Flags())` in the cobra command.

### Validation that fails closed

```go
func (c *Config) Validate() error {
    var errs []error

    if c.DB.Name == "" {
        errs = append(errs, errors.New("db.name is required (set APP_DB_NAME)"))
    }
    if c.DB.User == "" {
        errs = append(errs, errors.New("db.user is required (set APP_DB_USER)"))
    }
    if c.Server.RequestTimeout > c.Server.WriteTimeout {
        errs = append(errs, fmt.Errorf(
            "server.request_timeout (%s) must not exceed server.write_timeout (%s)",
            c.Server.RequestTimeout, c.Server.WriteTimeout))
    }
    if c.Server.ShutdownTimeout < c.Server.RequestTimeout {
        errs = append(errs, errors.New("server.shutdown_timeout must be >= server.request_timeout"))
    }
    if c.Identity.AccessTokenTTL >= c.Identity.RefreshTokenTTL {
        errs = append(errs, errors.New("identity.access_token_ttl must be shorter than refresh_token_ttl"))
    }
    if c.DB.MinConns > c.DB.MaxConns {
        errs = append(errs, errors.New("db.min_conns must not exceed db.max_conns"))
    }

    errs = append(errs, c.Identity.Validate()...)   // modules validate their own config
    return errors.Join(errs...)
}
```

`Load` calls `Validate` and returns the joined error. The process refuses to
start on a misconfiguration instead of failing on the first request that touches
the bad value.

*Shipped in `acc057d`, with more checks than the sketch (see problem 2 above).
Two differences: the TTL keys are `c.Auth.*`, not `c.Identity.*` — there is no
`identity` config section, the module reads `auth.*` — and there is no
`c.Identity.Validate()` delegation, because module-owned config does not exist
yet. `modules/identity/module.go` does call a `cfg.Validate()` of its own on the
module config it is handed, but that is a separate object, not a hook into this
one.*

### Strict decoding — not shipped

```go
if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
    dc.ErrorUnused = true       // unknown key in YAML → error, catches typos
    dc.WeaklyTypedInput = false // "500" is not 500
    dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
        mapstructure.StringToTimeDurationHookFunc(),
        mapstructure.StringToSliceHookFunc(","),
    )
}); err != nil {
    return nil, fmt.Errorf("decode config: %w", err)
}
```

`ErrorUnused` turns a misspelled key — `read_timout` — into a startup error
rather than a silently-defaulted value.

*Not implemented. `Load` still calls plain `v.Unmarshal(&cfg)` with no decoder
options, so a misspelled key is still silently defaulted and `"500"` is still
weakly coerced. This was in scope for Phase 2.9 and did not ship; it is the one
item of 2.9 that is simply outstanding rather than superseded.
`TestUnknownKeyIsRejected` remains unwritten.*

### Typed module config replaces the plugin maps — not shipped

```go
type Config struct {
    Server   Server            `mapstructure:"server"`
    DB       DB                `mapstructure:"db"`
    Log      Log               `mapstructure:"log"`
    Security Security          `mapstructure:"security"`   // CORS, rate limit, proxies
    Identity identity.Config   `mapstructure:"identity"`   // owned by the module
}
```

Named fields rather than embedded structs, so `cfg.Server.Port` reads
unambiguously and there is no risk of promoted-field collisions as the config
grows.

`map[string]interface{}` disappears from the configuration surface entirely.

### Environment naming

| Key | Variable |
|---|---|
| `db.password` | `APP_DB_PASSWORD` |
| `server.port` | `APP_SERVER_PORT` |
| `identity.access_token_ttl` | `APP_IDENTITY_ACCESS_TOKEN_TTL` |
| `security.cors.allowed_origins` | `APP_SECURITY_CORS_ALLOWED_ORIGINS` (comma-separated) |

Rule: uppercase, `.` → `_`, prefix `APP_`. Already implemented by the existing
replacer; it just needs the keys to be registered.

### Configuration files by environment

```
config/
├── default.yaml       committed — NO SECRETS, safe defaults
├── local.yaml         git-ignored — developer overrides
└── example.env        committed — documents every APP_* variable, no values
```

`config/default.yaml` becomes safe to read publicly: no password, no JWT secret.
See [secret management](secret-management.md).

---

## Tests

```go
func TestEnvOverridesFile(t *testing.T) {
    t.Setenv("APP_DB_PASSWORD", "from-env")
    cfg, err := Load("testdata/full.yaml")
    require.NoError(t, err)
    require.Equal(t, "from-env", cfg.DB.Password)   // failed before b74b358
}

func TestValidateRejectsTimeoutInversion(t *testing.T) {
    cfg := valid(t)
    cfg.Server.RequestTimeout = 60 * time.Second
    cfg.Server.WriteTimeout = 30 * time.Second
    require.Error(t, cfg.Validate())
}

func TestUnknownKeyIsRejected(t *testing.T) {
    _, err := Load("testdata/typo.yaml")           // contains `read_timout`
    require.ErrorContains(t, err, "read_timout")
}
```

`TestEnvOverridesFile` is the regression test for the defect that made secrets
end up in the repository. It shipped in `b74b358` as
`internal/config/config_test.go`, split into `TestLoad_DBPasswordEnvFallback`,
`TestLoad_EnvOverridesFileValue`, and `TestLoad_JWTSecretEnvFallback`, and it
passes — *`TestLoad_JWTSecretEnvFallback` was removed in `0cf07d9` with the key
it tested, and `TestLoad_DBUserAndNameEnvFallback` was added in `acc057d`.*

*`TestValidate_RejectsTimeoutInversion` shipped in `acc057d` and passes, along
with twelve sibling validation tests. `TestUnknownKeyIsRejected` is still not
written, because strict decoding is still not implemented.*

*Also new in `acc057d`: `TestLoad_RejectsPlaceholderSecrets` (the server refuses
to start on a `CHANGEME*` or historically-committed `db.password`) and
`TestLoad_DefaultYAMLIsValid`, which loads the real `config/default.yaml` and
asserts it passes `Validate` — so the shipped defaults cannot drift into a state
that fails at boot without a test noticing.*

---

## Related documents

- [ADR-008: Configuration Precedence and Secret Handling](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
- [Secret management](secret-management.md)
- [Lifecycle and shutdown](../02-composition/lifecycle-and-shutdown.md)
