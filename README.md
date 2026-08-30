# Kopiochi

> **🔥 Production-Ready DDD Go API Boilerplate**

A **Domain-Driven Design (DDD)** Go web API boilerplate built with modern,
production-ready technologies: self-contained business modules, an explicit
compile-time composition root, and mechanically enforced dependency rules.

**[📚 Boilerplate Guide](BOILERPLATE.md)** | **[🖧 HTTP Server & Router](SERVER.md)** | **[📝 Swagger](SWAGGER.md)** | **[🗄️ Migrations](MIGRATIONS.md)** | **[🏛️ Architecture](docs/architectures/README.md)**

## 🏗️ Architecture

This project follows **Domain-Driven Design (DDD)** principles with clear separation of concerns:

```
cmd/
├── api/              # composition root (BuildApp) + serve/healthcheck commands
├── migrate/          # goose runner — never links into the server
└── generator/        # CRUD scaffolding (currently broken, see below)

modules/              # self-contained business capabilities
├── identity/         # login, tokens, MFA, refresh rotation
│   ├── domain/       # entities + repository interfaces; no bun, no chi
│   ├── application/  # use cases over those interfaces
│   ├── infrastructure/  # bun models, repositories, JWT, bcrypt, TOTP
│   └── transport/    # HTTP handlers + Routes(chi.Router)
├── user/             # profile CRUD, same four-layer split
└── ofbiz/            # Apache OFBiz entity compatibility

internal/             # shared kernel — knows nothing about any module
├── httpx/            # router, server, middleware, problem+json, health
├── config/           # typed config, loaded and validated once at boot
├── db/               # pool, DSN, transactions, error translation
├── lifecycle/        # LIFO teardown stack
├── audit/            # security event stream
├── metrics/          # Prometheus collectors
├── module/           # the Module contract
└── platform/secret/  # self-redacting secret type

tools/                # build-time checks: archtest, coverage, schemacheck
```

`internal/domain/`, `internal/application/` and `internal/infrastructure/` no
longer exist — every business capability moved into `modules/` in Phase 3, and
`internal/` is now a flat shared kernel.

### Layer responsibilities, inside a module

| Layer | Purpose | May import |
|-------|---------|-----------|
| **domain** | Entities, invariants, repository interfaces | stdlib and `internal/platform` only |
| **application** | Use cases over domain interfaces | its own domain |
| **infrastructure** | bun models, repositories, external clients | domain, `internal/**` |
| **transport** | HTTP handlers and the module's route table | application, domain, `internal/authn`, `internal/httpx` |

These are not conventions — they are enforced. `depguard` and the tests in
`tools/archtest` walk the real import graph, so a domain package that imports
the ORM fails the build rather than a review. Modules may not import each
other, and `internal/**` may not import `modules/**`; only `cmd/**` sees both.
See [dependency rules](docs/architectures/01-modularity/dependency-rules.md).

