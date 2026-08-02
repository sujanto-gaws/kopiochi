# Current State Assessment

**Date:** 2026-08-02
**Method:** Full repository review — `go build ./...`, `go vet ./...`, import-graph
sweep, `git ls-files`, manual reading of every file on the startup path.

---

## Executive summary

The project **compiles cleanly** but is **functionally inert**. The dependency
injection container returns an empty registrar list, so the running server exposes
only `/api/health` and `/swagger/*`. Every business handler that exists in the
codebase is unreachable.

Three generations of directory layout and two competing extension frameworks
coexist. Approximately **40% of hand-written Go is dead code** — it compiles, but
nothing in `cmd/` imports it.

---

## What actually runs

The complete live request path today:

```
cmd/api/main.go
  ├─ config.Load()                    internal/config
  ├─ plugin.NewRegistry()             internal/plugin
  │    └─ RegisterBuiltinPlugins()     internal/plugins  → cors, ratelimit, jwt-auth, fido2-auth
  ├─ db.NewDB()                       internal/db
  ├─ container.New()                  cmd/api/container  ← RETURNS EMPTY
  ├─ server.NewRouter()               internal/infrastructure/http/server
  └─ routes.Setup()                   internal/infrastructure/http/routes
       ├─ GET /api/health
       ├─ GET /swagger/*
       └─ /api/v1  → (no registrars)
```

Everything else in the repository is reachable only by the compiler.

---

## Structural findings

### Three coexisting layouts

| Generation | Location | State |
|---|---|---|
| gen-1 | `internal/domain/`, `internal/application/`, `internal/infrastructure/persistence/` | Flat aquaculture files; two are **0 bytes** |
| gen-2 | `internal/modules/aquaculture/` | **6 of 7 files empty** — skeleton only |
| gen-3 | `extensions/identity/` | Complete (~2,000 LOC), **zero imports from `cmd/`** |

The intent is clearly a migration from gen-1 → gen-2/gen-3, but no generation was
ever removed and the newest one was never wired up.

### Two competing extension frameworks

| Framework | Location | Size | Wired into `main.go`? |
|---|---|---|---|
| `Manager` (Yii-style) | `internal/extension/` | ~600 LOC | **No** — only `examples/` and dead identity code |
| `Registry` (factory) | `internal/plugin/` + `internal/plugins/` | ~500 LOC | **Yes** |

Both implement lifecycle, registration, and service lookup. Only one is used.

### Dead code inventory

Verified by grepping the import graph from `cmd/`:

```
extensions/identity/**            ~2,000 LOC   entire auth / MFA / roles stack
internal/modules/aquaculture/**     ~210 LOC   mostly empty stubs
internal/domain|application/aqua*             orphaned, 2 files are 0 bytes
internal/extension/**               ~600 LOC   examples only
internal/plugins/auth/fido2.go       383 LOC   unreachable by construction
```

### Dependency direction is inverted

`extensions/identity/**` imports `internal/utils` in 11 files. An `extensions/`
tree that depends on `internal/` cannot be extracted, published, or loaded
independently — which defeats the purpose of having an `extensions/` tree.

---

## Weight findings

| Metric | Value |
|---|---|
| Hand-written Go | ~8,000 LOC (1,144 of it is `cmd/generator`) |
| Generated | `docs/api/docs.go` 2,100 LOC + swagger.json/yaml |
| Module graph | **154 modules** |
| `cmd/api` binary | **38 MB** |
| `.git` directory | **58 MB** |

**Binaries are committed to version control.** `git ls-files bin/` confirms:

```
bin/kopiochi            38.7 MB   (two distinct blobs in history)
bin/kopiochi-migrate    22.9 MB
kopiochi.exe            20.1 MB   (historical blob at repo root)
```

