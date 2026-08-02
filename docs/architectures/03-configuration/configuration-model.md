# Configuration Model

**Status:** Proposed — see [ADR-008](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
**Date:** 2026-08-02

---

## Problem 1: environment variables silently do not work

`internal/config/config.go:76-123` sets up Viper:

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

`APP_DB_PASSWORD` is therefore **silently ignored** unless `db.password` also
appears in the YAML file. This is very likely *why* the password was hardcoded in
`config/default.yaml:14` — the environment path appeared not to work, because it
does not.

The same applies to every plugin secret: `plugins.auth.jwt.config.secret` has no
default, so it can only come from the file.

## Problem 2: no validation

`Load` returns as soon as `Unmarshal` succeeds. Nothing checks that:

- `db.name` is non-empty (an empty database name yields a confusing DSN error later)
- `server.request_timeout` ≤ `server.write_timeout` — **currently violated**:
  60s vs 30s, so long requests get their connection cut at 30s
- `auth.access_token_ttl` is shorter than `auth.refresh_token_ttl`
- key files at `auth.private_key_path` exist and are readable
- the JWT secret is not the committed placeholder

## Problem 3: config decides what code exists

`plugins.middleware: [cors, ratelimit]` and `plugins.custom` drive both
*activation* and *configuration* through `map[string]interface{}`. A typo in a
plugin name produces a runtime error at startup rather than a compile error, and
`initializer.go:72-81` initialises every `custom` entry as a plugin even when it
was only meant to supply config for a middleware. See
[extension framework](../01-modularity/extension-framework.md).

## Problem 4: unchecked type assertions swallow mistakes

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
with `Unmarshal` regardless of defaults:

```go
_ = v.BindEnv("db.password", "APP_DB_PASSWORD")
_ = v.BindEnv("identity.jwt_secret", "APP_IDENTITY_JWT_SECRET")
```

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

### Strict decoding

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

### Typed module config replaces the plugin maps

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
    require.Equal(t, "from-env", cfg.DB.Password)   // fails against today's code
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
end up in the repository.

---

## Related documents

- [ADR-008: Configuration Precedence and Secret Handling](../adr/008%20-%20Configuration%20Precedence%20and%20Secret%20Handling.md)
- [Secret management](secret-management.md)
- [Lifecycle and shutdown](../02-composition/lifecycle-and-shutdown.md)
