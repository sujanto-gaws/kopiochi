# Kopiochi Plugin System Guide

## Overview

Kopiochi's plugin system provides a flexible, config-driven way to extend the API with middleware, authentication, cache providers, and custom functionality. Plugins are initialized at startup based on YAML configuration and can be easily enabled/disabled without code changes.

**📚 Want to add your own plugins? See [USER_PLUGINS.md](USER_PLUGINS.md) for a complete guide.**

## Architecture

The plugin system is separated into two parts:

### 1. Plugin Core (`internal/plugin/`) - DO NOT MODIFY

```
internal/plugin/
├── plugin.go              # Core interfaces (Plugin, MiddlewarePlugin, AuthPlugin, etc.)
├── registry.go            # Plugin registry & lifecycle management
├── middleware.go          # Middleware chain builder
└── initializer.go         # Config-driven plugin initialization
```

This is the **infrastructure layer**. Users interact with these interfaces but never modify them.

### 2. Built-in Plugins (`internal/plugins/`) - Use as Examples

```
internal/plugins/
├── register.go            # Built-in plugin registration
├── adapters.go            # Type adapters for registration
├── auth/
│   ├── jwt.go             # JWT authentication plugin
│   └── fido2.go           # FIDO2/WebAuthn passwordless authentication
└── middleware/
    ├── ratelimit.go       # Rate limiting plugin
    └── cors.go            # CORS plugin
```

These are **implementations** that ship with Kopiochi. Use them as examples for your own plugins.

### 3. Your Custom Plugins (`internal/myplugins/`) - Create This

```
internal/myplugins/
├── compression.go         # Your compression plugin
├── apilogger.go           # Your logging plugin
└── customauth.go          # Your custom auth
```

This is where **you add your own plugins**. See [USER_PLUGINS.md](USER_PLUGINS.md) for the complete guide.

## Plugin Types

### 1. MiddlewarePlugin

Applied to all HTTP requests through the chi router middleware chain.

**Interface:**
```go
type MiddlewarePlugin interface {
    Name() string
    Initialize(cfg map[string]interface{}) error
    Close() error
    Middleware() func(http.Handler) http.Handler
}
```

**Built-in Examples:**
- `cors` - Cross-Origin Resource Sharing
- `ratelimit` - Request rate limiting

### 2. AuthPlugin

Specialized plugin for authentication with user context extraction.

**Interface:**
```go
type AuthPlugin interface {
    MiddlewarePlugin
    AuthMiddleware() func(http.Handler) http.Handler
    ExtractUserID(ctx context.Context) string
}
```

**Built-in Examples:**
- `fido2-auth` - FIDO2/WebAuthn passwordless authentication (see [FIDO2_GUIDE.md](FIDO2_GUIDE.md))

> **`jwt-auth` has been removed.** The HS256 JWT plugin
> (`internal/plugins/auth/jwt.go`) was deleted along with its `plugins.auth.jwt`
> config block and its `APP_JWT_SECRET` variable. The API's own authentication
> comes from the `identity` module's RS256 token service
> (`modules/identity/infrastructure/token`), configured under `auth:` in
> `config/default.yaml` — not from an auth plugin. See
> [docs/architectures/04-security/token-architecture.md](docs/architectures/04-security/token-architecture.md).
> `AuthPlugin` itself still exists for `fido2-auth` and third-party plugins.

### 3. CachePlugin

For caching providers (Redis, Memcached, etc.).

**Interface:**
```go
type CachePlugin interface {
    ProviderPlugin
    Get(ctx context.Context, key string) (interface{}, error)
    Set(ctx context.Context, key string, value interface{}) error
    Delete(ctx context.Context, key string) error
}
```

**Planned:**
- Redis cache provider
- Memcached provider

## Configuration

### Basic Setup

Edit `config/default.yaml`:

```yaml
plugins:
  # Middleware plugins - applied in order
  middleware:
    - cors
    - ratelimit
  
  # Authentication plugins.
  #
  # Empty in the shipped config: the only registered auth plugin is
  # fido2-auth, and the API's own authentication comes from the identity
  # module (see the note under AuthPlugin above), not from here.
  auth: {}
  
  # Cache plugins
  cache:
    redis:
      enabled: false
      provider: redis
      config:
        host: "localhost"
        port: 6379
  
  # Custom plugins
  custom:
    myplugin:
      some_option: "value"
```

### Environment Variables

Plugin configuration can be overridden via environment variables:

```bash
# Which middleware plugins are active, in order
APP_PLUGINS_MIDDLEWARE='["cors","ratelimit"]'
```