That is ~120 MB of binary blobs permanently in history, for an 8k-LOC project.
`.gitignore` has no `bin/` entry — and is itself wrapped in literal markdown
` ``` ` fences, which makes its first and last lines meaningless patterns.

**Dependencies carried for dead code:**

- `go-webauthn/webauthn` + `go-tpm` + `fxamacker/cbor` — only the unreachable FIDO2 plugin
- `pquerna/otp` + `boombuler/barcode` — only the unreachable identity MFA
- `swaggo/swag v1.8.1` (2022 release) + `http-swagger` + 4× `go-openapi/*`
- Two YAML libraries: `gopkg.in/yaml.v3` **and** `go.yaml.in/yaml/v3`

---

## Correctness findings on the live path

These are the defects in code that actually executes. Full detail lives in the
topic documents; this is the index.

| # | Severity | Finding | Location | Topic doc |
|---|----------|---------|----------|-----------|
| 1 | Critical | DI container returns empty registrars | `cmd/api/container/container.go:29` | [DI](../02-composition/dependency-injection.md) |
| 2 | Critical | `/api/v1` prefix is a no-op (shadowed router) | `routes/routes.go:25` | [Routing](../02-composition/routing-and-versioning.md) |
| 3 | Critical | Rate limiter holds mutex across `next.ServeHTTP` | `plugins/middleware/ratelimit.go:80` | [Rate limiting](../04-security/rate-limiting.md) |
| 4 | High | Rate limiter map never evicts | `ratelimit.go:50` | [Rate limiting](../04-security/rate-limiting.md) |
| 5 | High | `X-Forwarded-For` trusted verbatim | `ratelimit.go:76` | [Rate limiting](../04-security/rate-limiting.md) |
| 6 | High | JWT signing algorithm not pinned | `plugins/auth/jwt.go:109` | [Tokens](../04-security/token-architecture.md) |
| 7 | High | DB password + JWT secret committed | `config/default.yaml:14,51` | [Secrets](../03-configuration/secret-management.md) |
| 8 | High | RSA private key not git-ignored | `keys/private.pem` | [Secrets](../03-configuration/secret-management.md) |
| 9 | High | `.gitignore` malformed (markdown fences) | `.gitignore` | [Hygiene](../06-quality/repository-hygiene.md) |
| 10 | High | CORS reflects arbitrary `Origin`, no `Vary` | `plugins/middleware/cors.go:103` | [Middleware](../04-security/middleware-hardening.md) |
| 11 | High | Env-var config override silently inert | `internal/config/config.go:76` | [Config](../03-configuration/configuration-model.md) |
| 12 | Medium | DSN does not escape credentials | `internal/db/database.go:66` | [Persistence](../05-data/persistence-and-pooling.md) |
| 13 | Medium | `sql.DB` pool mis-sized over pgxpool | `database.go:42` | [Persistence](../05-data/persistence-and-pooling.md) |
| 14 | Medium | Double shutdown of registry and pool | `main.go:66,79` | [Lifecycle](../02-composition/lifecycle-and-shutdown.md) |
| 15 | Medium | `GenerateToken` drops `email`, overwrites `name` | `plugins/auth/jwt.go:152` | [Tokens](../04-security/token-architecture.md) |
| 16 | Medium | Registry overwrites plugin without `Close()` | `internal/plugin/registry.go:99` | [Extensions](../01-modularity/extension-framework.md) |
| 17 | Medium | FIDO2 plugin cannot be initialised from YAML | `plugins/auth/fido2.go:92` | [Extensions](../01-modularity/extension-framework.md) |
| 18 | Medium | Migrations match nothing in the codebase | `migrations/` | [Migrations](../05-data/migration-strategy.md) |
| 19 | Medium | `migrate-*` targets pass empty `--config` | `Makefile` | [Migrations](../05-data/migration-strategy.md) |
| 20 | Medium | `db-migrate` is a TODO next to a working target | `Makefile` | [Migrations](../05-data/migration-strategy.md) |
| 21 | Medium | Token `Validate` skips `iss`/`aud`/scope | `identity/.../token/jwt.go:73` | [Tokens](../04-security/token-architecture.md) |
| 22 | Medium | CRLF everywhere, no `.gitattributes` | repo-wide | [Hygiene](../06-quality/repository-hygiene.md) |
| 23 | Low | `go vet` warnings | `examples/extension-demo/main.go:131,183` | [Testing](../06-quality/testing-strategy.md) |
| 24 | Medium | **Zero test files** | repo-wide | [Testing](../06-quality/testing-strategy.md) |

---

## Root causes

The 24 findings above are symptoms. There are four underlying causes:

1. **No enforced dependency rules.** Nothing prevents a new layout from being
   added beside an old one, or `extensions/` from importing `internal/`.
   → [`01-modularity/dependency-rules.md`](../01-modularity/dependency-rules.md)

2. **Composition is optional.** Because `container.New()` compiles perfectly well
   while returning nothing, "wired up" is not a compile-time property.
   → [`02-composition/dependency-injection.md`](../02-composition/dependency-injection.md)

3. **Config carries live objects.** The plugin config contract is
   `map[string]interface{}`, which invites passing Go interfaces that YAML can
   never supply (the FIDO2 bug) and defers all validation to runtime.
   → [`01-modularity/extension-framework.md`](../01-modularity/extension-framework.md)

4. **No feedback loop.** Zero tests, no CI gate, and a `make check` that reports
   the entire repo dirty because of line endings. Nothing catches regressions.
   → [`06-quality/testing-strategy.md`](../06-quality/testing-strategy.md)

---

## Related documents

- [Target architecture](target-architecture.md) — where this should land
- [Design principles](design-principles.md) — the rules that keep it there
- [Remediation plan](../07-roadmap/remediation-plan.md) — sequenced execution
