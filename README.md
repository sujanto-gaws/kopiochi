# Kopiochi

> **🔥 Production-Ready DDD Go API Boilerplate**

A **Domain-Driven Design (DDD)** Go web API boilerplate built with modern, production-ready technologies. Start your next Go project with clean architecture, plugin system, and code generation in seconds.

**[📚 View Boilerplate Guide](BOILERPLATE.md)** | **[🔌 Plugin Documentation](PLUGIN_GUIDE.md)** | **[📝 Swagger API Documentation](SWAGGER.md)** | **[🗄️ Database Migrations](MIGRATIONS.md)**

## 🏗️ Architecture

This project follows **Domain-Driven Design (DDD)** principles with clear separation of concerns:

```
internal/
├── domain/           # Business entities, rules, and domain interfaces
├── application/      # Use cases and application services
└── infrastructure/   # External concerns (HTTP, persistence, etc.)

modules/             # Self-contained business modules, each with the same
└── identity/        # domain/application/infrastructure/transport split
```

### Layer Responsibilities

| Layer | Purpose |
|-------|---------|
| **Domain** | Core business logic, entities, and repository interfaces |
| **Application** | Use case orchestration and application services |
| **Infrastructure** | HTTP handlers, database repositories, external integrations |

Newer features live under `modules/<name>/` as a `module.Module`: they own
their own routes and route protection, and the composition root
(`BuildApp` in `cmd/api/container.go`) assembles them.

## 🚀 Tech Stack