> The `APP_PLUGINS_AUTH_JWT_SECRET` / `APP_PLUGINS_AUTH_JWT_EXPIRY` variables
> previously listed here no longer exist — see the `jwt-auth` note above. Note
> also that Viper's `AutomaticEnv` does not feed `Unmarshal`: a key reaches the
> typed config only if it has a registered default or an explicit `BindEnv`. See
> [docs/architectures/03-configuration/configuration-model.md](docs/architectures/03-configuration/configuration-model.md).

## Using Built-in Plugins

### JWT Authentication — not a plugin

There is no JWT plugin. Tokens are issued and verified by the `identity`
module's RS256 service, which is wired as a constructor dependency rather than
looked up from the registry — a module that needs authentication cannot be built
without a verifier, so protected routes cannot silently become public.

**Configure it** under `auth:` in `config/default.yaml` (key paths, issuer,
client ID, TTLs, `token_leeway`), and generate the keypair with `make keys`.

**Protect routes** by declaring them inside a module's own protected group; the
module derives its `AuthRequired` middleware from the token service it built.
See `modules/identity/transport/auth.go` for a working example.

**Validate with an explicit token class.** `Validate` takes the class the caller
expects, so an MFA token cannot be used where an access token is required:

```go
claims, err := tokenIssuer.Validate(tokenStr, domain.ClassAccess)
```

Full detail:
[docs/architectures/04-security/token-architecture.md](docs/architectures/04-security/token-architecture.md).

### Rate Limiting

**Enable in config:**
```yaml
plugins:
  middleware:
    - ratelimit
  
  custom:
    ratelimit:
      # Token-bucket parameters:
      rate: 100          # Sustained requests per minute
      burst: 20          # Instantaneous allowance (bucket capacity)
      ttl: "10m"         # Evict buckets idle this long
      max_keys: 100000   # Hard cap on tracked keys
      # Legacy fixed-window pair, still accepted and translated
      # (burst = requests, rate = requests / window):
      # requests: 100
      # window: "1m"
```

**Behavior:**
- Token bucket per client key, refilled continuously — no fixed-window boundary
  to burst across. The lock is released before the downstream handler runs, so
  the limiter never serialises the server.
- Returns `429 Too Many Requests` when the bucket is empty; a rejected request
  does not consume a token.
- Adds `RateLimit-Limit` and `RateLimit-Remaining` on **both** the success and
  the 429 path, plus `Retry-After` on the 429. (These are the standardised
  names; the old `X-RateLimit-*` forms are gone.)
- Keys on the client IP resolved by `internal/middleware.RealIP` — which honours
  `X-Forwarded-For` **only** from CIDRs listed in `server.trusted_proxies`, empty
  by default. The limiter never reads that header itself; doing so was a trivial
  bypass.
- Idle buckets are evicted after `ttl`, and once `max_keys` is reached **new**
  keys are rejected rather than existing ones evicted. A request rejected for
  that reason gets `RateLimit-Remaining: 0` and a fixed 1s `Retry-After` for a
  key that was never admitted.

Detail:
[docs/architectures/04-security/rate-limiting.md](docs/architectures/04-security/rate-limiting.md).

### CORS

**Enable in config:**
```yaml
plugins:
  middleware:
    - cors
  
  custom:
    cors:
      allowed_origins:
        - "https://example.com"
        - "https://app.example.com"
      allowed_methods:
        - "GET"
        - "POST"
        - "PUT"
        - "DELETE"
      allowed_headers:
        - "Authorization"
        - "Content-Type"
      allow_credentials: true
      max_age: 300
```

**Behavior:**
- **Allowlist-only, deny by default.** With no `cors` section (the shipped
  default), no origin is granted `Access-Control-Allow-Origin`. A wildcard `"*"`
  is honoured only if you list it explicitly, and combining it with
  `allow_credentials: true` is rejected at config load — the process will not
  start.
- An allowed `Origin` is echoed back only after an exact allowlist match; an
  unlisted one gets no header and **no 403** — the browser enforces, and 403-ing
  would only break non-browser clients.
- `Vary: Origin` is always set, including on non-CORS responses, so shared caches
  cannot serve one origin's headers to another.
- Only a genuine preflight (`OPTIONS` plus `Access-Control-Request-Method`) is
  answered with 204; any other `OPTIONS` falls through to the router.

Detail:
[docs/architectures/04-security/middleware-hardening.md](docs/architectures/04-security/middleware-hardening.md).

**Defaults:**
- `allowed_origins`: **empty — no origin is allowed.** (This used to default to
  `["*"]`, allow-all. Permissive CORS must now be a deliberate, explicit
  choice.)
- `allowed_methods`: `["GET", "POST", "PUT", "DELETE", "OPTIONS"]`
- `allowed_headers`: Common headers including `Authorization`
- `allow_credentials`: `false`
- `max_age`: `300` (5 minutes)

## Creating Custom Plugins

**See the complete step-by-step guide: [USER_PLUGINS.md](USER_PLUGINS.md)**

Quick overview:

