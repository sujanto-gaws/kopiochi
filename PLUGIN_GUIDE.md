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
- `jwt-auth` - JWT Bearer token authentication
- `fido2-auth` - FIDO2/WebAuthn passwordless authentication (see [FIDO2_GUIDE.md](FIDO2_GUIDE.md))

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
  
  # Authentication plugins
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        secret: "${JWT_SECRET}"  # Use env var
        expiry: "24h"
        issuer: "kopiochi"
  
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
# JWT Auth
APP_PLUGINS_AUTH_JWT_SECRET=my-secret
APP_PLUGINS_AUTH_JWT_EXPIRY=48h

# Rate Limiting
APP_PLUGINS_MIDDLEWARE='["cors","ratelimit"]'
```

## Using Built-in Plugins

### JWT Authentication

**Enable in config:**
```yaml
plugins:
  auth:
    jwt:
      enabled: true
      provider: jwt-auth
      config:
        secret: "your-256-bit-secret"
        expiry: "24h"
```

**Generate tokens:**
```go
// In your handler
jwtPlugin := plugin.GetAuthPlugin(registry, &cfg.Plugins)
if jwtPlugin != nil {
    token, err := jwtPlugin.(*auth.JWTPlugin).GenerateToken(
        "user-123", 
        "John Doe", 
        "john@example.com",
    )
}
```

**Protect routes:**
```go
// The middleware automatically validates tokens
// and adds user info to context
r.Use(jwtPlugin.AuthMiddleware())

// Extract user ID in handler
userID := jwtPlugin.ExtractUserID(r.Context())
```

### Rate Limiting

**Enable in config:**
```yaml
plugins:
  middleware:
    - ratelimit
  
  custom:
    ratelimit:
      requests: 100      # Max requests per window
      window: "1m"       # Time window
```

**Behavior:**
- Tracks requests per client IP
- Returns `429 Too Many Requests` when limit exceeded
- Adds `X-RateLimit-Limit` and `X-RateLimit-Remaining` headers
- Respects `X-Forwarded-For` for proxied requests

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

**Defaults:**
- `allowed_origins`: `["*"]` (allow all)
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
- `internal/plugins/auth/jwt.go` - Authentication plugin
- `internal/plugins/middleware/ratelimit.go` - Middleware plugin
- `internal/plugins/middleware/cors.go` - Middleware plugin

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
authPlugin := registry.GetAuth("jwt-auth")
cachePlugin := registry.GetCache("redis")

// Use plugin methods
userID := authPlugin.ExtractUserID(ctx)
```

### Access from Handlers

Pass the registry to your handlers:

```go
type UserHandler struct {
    service  *appUser.Service
    registry *plugin.Registry
}

func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    // Extract authenticated user
    userID := h.registry.GetAuth("jwt-auth").ExtractUserID(r.Context())
    
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