| Component | Technology |
|-----------|------------|
| **Router** | [chi v5](https://github.com/go-chi/chi) - Lightweight, idiomatic HTTP router |
| **Database** | [bun](https://github.com/uptrace/bun) - SQL ORM for Go |
| **Driver** | [pgx v5](https://github.com/jackc/pgx) - PostgreSQL driver |
| **Config** | [viper](https://github.com/spf13/viper) - Configuration management |
| **CLI** | [cobra](https://github.com/spf13/cobra) - Command-line interface framework |
| **Logging** | [zerolog](https://github.com/rs/zerolog) - Fast, structured logging |

## 📋 Features

- ✅ **Domain-Driven Design** - Clean architecture with separation of concerns
- ✅ **Dependency Injection** - Loose coupling between layers
- ✅ **Plugin System** - Extensible middleware, auth, and cache plugins
- ✅ **Swagger/OpenAPI Documentation** - Auto-generated API documentation
- ✅ **Database Migrations** - Version-controlled schema management with Goose
- ✅ **PostgreSQL** - Production-ready database with connection pooling
- ✅ **Structured Logging** - JSON or console format with configurable levels
- ✅ **Liveness & Readiness Probes** - `/healthz` and `/readyz` (real DB ping) for Kubernetes/container orchestration
- ✅ **Environment Configuration** - Flexible config via YAML, env vars, or both
- ✅ **Docker Support** - Multi-stage build for optimized container images

## 🛠️ Getting Started

### Quick Start (Recommended)

```bash
# 1. Use as GitHub template or clone
git clone https://github.com/sujanto-gaws/kopiochi.git myapi
cd myapi
rm -rf .git

# 2. Initialize with your project name
make init-project PROJECT=myapi AUTHOR="Your Name"
# Or on Windows:
# .\scripts\init.ps1 -ProjectName myapi -Author "Your Name"

# 3. Start developing
make run
```

**📖 See full setup instructions: [BOILERPLATE.md](BOILERPLATE.md)**

### Prerequisites

- Go 1.25+
- PostgreSQL 14+
- Docker (optional)

### Installation

```bash
# Clone the repository
git clone https://github.com/sujanto-gaws/kopiochi.git
cd kopiochi

# Initialize as your project
make init-project PROJECT=myapi AUTHOR="Your Name"

# Copy environment example
cp .env.example .env

# Update configuration as needed
# Edit .env or config/default.yaml
```

### First-Time Setup

Two things must be done before the server will start correctly:

1. **Generate a JWT signing keypair.** `config/default.yaml` points to
   `keys/private.pem` / `keys/public.pem`, which are gitignored and not
   included in the repo:

   ```bash
   make keys
   ```

   This refuses to run if `keys/private.pem` or `keys/public.pem` already
   exist, so it won't silently invalidate live tokens.

   > ⚠️ **Security note:** this repository previously had a keypair committed
   > to git history at the repo root (`private.pem`, `public.pem`). That
   > keypair is compromised (public in version control history) and must
   > never be used — always generate your own with `make keys`.

2. **Set `APP_DB_PASSWORD`.** The database password is no longer stored in
   `config/default.yaml`; it must be supplied via the environment (e.g. in
   your `.env` file or shell), otherwise the app will try to connect with an
   empty password.

### Running Locally

```bash
# Start the server
make run
# or
go run ./cmd/api serve

# Or with custom config
go run ./cmd/api serve --config config/default.yaml
```

## 💻 Development Workflow

### Generate New Domain (CRUD)

> ⚠️ **`make generate` is currently broken — do not use it.** The generator
> still writes to `internal/infrastructure/http/routes/routes.go`, a file that
> was deleted when routing moved to `internal/httpx`, and it still injects
> wiring into `cmd/api/main.go`, where the composition root no longer lives
> (it is now `BuildApp` in `cmd/api/container.go`). A run generates the seven
> domain files, prints a warning that it could not update routes, and then
> adds an **unused import to `cmd/api/main.go` that breaks `go build ./...`**.
> Repairing the generator is tracked as separate work; until then, add new
> domains by hand and wire them into `BuildApp` as a `module.Module`.

```bash
# Generate Product domain with all CRUD operations (see warning above)
make generate DOMAIN=Product FIELDS="name:string,description:string,price:float64,stock:int"

# This creates:
# ✅ Domain entity with validation
# ✅ Repository interface
# ✅ DTOs (Request/Response)
# ✅ Application service
# ✅ Database model & repository
# ✅ HTTP handlers
# ❌ Route registration and DI wiring — broken, see warning above
```

### Common Commands

```bash
make help             # Show all commands
make run              # Start server
make build            # Build binary
make test             # Run tests
make test-coverage    # Run tests with coverage
make lint             # Run linter
make fmt              # Format code
make swagger-docs     # Generate swagger documentation
make migrate-up       # Run database migrations
make migrate-status   # Check migration status
make docker-build     # Build Docker image
```

See [BOILERPLATE.md](BOILERPLATE.md) for complete workflow documentation.

### Running with Docker

```bash
# Build the image
docker build -t kopiochi .

# Run the container
docker run -p 8080:8080 --env-file .env kopiochi
```

## 📡 API Endpoints

Business routes are mounted under `/api/v1`; operational routes are
unversioned. The authoritative list is `TestRouteTable` in
`cmd/api/routes_test.go`, which walks the real chi tree built by
`httpx.Mount`.

**Operational (unversioned, unauthenticated)**

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/healthz` | Liveness — process is up; touches no dependency |
| `GET` | `/readyz` | Readiness — pings the database pool; `503` when unreachable |
| `GET` | `/health` | **Deprecated** alias for `/healthz`; will be removed |
| `GET` | `/swagger/*` | Swagger UI and `doc.json` |

**Auth (`modules/identity`)**

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/auth/login` | none | Log in; `200` with tokens, or `202` + `mfa_token` when MFA is enabled |
| `POST` | `/api/v1/auth/refresh` | none | Exchange a refresh token (JSON body or `refresh_token` cookie) for new tokens |
| `POST` | `/api/v1/auth/mfa/verify` | `Bearer <mfa_token>` | Complete the MFA login step with a TOTP or backup code |
| `POST` | `/api/v1/auth/logout` | `Bearer <access_token>` | Revoke the caller's refresh tokens and clear the cookie |
| `POST` | `/api/v1/auth/mfa/setup` | `Bearer <access_token>` | Start MFA enrolment; returns a TOTP secret and QR code URL |
| `POST` | `/api/v1/auth/mfa/setup/verify` | `Bearer <access_token>` | Confirm enrolment with a TOTP code; returns backup codes |

**Users** — every user route requires a valid access token.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/users` | `Bearer <access_token>` | Create a new user |
| `GET` | `/api/v1/users/{id}` | `Bearer <access_token>` | Get user by ID |
| `PUT` | `/api/v1/users/{id}` | `Bearer <access_token>` | Update a user |
| `DELETE` | `/api/v1/users/{id}` | `Bearer <access_token>` | Delete a user |

> The refresh token is returned as an HttpOnly cookie, not in the JSON body —
> `access_token` is the only token in the response payload.

### 📚 API Documentation (Swagger)

This project includes auto-generated Swagger/OpenAPI documentation for all endpoints.

**Quick Start:**
```bash
# 1. Generate docs
make swagger-docs

# 2. Start server
make run

# 3. Open browser
# Navigate to: http://localhost:8080/swagger/index.html
```

**📖 See complete guide: [SWAGGER.md](SWAGGER.md)**

#### What You Can Do
- ✅ Browse all API endpoints with interactive UI
- ✅ Test endpoints directly from the browser
- ✅ View detailed request/response schemas
- ✅ Authenticate with JWT to test protected endpoints
- ✅ Export code examples in multiple languages
- ✅ Download OpenAPI spec (JSON/YAML)

### Example Requests

**Log in** (obtain an access token — every user route needs one):
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"john","password":"s3cret"}'
# => {"access_token":"eyJ...","token_type":"Bearer","expires_in":900}
# The refresh token is set as an HttpOnly `refresh_token` cookie.
```

**Create User:**
```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"John Doe","email":"john@example.com"}'
```

**Get User:**
```bash
curl http://localhost:8080/api/v1/users/1 \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Omitting the `Authorization` header on a user route returns `401`.

**Liveness / readiness:**
```bash
curl http://localhost:8080/healthz   # 200 while the process is serving
curl http://localhost:8080/readyz    # 200 only when the database answers a ping
```

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_SERVER_HOST` | `0.0.0.0` | Server bind address |
| `APP_SERVER_PORT` | `8080` | Server port |
| `APP_DB_HOST` | `localhost` | PostgreSQL host |
| `APP_DB_PORT` | `5432` | PostgreSQL port |
| `APP_DB_USER` | `postgres` | Database user |
| `APP_DB_PASSWORD` | *(none — required)* | Database password. Not read from `config/default.yaml`; must be set via env. |
| `APP_DB_NAME` | `kopiochi` | Database name |
| `APP_DB_SSLMODE` | `disable` | SSL mode for database |
| `APP_DB_MAX_CONNS` | `10` | Maximum database connections |
| `APP_DB_MIN_CONNS` | `2` | Minimum database connections |
| `APP_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `APP_LOG_FORMAT` | `json` | Log format (json, console) |

## �️ Database Migrations

This project uses [Goose](https://github.com/pressly/goose) for version-controlled database migrations.

### Quick Start

```bash
# Run all pending migrations
make migrate-up

# Check migration status
make migrate-status

# Create a new migration
make migrate-create NAME=create_products_table

# Rollback last migration
make migrate-down
```

### Example Migration Commands

```bash
# Run migrations
make migrate-up

# Create migration
make migrate-create NAME=add_users_index

# Check status
make migrate-status
```

**📖 See complete guide: [MIGRATIONS.md](MIGRATIONS.md)**

## �📁 Project Structure

```
kopiochi/
├── cmd/
│   ├── api/
│   │   ├── main.go          # Application entry point (cobra `serve`)
│   │   └── container.go     # Composition root: BuildApp() assembles all modules
│   ├── generator/
│   │   └── main.go          # Code generator for CRUD operations (currently broken)
│   └── migrate/
│       └── main.go          # Database migration CLI
├── config/
│   └── default.yaml         # Default configuration
├── migrations/              # Database migrations (Goose)
│   ├── 00001_create_users.sql
│   ├── 00002_create_products.sql
│   ├── 00003_create_auth_users.sql
│   ├── 00004_create_auth_refresh_tokens.sql
│   └── 00005_create_auth_mfa_backup_codes.sql
├── modules/                 # Business modules (module.Module implementations)
│   └── identity/            # Login, refresh, MFA, access-token middleware
│       ├── module.go        # identity.New() → *module.Module
│       ├── domain/
│       ├── application/
│       ├── infrastructure/  # hasher, mfa, token, persistence
│       └── transport/       # HTTP handlers + Routes()
├── internal/
│   ├── application/         # Application layer (use cases)
│   │   └── user/
│   │       └── service.go
│   ├── config/              # Configuration loading
│   ├── db/                  # Database connection setup
│   ├── domain/              # Domain layer (entities, interfaces)
│   │   ├── user/
│   │   │   ├── user.go
│   │   │   ├── repository.go
│   │   │   └── service.go
│   │   └── ofbizuser/
│   ├── httpx/               # Route tree: /healthz, /readyz, /swagger + /api/v1 mount
│   │   ├── routes.go        # httpx.Mount(r, modules, deps)
│   │   └── health.go
│   ├── module/              # module.Module / module.Deps contract
│   ├── infrastructure/      # Infrastructure layer
│   │   ├── http/
│   │   │   ├── handlers/    # Legacy user handler (owns its own Routes())
│   │   │   └── server/      # NewRouter, NewServer, Run, graceful shutdown
│   │   └── persistence/
│   │       └── repository/
│   ├── extension/           # Extension framework
│   ├── plugin/              # Plugin contracts, registry, initializer
│   ├── plugins/             # Built-in plugins (auth, middleware) + register.go
│   ├── logger/              # Logger initialization
│   ├── middleware/          # HTTP middleware (zerolog request logger)
│   ├── testutil/            # Test helpers (throwaway Postgres, etc.)
│   └── version/             # Build version string
├── .env.example             # Environment variables template
├── Dockerfile               # Docker build configuration
├── go.mod                   # Go module definition
└── README.md
```

### How routing is wired

`cmd/api/main.go` builds the router with `server.NewRouter`, calls
`BuildApp` (`cmd/api/container.go`) to assemble the modules, then hands both to
`httpx.Mount(r, app.Modules, httpx.Deps{Pinger: pool})`. `Mount` registers the
operational endpoints and mounts every module's `Routes` under `/api/v1`. Each
module declares its own auth middleware, so a module can never be mounted
unprotected by accident.

## 🔌 Plugin System

Kopiochi includes a powerful, config-driven plugin system that allows you to easily extend functionality without code changes.

### Available Plugins

| Plugin | Type | Description |
|--------|------|-------------|
| `jwt-auth` | Authentication | JWT-based authentication with token generation |
| `fido2-auth` | Authentication | FIDO2/WebAuthn passkeys — **registered but not usable**, see [FIDO2_GUIDE.md](FIDO2_GUIDE.md) |
| `ratelimit` | Middleware | Request rate limiting per client IP |
| `cors` | Middleware | Cross-Origin Resource Sharing support |

> Note: the request-authentication used by the API's own routes comes from the
> `identity` module, not from an auth plugin. The auth plugins above are
> optional extras.

### Configuration

Enable and configure plugins in `config/default.yaml`:

```yaml
plugins:
  # Middleware plugins (applied in order)
  middleware:
    - cors
    - ratelimit
  
  # Authentication plugins
  auth:
    jwt:
      enabled: false
      provider: jwt-auth
      config:
        # secret intentionally omitted - set via APP_JWT_SECRET env var
        expiry: "24h"
        issuer: "kopiochi"
  
  # Cache plugins. `config/default.yaml` ships a disabled `redis` stanza, but
  # no cache plugin is registered in internal/plugins/register.go yet — leave
  # it disabled.
  cache:
    redis:
      enabled: false
      provider: redis
  
  # Custom plugins
  custom: {}
```

Enabling `fido2-auth` here will fail startup — see the compatibility note at the
top of [FIDO2_GUIDE.md](FIDO2_GUIDE.md).

### Creating Custom Plugins

1. Create your plugin in `internal/plugins/<category>/` (`auth/` or `middleware/`)
2. Implement the required interface:
   - **MiddlewarePlugin**: `Name()`, `Initialize()`, `Close()`, `Middleware()`
   - **AuthPlugin**: All middleware methods + `ExtractUserID()`
   - **CachePlugin**: `Get()`, `Set()`, `Delete()`
3. Register it in `internal/plugins/register.go`

Example:
```go
// internal/plugins/middleware/myplugin.go
package middleware

type MyPlugin struct { /* ... */ }

func (p *MyPlugin) Name() string { return "myplugin" }
func (p *MyPlugin) Initialize(cfg map[string]interface{}) error { /* ... */ }
func (p *MyPlugin) Close() error { /* ... */ }
func (p *MyPlugin) Middleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Your middleware logic
            next.ServeHTTP(w, r)
        })
    }
}
func (p *MyPlugin) Provider() interface{} { return p }
```

Then register it:
```go
// internal/plugins/register.go
func RegisterBuiltinPlugins(registry *plugin.Registry) {
    // ... existing plugins
    registry.Register("myplugin", func() Plugin {
        return &middlewarePluginAdapter{middleware.NewMyPlugin()}
    })
}
```

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/domain/user/...
```

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

**Built with ❤️ using Go and Domain-Driven Design principles**