1. **Create your plugin** in `internal/myplugins/` (not in `internal/plugin/`)
2. **Implement interfaces** from `internal/plugin/plugin.go`
3. **Register it** in `internal/plugins/register.go`
4. **Enable it** in `config/default.yaml`

**Examples to follow:**
- `internal/plugins/middleware/ratelimit.go` - Middleware plugin
- `internal/plugins/middleware/cors.go` - Middleware plugin
- `internal/plugins/auth/fido2.go` - Authentication plugin (note: it cannot
  currently be initialised from YAML — see [FIDO2_GUIDE.md](FIDO2_GUIDE.md))

*`internal/plugins/auth/jwt.go` used to be listed here as the authentication
example. It has been deleted.*

**Full examples with explanations:**
- Request logger plugin
- API key authentication plugin
- Compression plugin
- Database-access plugin

All examples are in [USER_PLUGINS.md](USER_PLUGINS.md).

## Programmatic Access

### Access Plugin Registry

```go
// In main.go or anywhere you have access to registry
registry, err := plugin.InitializeFromConfig(&cfg.Plugins)
if err != nil {
    log.Fatal(err)
}

// Get a specific plugin
authPlugin := registry.GetAuth("fido2-auth")
cachePlugin := registry.GetCache("redis")

// Use plugin methods
userID := authPlugin.ExtractUserID(ctx)
```

> For the API's *own* authenticated user, do not go through the registry at all —
> read the claims the identity module's `AuthRequired` middleware put in the
> request context.

### Access from Handlers

Pass the registry to your handlers:

```go
type UserHandler struct {
    service  *appUser.Service
    registry *plugin.Registry
}

func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    // Extract a user identified by an auth *plugin* (e.g. fido2-auth).
    // For the API's own authentication, read the identity module's claims
    // from the request context instead — see the note above.
    userID := h.registry.GetAuth("fido2-auth").ExtractUserID(r.Context())
    
    // Your logic...
}
```

## Plugin Lifecycle

1. **Registration**: Plugins are registered with the registry via `RegisterBuiltinPlugins()`
2. **Initialization**: Config is loaded and plugins are initialized via `InitializeFromConfig()`
3. **Execution**: Middleware plugins are applied to requests through the middleware chain
4. **Shutdown**: Plugins are closed during graceful shutdown

## Best Practices

### ✅ Do

- Use config-driven activation (YAML + env vars)
- Keep plugins focused on single responsibilities
- Use interfaces for loose coupling
- Validate configuration in `Initialize()`
- Clean up resources in `Close()`
- Handle uninitialized state gracefully

### ❌ Don't

- Create import cycles (plugin package → auth → plugin)
- Block in middleware without timeouts
- Store request state in plugin instances (use context)
- Panic on invalid config (return errors instead)
- Forget to close plugins on shutdown

## Troubleshooting

### Plugin Not Initializing

**Check:**
1. Plugin name matches in config and registration
2. No errors in `InitializeFromConfig()`
3. `enabled: true` for auth/cache plugins

**Debug:**
```go
log.Info().Strs("available", registry.List()).Msg("registered plugins")
log.Info().Strs("initialized", registry.ListInitialized()).Msg("active plugins")
```

### Import Cycle Error

**Cause:** Plugin package imports auth/middleware which imports plugin package.

**Solution:** Use adapter pattern (see `register.go`) or define interfaces in the plugin subpackages.

### Middleware Not Applied

**Check:**
1. Plugin name in `plugins.middleware` array
2. Order matters - middleware is applied in array order
3. Check `middlewareChain.Len() > 0`

## Advanced: Custom Plugin Types

### Database Provider Plugin

```go
type DatabasePlugin interface {
    plugin.ProviderPlugin
    GetDB() *sql.DB
    Migrate() error
    Seed() error
}
```

### Event Bus Plugin

```go
type EventBusPlugin interface {
    plugin.ProviderPlugin
    Publish(event string, data interface{}) error
    Subscribe(event string, handler func(interface{})) error
}
```

### Metrics Plugin

```go
type MetricsPlugin interface {
    plugin.MiddlewarePlugin
    RecordMetric(name string, value float64)
    GetMetrics() map[string]interface{}
}
```

## Future Enhancements

- [ ] Redis cache plugin
- [ ] API Key authentication plugin
- [ ] OAuth2 plugin
- [ ] Request validation plugin
- [ ] Audit logging plugin
- [ ] GraphQL plugin
- [ ] WebSocket plugin
- [ ] gRPC plugin support

## Contributing

To contribute a new plugin:

1. Create plugin in `internal/plugin/<category>/`
2. Implement required interfaces
3. Add to `RegisterBuiltinPlugins()`
4. Update documentation
5. Add tests
6. Submit PR

---

For questions or issues, please open an issue on GitHub.