The composition root (`BuildApp` in `cmd/api/container.go`) assembles every
module. Adding one is a function call there — that is the whole registration
mechanism.

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
- ✅ **Enforced dependency rules** - depguard plus architecture tests over the real import graph; a layering violation fails the build
- ✅ **Refresh-token reuse detection** - a replayed token revokes its whole family
- ✅ **Key rotation** - `kid` headers and a JWKS endpoint, with an overlap window so rotating does not log everyone out
- ✅ **Audit event stream** - security events on their own stream, never below warn level
- ✅ **Prometheus metrics** - on a separate admin port, labelled by route pattern
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
make run              # Start the server
make build            # Build (development: symbols kept)
make build-release    # Build stripped and trimmed
make size             # Report the release binary size
make test             # Run tests
make coverage-check   # Enforce the coverage floors and ratchet
make arch             # Check the dependency rules
make lint             # Run golangci-lint
make fmt              # Format code
make swagger-run      # Generate the spec and run with the UI mounted
make compose-up       # Postgres + migrations + API
make migrate-up       # Run database migrations
make migrate-status   # Check migration status
make docker-build     # Build Docker image
```

See [BOILERPLATE.md](BOILERPLATE.md) for complete workflow documentation.

### Running with Docker

See [Running locally](#-running-locally) for the full stack. For the image
alone:

```bash
make docker-build
make docker-run     # requires a .env; refuses to run without one
```

The image is `distroless/static:nonroot` — no shell, no package manager,
running as uid 65532 — and `.dockerignore` keeps `keys/`, `*.pem` and `.env`
out of the build context entirely.

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
| `GET` | `/swagger/*` | Swagger UI — **only in a `-tags swagger` build**; otherwise a problem document explaining the flag |

**Auth (`modules/identity`)**

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/auth/login` | none | Log in; `200` with tokens, or `202` + `mfa_token` when MFA is enabled |
| `POST` | `/api/v1/auth/refresh` | none | Exchange a refresh token (JSON body or `refresh_token` cookie) for new tokens |
| `POST` | `/api/v1/auth/mfa/verify` | `Bearer <mfa_token>` | Complete the MFA login step with a TOTP or backup code |
| `POST` | `/api/v1/auth/logout` | `Bearer <access_token>` | Revoke the caller's refresh tokens and clear the cookie |
| `POST` | `/api/v1/auth/mfa/setup` | `Bearer <access_token>` | Start MFA enrolment; returns a TOTP secret and QR code URL |
| `POST` | `/api/v1/auth/mfa/setup/verify` | `Bearer <access_token>` | Confirm enrolment with a TOTP code; returns backup codes |

**Profile** — the profile of the authenticated caller. Both routes require a
valid access token, and neither takes an id.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/v1/users/me` | `Bearer <access_token>` | Create the caller's own profile. No body. Idempotent |
| `GET` | `/api/v1/users/me` | `Bearer <access_token>` | Get the caller's own profile |

> **There is no route that names another user, and that is the design.** A
> profile is keyed by the identity it belongs to (`auth_users.id`), so the id a
> handler acts on is the one in the caller's own token — there is nothing else
> to pass. Earlier versions exposed `POST /api/v1/users` and
> `GET|PUT|DELETE /api/v1/users/{id}`, where any valid token could read,
> overwrite or delete any other user's record.
>
> The profile carries an id and timestamps and nothing else. A name and an email
> live on the identity, which is their single source of truth.

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

**Create your profile** (no body, and it is the caller's own — there is no id
to supply):
```bash
curl -X POST http://localhost:8080/api/v1/users/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"
# => {"id":"3f1b8a54-...","created_at":"...","updated_at":"..."}
```

Calling it twice is not an error: it returns the same profile.

**Get your profile:**
```bash
curl http://localhost:8080/api/v1/users/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Omitting the `Authorization` header on a profile route returns `401`. There is
no request that fetches somebody else's — the id comes from the token.

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

## 📁 Project Structure

```
kopiochi/
├── cmd/
│   ├── api/
│   │   ├── main.go          # Entry point (cobra `serve`) — linear startup, LIFO teardown
│   │   └── container.go     # Composition root: BuildApp() assembles all modules
│   ├── generator/
│   │   └── main.go          # Code generator for CRUD operations (currently broken)
│   └── migrate/
│       └── main.go          # Database migration CLI
├── config/
│   └── default.yaml         # Default configuration
├── migrations/              # Database migrations (Goose)
├── modules/                 # Business capabilities. One module = one module.Module
│   ├── identity/            # Login, refresh, MFA, access-token middleware
│   │   ├── module.go        # identity.New() → *module.Module
│   │   ├── domain/          # Entities and repository interfaces. No bun, no chi
│   │   ├── application/     # Use cases over domain interfaces
│   │   ├── infrastructure/  # hasher, mfa, token, persistence
│   │   └── transport/       # HTTP handlers + Routes()
│   ├── user/                # Profile CRUD; same five-part shape
│   │   └── module.go        # user.New() — fails closed without auth middleware
│   └── ofbiz/               # Apache OFBiz UserLogin compatibility layer (no transport)
├── internal/                # Shared kernel. Must NOT import modules/** (enforced)
│   ├── config/              # Viper loading, typed config, Validate()
│   ├── db/                  # DSN, pgx pool, bun, the *sql.DB the migrator uses
│   ├── httpx/               # Everything HTTP that is not a module
│   │   ├── router.go        # NewRouter: core middleware + CORS/rate limit if enabled
│   │   ├── routes.go        # Mount: /healthz, /readyz, /swagger + /api/v1
│   │   ├── server.go        # Serve(ctx) error — no log.Fatal
│   │   ├── health.go        # Liveness and readiness (readiness knows about draining)
│   │   ├── cors.go          # Allowlist-only CORS
│   │   ├── ratelimit.go     # Token bucket, per resolved client IP
│   │   └── security_headers.go
│   ├── lifecycle/           # Stack: one owner per resource, strict LIFO teardown
│   ├── module/              # module.Module / module.Deps contract
│   ├── middleware/          # RealIP (trusted proxies only), zerolog request logger
│   ├── platform/            # Shared value types
│   │   └── secret/          # secret.String — redacts itself everywhere but Reveal()
│   ├── logger/              # Logger initialization
│   ├── testutil/            # Test helpers (throwaway Postgres, etc.)
│   └── version/             # Build version string
├── tools/                   # Cross-cutting checks. May import every module
│   ├── archtest/            # Architecture rules as tests (run with -count=1)
│   └── schemacheck/         # Bun models vs. migrated schema drift
├── .github/workflows/ci.yml # gofmt, build, vet, test, arch rules, golangci-lint
├── .golangci.yml            # depguard rules for the dependency boundaries
├── .env.example             # Environment variables template
├── Dockerfile               # Docker build configuration
├── go.mod                   # Go module definition
└── README.md
```

### How startup is wired

`cmd/api/serve()` runs seven phases in order, returning an error at any of
them rather than exiting:

1. **Config** — `config.Load` validates before anything is allocated.
2. **Logger.**
3. **Database** — `db.Open(ctx, cfg.DB)`, bounded by `db.startup_timeout`, so
   an unreachable database fails boot fast instead of hanging.
4. **Modules** — `BuildApp` (`cmd/api/container.go`). It refuses to return an
   application with zero modules.
5. **Router** — `httpx.NewRouter(cfg.Server, cfg.Security)`.
6. **Server** — `httpx.NewServer`, then `httpx.Mount(r, app.Modules, deps)`
   registers the operational endpoints and mounts every module's `Routes`
   under `/api/v1`. Each module declares its own auth middleware, so one can
   never be mounted unprotected by accident.
7. **Serve** until the first SIGINT/SIGTERM.

Every resource is registered on an `internal/lifecycle.Stack` exactly once, by
whoever created it, and released in the reverse order at the end — there are
no `defer x.Close()` calls in `main` for anything on that stack. A second
signal forces exit rather than leaving `SIGKILL` as the only way to abort a
stuck drain.
## 🧩 Modules & cross-cutting middleware

Extensibility splits cleanly in two, with no registry on either side.

> Earlier versions of this project had a plugin system — two competing
> registration frameworks, ~4,000 lines between them, neither of which anything
> imported. Both were deleted in Phase 3.6; see the
> [remediation plan](docs/architectures/07-roadmap/remediation-plan.md).

### Business capabilities are modules

A module is a plain constructor returning a `*module.Module`. No registry, no
`map[string]interface{}` config, no adapters:

```go
// internal/module/module.go
type Deps struct {
    DB     bun.IDB
    Logger zerolog.Logger
}

type Module struct {
    Name       string
    Routes     func(r chi.Router)  // mounted onto /api/v1
    Migrations fs.FS               // not wired up yet
    Close      func() error        // optional
}
```

Two modules ship today:

| Module | Routes | Notes |
|--------|--------|-------|
| `modules/identity` | `/api/v1/auth/*` | Login, refresh, logout, MFA. Owns RS256 token issuance and the auth middleware protecting its own routes |
| `modules/user` | `/api/v1/users/me` | The caller's own profile, keyed by their identity. Takes its auth middleware as a dependency rather than importing identity |

`modules/ofbiz` is an Apache OFBiz `UserLogin` compatibility layer with no
transport and no wiring — it is carried, not served.

**To add a module**, write `New(deps module.Deps, cfg Config) (*module.Module, error)`
in `modules/<name>/module.go` and build it in `cmd/api/container.go`. Two rules
are enforced mechanically by `tools/archtest` and `.golangci.yml`, so a
violation fails the build rather than review:

- A module must not import another module. Declare the interface you need in
  your own package and satisfy it in the composition root — the way
  `modules/user` takes a `func(http.Handler) http.Handler` in its `Config`
  instead of reaching into identity.
- `internal/**` must not import `modules/**`. Only `cmd/**` and the
  cross-cutting checks under `tools/` know about every module.

Validate your `Config` and return an error rather than degrading: `user.New`
refuses to construct without an auth middleware, because a module that mounts
its routes unprotected looks identical from the outside.

### CORS and rate limiting are not modules

They are cross-cutting HTTP concerns with typed config, constructed directly in
`internal/httpx.NewRouter` — an `if` each, rather than a registration
framework. Both are **off by default**:

```yaml
security:
  cors:
    enabled: false
    # Allowlist-only. An empty list grants no origin access, and "*" must be
    # listed explicitly. Combining "*" with allow_credentials is rejected at
    # config load.
    allowed_origins: []
    allow_credentials: false
    max_age: "5m"

  rate_limit:
    enabled: false
    rate: 100          # sustained requests per minute, per resolved client IP
    burst: 100         # instantaneous allowance
    ttl: "10m"         # idle bucket eviction
    max_keys: 100000   # new keys rejected once full; existing ones unaffected
```

Because these fields are typed, a YAML type error (`rate: "500"`) now fails
startup instead of being silently replaced by a default nobody chose — which
is what the plugin config map used to do.

The environment overrides are `APP_SECURITY_CORS_*` and
`APP_SECURITY_RATE_LIMIT_*`; `allowed_origins` takes a comma-separated list.
The `APP_RATELIMIT_*` / `APP_CORS_*` variables older copies of `.env.example`
advertised mapped to no config key at all and were silently ignored.

Request authentication for the API's own routes comes from the `identity`
module. Tokens are RS256, configured under `auth:` in `config/default.yaml`
and signed with the keypair from `make keys`. HS256 is not supported: the
signing algorithm is pinned, so a token presented with any other `alg` is
rejected before the key is even consulted.

Server-level security settings live under `server:`:

```yaml
server:
  request_timeout: "25s"    # must not exceed write_timeout; validated at boot
  trusted_proxies: []       # CIDRs whose X-Forwarded-For is honoured; empty = trust nothing
  enable_hsts: false        # only turn on where TLS is terminated in front of this process
```
## Agents Workspace Setup

```
.claude/
├── settings.json            # spawn depth, concurrency, deny rules (project scope)
└── agents/
    ├── team-lead.md         # orchestrator — run as MAIN session: claude --agent team-lead
    ├── domain-engineer.md   # domain/ + application/ layers        (Agent: research-only spawning)
    ├── persistence-engineer.md  # migrations, bun, repositories    (Agent: research-only spawning)
    ├── transport-engineer.md    # HTTP, middleware, routes         (Agent: research-only spawning)
    ├── platform-engineer.md     # config, dispatcher, cmd/api      (Agent: research-only spawning)
    ├── test-guardian.md         # goldens, fakes, conformance      (Agent: research-only spawning)
    ├── docs-scribe.md           # documentation                    (no Agent — cannot spawn)
    └── arch-reviewer.md         # read-only PR gate                (no Agent — cannot spawn)
docs/
└── plans/
    ├── agent-implementation-plan.md    # 19 tasks, guardrails §0, dependency graph
    ├── authn-spi-impact-analysis.md    # before/after per package, 401 resolution
    ├── notification-module-blueprint.md # module concept + blueprint
    └── task-status.md                  # team-lead's board (its only writable file)
```

### Setup

1. Copy `.claude/` and `docs/plans/` into the repo root. If you already have
   files in `.claude/agents/` or a `.claude/settings.json`, MERGE by hand —
   in particular, merge the `env` and `permissions.deny` keys rather than
   replacing your settings file.
2. `claude --version` — nesting defaults require v2.1.219+; the settings pin
   `CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=2` so behavior is explicit on any
   recent version.
3. Commit everything: agents and plans are project-scoped and meant for
   version control.
4. Start the effort from the repo root:

   ```
   claude --agent team-lead
   ```

   First dispatch is A1 (probe). The lead maintains docs/plans/task-status.md;
   read that file, not the transcript, to see where things stand.

### Settings notes

- `CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=2`: workers may spawn read-only
  research children; children cannot spawn further.
- `CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS=8`: enough for the plan's permitted
  parallelism (D1 + D2 alongside Phase B, plus research children) without
  letting a runaway loop fan out to the default 20.
- `permissions.deny`:
  - `Bash(make generate:*)` — the generator is broken (plan guardrail 2);
    denied mechanically, not just by instruction.
  - `Read(./keys/**)`, `Read(./.env)`, `Read(./.env.*)` — no agent has any
    reason to read signing keys or env secrets.
- Nothing here sets `"agent"` as a session default on purpose: normal coding
  sessions in the repo stay plain Claude Code; the team-lead session is
  started explicitly when you are running the plan.

## 🐳 Running locally

```bash
make keys          # generate a signing keypair into keys/ (never committed)
cp .env.example .env
                   # set APP_DB_PASSWORD — compose refuses to start without it
make compose-up    # Postgres, migrations, then the API on 127.0.0.1:8080
make compose-logs
make compose-down  # add compose-reset to drop the database volume
```

Migrations run as their own one-shot service, not from the API entrypoint: an
application that migrates on boot will, with two replicas, run two concurrent
migrations against one database.

The image is `distroless/static:nonroot` — no shell, no package manager,
running as uid 65532 — so the container healthcheck runs the binary's own
`healthcheck` subcommand rather than curl. `.dockerignore` keeps `keys/`,
`*.pem` and `.env` out of the build context entirely, so they are absent from
every layer rather than merely uncopied.

### Swagger

The UI is behind a build tag and its spec is generated, not committed:

```bash
make swagger-run   # generate the spec, then run with -tags swagger
```

A default build answers /swagger/* with a problem document explaining the flag.
Leaving the UI out halves the binary — it links swaggo/swag, the embedded
Swagger UI distribution and four go-openapi packages, none of which a
production server needs.

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run a single module's tests
go test ./modules/identity/...

# Check the dependency rules (docs/architectures/01-modularity/dependency-rules.md)
make arch

# Enforce the per-package coverage floors and the no-regression ratchet
make coverage-check

# Raise the baseline after genuinely improving coverage
make coverage-update
```

Tests that need a database stand up a throwaway Postgres container, or use
`TEST_DATABASE_URL` if it is set, and skip cleanly when neither is available
(`internal/testsupport.ScratchPostgres`). They never guess at credentials for
whatever happens to be listening on `localhost:5432`.

Shared fixtures live in `internal/testsupport`: `MigratedDB` and `TruncateAll`
for a clean database, `Config` for a valid `*config.Config` backed by a
freshly generated keypair, `MintToken` for tokens login cannot produce
(expired, wrong issuer, wrong class), and JSON request/response helpers.

### Two things that will lie to you

> **`make arch` passes `-count=1`, and so must you.** The architecture tests
> read the whole repository through `go/packages`, but Go's test cache keys
> only on the `tools/archtest` package's own files — so a violation introduced
> anywhere else returns a cached `ok` against a tree that now fails. Plain
> `go test ./tools/archtest/...` will lie to you.

> **A local `go test ./...` does not run the integration suite.** Every
> database-backed test skips cleanly without a Postgres, and a skip prints
> `ok`. `make coverage-check` names the packages it could not check for that
> reason rather than passing them at 0%. CI has a service container, so that
> is where they actually run.

### Coverage policy

Floors are per-package, in `tools/coverage/policy.json`, each with the reason
it exists — 90% for `domain`, 80% for `application`, 85% for `internal/config`
and the HTTP middleware. There is no global percentage target, because a
global number is met most cheaply by testing trivial getters.

The same file carries a baseline that coverage may not drop below.
`-update` refuses to lower a recorded value without `-allow-decrease`, so
re-baselining is a deliberate, reviewable act.

### CI

`.github/workflows/ci.yml` runs five jobs on every push and pull request:

| Job | What it gates |
|---|---|
| **build** | gofmt, build, vet, `go test -race` against a real Postgres, the architecture rules, the coverage floors and ratchet |
| **migrations** | up → reset (every `Down`) → up, then the model/schema drift test |
| **lint** | `golangci-lint` with the standard set plus the `depguard` layering rules |
| **security** | `govulncheck`, and `gitleaks` over the full history |
| **size** | reports the `cmd/api` binary size to the job summary |

`-race` is the one that matters most: it does not run on Windows without a
modern mingw-w64, and a data race once shipped in the very commit whose
purpose was concurrency correctness. CI is the only place that check exists.

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
