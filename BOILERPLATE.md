# Kopiochi Boilerplate Guide

## 🎯 Overview

**Kopiochi** is a production-ready boilerplate for building Go web APIs using **Domain-Driven Design (DDD)** principles. It provides a clean, extensible foundation with modern tooling and best practices.

### Why Use This Boilerplate?

✅ **Clean Architecture** - Strict DDD layer separation  
✅ **Production-Ready** - Graceful shutdown, structured logging, health checks  
✅ **Plugin System** - Extensible middleware, auth, and cache  
⚠️ **Code Generator** - Auto-generate CRUD domains — **currently broken**, see [Generate Your First Domain](#2-generate-your-first-domain)  
✅ **Best Practices** - Dependency injection, interface-based design  
✅ **Modern Stack** - chi router, bun ORM, pgx, viper, zerolog  

---

## 🚀 Quick Start

### Option 1: Use GitHub Template (Recommended)

1. Click **"Use this template"** on GitHub
2. Create your new repository
3. Clone and initialize:

```bash
git clone https://github.com/YOUR_USERNAME/YOUR_PROJECT.git
cd YOUR_PROJECT

# Initialize the boilerplate
make init-project PROJECT=myapi AUTHOR="Your Name"
```

### Option 2: Clone and Initialize

```bash
# Clone the repository
git clone https://github.com/sujanto-gaws/kopiochi.git myapi
cd myapi

# Remove original git history
rm -rf .git

# Initialize with your project details
make init-project PROJECT=myapi AUTHOR="Your Name"

# Make initial commit
git init
git add .
git commit -m "Initial commit: myapi boilerplate"
```

### Option 3: Manual Setup

```bash
# Clone
git clone https://github.com/sujanto-gaws/kopiochi.git myapi
cd myapi
rm -rf .git

# Manually edit these files:
# - go.mod: Change module path
# - config/default.yaml: Update project name
# - .env.example: Update database name
# - README.md: Update project details
# - All *.go files: Replace import paths

# Or use the scripts directly:
./scripts/init.sh --project-name myapi --author "Your Name"
# or on Windows:
.\scripts\init.ps1 -ProjectName myapi -Author "Your Name"
```

---

## 🛠️ Initialization

### Using Make (Linux/Mac)

```bash
# Basic initialization
make init-project PROJECT=myapi AUTHOR="John Doe"

# With custom database name
make init-project PROJECT=myapi AUTHOR="John Doe" DB_NAME=myapi_db

# Keep example User CRUD domain
# (Edit scripts/init.sh and set REMOVE_EXAMPLE=false)
```

### Using PowerShell (Windows)

```powershell
# Basic initialization
.\scripts\init.ps1 -ProjectName myapi -Author "John Doe"

# With custom module path
.\scripts\init.ps1 -ProjectName myapi -Author "John Doe" `
  -ModulePath "github.com/john/myapi" -DBName myapi_db
```

### Using Bash Script (Linux/Mac)

```bash
# Basic initialization
./scripts/init.sh --project-name myapi --author "John Doe"

# With all options
./scripts/init.sh \
  --project-name myapi \
  --author "John Doe" \
  --module-path github.com/john/myapi \
  --db-name myapi_db

# Keep example domain
./scripts/init.sh --project-name myapi --keep-example
```

### What Gets Changed

The initialization script updates:

1. **Module Path** - All Go import paths
2. **Project Name** - README, config, CLI name
3. **Database Name** - Config and .env.example
4. **Author** - LICENSE copyright
5. **Removes Example Domain** - User CRUD (optional)
6. **Resets Git History** - Fresh start (optional)

---

## 📁 Project Structure

```
myapi/
├── cmd/
│   ├── api/              # Main API server entry point
│   │   ├── main.go
│   │   └── container.go  # Composition root: BuildApp() assembles all modules
│   ├── generator/        # Code generator for new domains (currently broken)
│   │   └── main.go
│   └── migrate/          # Database migration CLI
│       └── main.go
├── config/
│   └── default.yaml      # Default configuration
├── migrations/           # Goose SQL migrations
├── modules/              # Business modules (module.Module implementations)
│   └── identity/         # Login, refresh, MFA + access-token middleware
│       ├── module.go
│       ├── domain/
│       ├── application/
│       ├── infrastructure/
│       └── transport/
├── internal/
│   ├── httpx/            # Router, server, route tree, CORS, rate limit, headers
│   ├── lifecycle/        # Teardown stack: one owner per resource, strict LIFO
│   ├── module/           # module.Module / module.Deps contract
│   ├── config/           # Configuration loader
│   ├── db/               # DSN, pgx pool, bun
│   ├── logger/           # Logger setup
│   ├── middleware/       # RealIP (trusted proxies only), request logger
│   ├── platform/         # Shared value types (secret.String)
│   ├── testutil/         # Test helpers
│   └── version/          # Build version string
├── scripts/
│   ├── init.ps1          # Windows initialization script
│   └── init.sh           # Linux/Mac initialization script
├── Makefile              # Development commands
├── .env.example          # Environment variables template
├── Dockerfile            # Docker build configuration
├── go.mod                # Go module definition
└── README.md             # Project documentation
```

---

## 🔧 Development Workflow

### 1. Initial Setup

```bash
# After initialization
make run              # Start the server
make test             # Run tests
make build            # Build binary
```

### 2. Generate Your First Domain

> ### ⚠️ The code generator is currently broken — do not use it
>
> Routing was restructured: `internal/infrastructure/http/routes/` was deleted
> (`routes.Setup` no longer exists, replaced by `httpx.Mount`), and dependency
> injection moved from `cmd/api/main.go` into `BuildApp` in
> `cmd/api/container.go`. `cmd/generator/main.go` was not updated for either
> change. It still compiles — the stale paths are runtime strings, not
> imports — but a run leaves the repository in a broken state:
>
> - `updateRoutes` tries to read `internal/infrastructure/http/routes/routes.go`,
>   which no longer exists. This is reported only as a warning, so the run
>   still reports success.
> - `updateMainGo` looks for repository/service/handler/`routes.Setup` wiring in
>   `cmd/api/main.go`. None of it is there any more, so **no** wiring is added —
>   but the import injection step still fires, leaving an unused
>   `app<Domain> ".../internal/application/<domain>"` import in `main.go`.
>   `go build ./...` then fails with `imported as app<Domain> and not used`.
>
> Repairing the generator is tracked as separate work. Until then, add domains
> by hand: write the layers yourself and register the domain as a
> `module.Module` in `BuildApp` (`cmd/api/container.go`). If you did run it,
> revert the injected import in `cmd/api/main.go`.

You have two options for generating CRUD domains:

#### Option A: Generate from Explicit Fields

```bash
# Generate Product CRUD
make generate DOMAIN=Product FIELDS="name:string,description:string,price:float64,stock:int"
```

#### Option B: Generate from Existing Database Table

If you already have a table in your database, the generator can read the schema directly:

```bash
# Read schema from database (uses DB config from config/default.yaml)
go run cmd/generator/main.go -domain Product -table products

# With custom config file
go run cmd/generator/main.go -domain Product -table products -config config/production.yaml
```

The generator will:
- Connect to your database using the config from `config/default.yaml`
- Read column names, data types, and nullable constraints from `information_schema.columns`
- Automatically skip internal columns (`id`, `created_at`, `updated_at`)
- Map PostgreSQL types to Go types (`varchar` → `string`, `bigint` → `int64`, etc.)
- Convert `snake_case` column names to `CamelCase` for Go field names

#### Generated Files

Both methods generate the following files:

```
modules/product/
├── module.go                        # product.New(deps, cfg) -> *module.Module
├── domain/
│   ├── entity.go                    # Domain entity with validation
│   ├── repository.go                # Repository interface
│   └── dto.go                       # Request/Response DTOs
├── application/
│   └── service.go                   # Business logic over domain interfaces
├── infrastructure/persistence/
│   ├── repository/product_repository.go   # Repository implementation
│   └── models/product_model.go            # Bun model
└── transport/
    └── product_handler.go           # HTTP handlers + Routes(chi.Router)
```

> The generator still emits the old `internal/{domain,application,infrastructure}`
> layout, which is one of several reasons `make generate` is currently broken.
> Those directories no longer exist — all business code lives under `modules/`.

#### Auto-Wiring (broken)

The generator was designed to also update routes and dependency injection
automatically. Both targets have moved and neither step works any more — see
the warning at the start of this section. Route registration and DI wiring must
be done by hand.

### 3. Add Routes

Routes are no longer registered in one central file. A handler owns its own
routes and its own protection through a `Routes(chi.Router)` method, and
`httpx.Mount` mounts each module's `Routes` under `/api/v1`.

Give your handler a `Routes` method (see
`modules/user/transport/user.go` or
`modules/identity/transport/auth.go` for working examples):

```go
// Mounted under /api/v1, so these serve at /api/v1/products, etc.
func (h *ProductHandler) Routes(r chi.Router) {
    r.Group(func(r chi.Router) {
        r.Use(h.authMW) // omit the group if the routes should be public
        r.Post("/products", h.CreateProduct())
        r.Get("/products/{id}", h.GetProduct())
        r.Put("/products/{id}", h.UpdateProduct())
        r.Delete("/products/{id}", h.DeleteProduct())
    })
}
```

Then register the module in the composition root, `cmd/api/container.go`:

```go
func BuildApp(cfg *config.Config, db bun.IDB, log zerolog.Logger) (*App, error) {
    // ... existing modules

    productMod, err := newProductModule(cfg, db)
    if err != nil {
        return nil, fmt.Errorf("build product module: %w", err)
    }
    mods = append(mods, productMod)

    // ...
}
```

where `newProductModule` returns a `*module.Module{Name: "product", Routes: productHandler.Routes}`.
`cmd/api/routes_test.go` (`TestRouteTable`) walks the real router and is the
place to assert your new paths.

#### The module constructor's shape

Every module's constructor has the same shape, and the sameness is the point —
`BuildApp` wires them all the same way:

```go
func New(deps module.Deps, cfg Config) (*module.Module, error)
```

`modules/user/module.go:75` and `modules/identity/module.go:88` are both exactly
that. `Config` is the module's own struct, even when the module has no settings
of its own yet, so a later requirement has somewhere to grow
(`modules/user/module.go:24-31`). Give it a `Validate() error` and call it as
the first statement of `New`: a module whose configuration would make it unsafe
must fail to construct, not construct and serve.

#### Where `h.authMW` comes from — consumer modules take `authn.Middleware`

A module that protects routes does **not** import the module that authenticates.
It declares the dependency in its own `Config`, typed as `authn.Middleware`, and
the composition root supplies it. `modules/user` is the template:

```go
import "github.com/sujanto-gaws/kopiochi/internal/authn"       // module.go:17

type Config struct {
    // Required, not optional: New refuses to build a module that would
    // serve records unauthenticated.
    AuthMiddleware authn.Middleware                            // module.go:53
}

func (c Config) Validate() error {                             // module.go:58
    if c.AuthMiddleware == nil {
        return errors.New("product: auth middleware is required")
    }
    return nil
}
```

and the composition root satisfies it (`cmd/api/container.go:103-105`):

```go
authMW := identitytransport.AuthRequired(jwtSvc)
return user.New(deps, user.Config{AuthMiddleware: authMW})
```

Four rules that fall out of this, all enforced by `make arch` and `make lint`:

- **Take a middleware, never a token verifier.** The module then never learns how
  authentication is implemented, and swapping the implementation is a one-line
  change in `BuildApp`.
- **Pass it down to the handler and let the handler apply it**, as
  `NewUserHandler(svc, authMW)` does (`modules/user/transport/user.go:41`,
  applied at `:199`). Routing and protection are declared together, so an
  unprotected route is visible in the diff.
- **Read the caller through the contract**, not through another module's context
  key: `authn.MustFromContext(r.Context()).Subject` in a handler that is only
  ever routed behind the middleware, `authn.FromContext` where the route is
  reachable both ways.
- **`internal/authn` belongs in `transport` (and the module root), never in
  `domain` or `application`.** `tools/archtest` denies it in both inner layers —
  a use case that reads a `Principal` is taking its caller identity from the HTTP
  request instead of from its own arguments.

In tests, use `testsupport.FakeAuth("u-123")` rather than minting a token; it
returns an `authn.Middleware` and assigns straight into `Config`.

Full contract, the canonical 401 it produces, and the recipe for replacing the
authentication provider: [`docs/architectures/08-authn/README.md`](docs/architectures/08-authn/README.md).

> **Client-visible:** every 401 from that middleware is
> `application/problem+json` with an invariant `detail`. Clients key off
> `status`, never `detail`. See [`CHANGELOG.md`](CHANGELOG.md).

### 4. Configure security middleware

Edit `config/default.yaml`:

```yaml
# Cross-cutting HTTP middleware. Not plugins -- the plugin system was
# deleted in Phase 3. These are typed config, constructed directly in
# internal/httpx.NewRouter behind one `if` each. Both default to off.
security:
  cors:
    enabled: false
    allowed_origins: []        # allowlist-only; empty grants nothing
    allow_credentials: false
  rate_limit:
    enabled: false
    rate: 100                  # requests/minute per resolved client IP
    burst: 100

# The API's own authentication comes from the identity module (RS256).
# The HS256 jwt-auth plugin and the unusable fido2-auth plugin are gone.
auth:
  private_key_path: "keys/private.pem"   # generate with `make keys`
  public_key_path: "keys/public.pem"
  issuer: "kopiochi"
  client_id: "kopiochi"
  access_token_ttl: "15m"
  refresh_token_ttl: "168h"
  token_leeway: "30s"                    # clock-skew allowance on exp
```

Secrets are never written into YAML. `db.password` comes from
`APP_DB_PASSWORD`, and the process refuses to start if it is empty or a known
placeholder. There is no JWT secret to supply — RS256 uses the keypair above.

### 5. Build and Deploy

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Run Docker container
make docker-run
```

---

## 📋 Common Commands

| Command | Description |
|---------|-------------|
| `make help` | Show all available commands |
| `make run` | Start the server |
| `make build` | Build the binary |
| `make test` | Run all tests |
| `make test-coverage` | Run tests with coverage report |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make generate DOMAIN=X FIELDS=Y` | Generate CRUD domain — **broken, see above** |
| `make migrate-up` / `make migrate-status` | Run / inspect database migrations |
| `make docker-build` | Build Docker image |
| `make clean` | Remove build artifacts |

See the full [Makefile](Makefile) for all available commands.

---

## 🎨 Customization

### Adding New Middleware

1. Create in `internal/middleware/`:

```go
// internal/middleware/mycustom.go
package middleware

import "net/http"

func MyCustomMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Your logic
        next.ServeHTTP(w, r)
    })
}
```

2. Add it to the core stack in `internal/httpx/router.go`:

```go
func NewRouter(srv config.Server, sec config.Security, mw ...func(http.Handler) http.Handler) (*chi.Mux, func() error, error) {
    r := chi.NewRouter()

    r.Use(chimw.Recoverer)
    r.Use(chimw.RequestID)
    r.Use(MyCustomMiddleware)  // add here
    // ... SecurityHeaders, corenet.RealIP, Timeout, ZerologRequestLogger
    // ... then CORS and RateLimit, each behind its own config flag

    return r, closeAll, nil
}
```

If the middleware should be optional, give it a typed config struct in
`internal/config` and register it behind an `if`, the way CORS and rate
limiting are. If it owns a resource -- a goroutine, a ticker, a connection --
return a `Close` and add it to the router closer list, so the lifecycle stack
releases it in order.

`NewRouter` also accepts variadic middleware appended after the core stack, so
a caller can pass one in without editing the core stack at all.

### There is no plugin system

Both extension frameworks were deleted in Phase 3.6 — 4,023 lines that nothing
imported. Whatever you were going to build is one of two things:

- **A business capability** — anything owning data, routes or domain rules.
  Write a module under `modules/<name>/` and build it in `BuildApp`. That
  function call is the entire registration mechanism.
- **Cross-cutting HTTP behaviour** — a header, a limiter, a tracer. Put it in
  `internal/httpx`, give it a typed config struct in `internal/config`, and
  register it in `NewRouter` behind an `if`, as CORS and rate limiting are.

Do not build a registry. That is what Phase 3 removed.

### Changing Database

The boilerplate uses PostgreSQL with Bun ORM. To switch:

1. Update `internal/db/database.go` with new driver
2. Change dialect in bun initialization
3. Update `config/default.yaml` accordingly

Supported by Bun: PostgreSQL, MySQL, SQLite, MSSQL

---

## 🚦 CI/CD Setup

### GitHub Actions Example

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  build:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:14
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: myapi_test
        ports:
          - 5432:5432
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.25'
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: go test -v ./...
      env:
        APP_DB_HOST: localhost
        APP_DB_PORT: 5432
        APP_DB_USER: postgres
        APP_DB_PASSWORD: postgres
        APP_DB_NAME: myapi_test
    
    - name: Build
      run: go build -v ./cmd/api
```

---

## 📚 Learning Resources

### Domain-Driven Design

- [Domain-Driven Design Distilled](https://www.infoq.com/minibooks/domain-driven-design-quickly/)
- [Clean Architecture by Robert C. Martin](https://www.amazon.com/Clean-Architecture-Craftsmans-Software-Structure/dp/0134494164)

### Go Best Practices

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go)

### Technologies Used

- [chi router](https://github.com/go-chi/chi)
- [Bun ORM](https://bun.uptrace.dev/)
- [Viper](https://github.com/spf13/viper)
- [Zerolog](https://github.com/rs/zerolog)
- [Cobra CLI](https://github.com/spf13/cobra)

---

## 🤝 Contributing

Found a bug or have a feature request? Please open an issue or submit a PR.

### Development Setup

```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/kopiochi.git
cd kopiochi

# Install dev dependencies
make deps-update

# Run tests
make test

# Make your changes and submit PR
```

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## ☕ Support

If you find this boilerplate useful, consider:

- ⭐ Starring the repository
- 🐛 Reporting bugs
- 💡 Suggesting features
- 🔀 Contributing improvements

---

**Built with ❤️ using Go and Domain-Driven Design principles**
