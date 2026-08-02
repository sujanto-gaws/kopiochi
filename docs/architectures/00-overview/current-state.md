# Current State Assessment

**Date:** 2026-08-02
**Method:** Full repository review — `go build ./...`, `go vet ./...`, import-graph
sweep, `git ls-files`, manual reading of every file on the startup path.
**Last verified:** 2026-08-02, after Phases 0 and 1.

> **This is a snapshot, and it has moved on.** Phases 0 and 1 of the
> [remediation plan](../07-roadmap/remediation-plan.md) have landed. The findings
> table below now carries a **Status** column; sections describing resolved
> problems are marked. Separately, several structural findings in an earlier
> revision — the three-layout table, the dead-code inventory, and the
> dependency-direction section — cited paths that appear in no commit of this
> repository. Those have now been withdrawn and replaced with what the tree
> actually contains; see the notes in place below.

---

## Executive summary

**As of the review:** the project **compiles cleanly**, but a large fraction of
the hand-written Go is unreachable from `cmd/` — it compiles, and nothing calls
it. Two competing extension frameworks coexist, one of them with an entire
parallel identity implementation attached to it.

> **Withdrawn.** This summary previously opened by calling the project
> "functionally inert", on the grounds that the DI container returned an empty
> registrar list and the server therefore served only `/api/health` and
> `/swagger/*`. That premise does not hold: the earliest `cmd/api/container/container.go`
> in this repository's history (`794d783`) already returned two registrars, as did
> the version current when this document was written (`0fbab20`). The claim could
> not be substantiated and has been withdrawn, together with the "approximately
> 40% of hand-written Go is dead code" figure, which was derived from the
> inventory withdrawn below.

**As of Phase 1:** `cmd/api/container.go` (`BuildApp`) wires the `identity` and
`user` modules and refuses to return a zero-module application;
`internal/httpx.Mount` serves them under a real `/api/v1`; `/healthz` and
`/readyz` replace `/api/health`. `POST /api/v1/auth/login` returns a token
against a migrated database, proven end to end by `cmd/api/login_e2e_test.go`.

The dead-code and duplicate-framework problem is unchanged: two extension
frameworks coexist (`internal/extension/` vs `internal/plugin{,s}/`), and a
substantial body of code is still reachable only by the compiler.

---

## What actually runs

The live request path, after Phase 1:

```
cmd/api/main.go
  ├─ config.Load()                    internal/config            main.go:45
  ├─ plugin.NewRegistry()             internal/plugin            main.go:56
  │    └─ RegisterBuiltinPlugins()     internal/plugins  → cors, ratelimit, jwt-auth, fido2-auth
  ├─ db.NewDB()                       internal/db                main.go:68
  ├─ BuildApp()                       cmd/api/container.go       main.go:80
  │    ├─ identity.New()               modules/identity
  │    └─ newUserModule()              internal/{application,infrastructure}/user
  ├─ server.NewRouter()               internal/infrastructure/http/server
  └─ httpx.Mount()                    internal/httpx             main.go:104
       ├─ GET /healthz
       ├─ GET /readyz          (pings the pool, 503 when unreachable)
       ├─ GET /health          (deprecated alias for /healthz)
       ├─ GET /swagger/*
       └─ /api/v1  → identity routes (/auth/*), user routes (/users/*)
```

At review time this path ended in `routes.Setup()`
(`internal/infrastructure/http/routes`, since deleted), whose `/api/v1` block
mounted onto a shadowed router — so every module route would have served at the
root. Both the container and the router were replaced in `ef76759` and `4fdc609`.

A large amount of the repository is still reachable only by the compiler; see the
dead-code inventory below.

---

## Structural findings

### Two coexisting layouts for business code

> **Withdrawn.** This section previously presented three coexisting layout
> "generations", the third being `extensions/identity/` and the first two
> containing aquaculture files, some of them zero bytes. Neither `extensions/` nor
> `internal/modules/` appears in any commit of this repository, and no aquaculture
> file has ever existed under any path. The table could not be substantiated and
> has been withdrawn.

| Location | State |
|---|---|
| `modules/identity/` | Live, wired through `BuildApp`, matches the target module shape |
| `internal/domain/`, `internal/application/`, `internal/infrastructure/` | Live but layer-first — the profile-user stack `cmd/api/container.go` wires as the `user` module, plus `internal/domain/ofbizuser` |

One capability has been moved to `modules/`; the other has not. See
[module layout](../01-modularity/module-layout.md).

### Two competing extension frameworks

| Framework | Location | Size | Wired into `main.go`? |
|---|---|---|---|
| `Manager` (Yii-style) | `internal/extension/` | ~600 LOC | **No** — only `examples/` and dead identity code |
| `Registry` (factory) | `internal/plugin/` + `internal/plugins/` | ~500 LOC | **Yes** |

Both implement lifecycle, registration, and service lookup. Only one is used.

### Dead code inventory

> **Withdrawn and replaced.** The previous inventory listed
> `extensions/identity/**` (~2,000 LOC), `internal/modules/aquaculture/**`, and
> orphaned `internal/domain|application/aqua*` files. None of those paths appear
> in any commit of this repository. The figures could not be substantiated and
> have been withdrawn. The list below was re-derived from the current tree by
> walking the import graph from `cmd/`.

```
internal/extension/**              1,581 LOC   Framework A; imported only by examples/
  └─ internal/extension/identity/  1,076 LOC   parallel identity impl, no importers,
                                               no transport layer
examples/extension-demo/**           214 LOC   the only consumer of Framework A
internal/plugins/auth/fido2.go       383 LOC   linked, but cannot initialise (see below)
```

`internal/plugin/` (442 LOC) and the rest of `internal/plugins/` (1,003 LOC
total) *are* reachable — `cmd/api/main.go:56-64` initialises the registry — but
they are slated for deletion once CORS and rate limiting become direct
construction (Phase 3.5).

### Dependency direction

> **Withdrawn.** This section previously reported an inverted dependency:
> `extensions/identity/**` importing `internal/utils` across 11 named files. No
> `extensions/` directory and no `internal/utils` package appears in any commit of
> this repository. The finding could not be substantiated and has been withdrawn.
> The forward-looking rules it motivated are unchanged and still apply — see
> [dependency rules](../01-modularity/dependency-rules.md).

What is true is that **nothing enforces** dependency direction: there is no
`.golangci.yml`, no `depguard` configuration, no architecture test, and no CI to
run one.

---

## Weight findings

| Metric | Value |
|---|---|
| Hand-written Go | ~8,000 LOC (1,144 of it is `cmd/generator`) |
| Generated | `docs/docs.go` 338 lines + swagger.json/yaml |
| Module graph | **154 modules** |
| `cmd/api` binary | **38 MB** |
| `.git` directory | **58 MB** |

**Binaries were committed to version control.** At review time `git ls-files bin/`
returned:

```
bin/kopiochi            38.7 MB   (two distinct blobs in history)
bin/kopiochi-migrate    22.9 MB
kopiochi.exe            20.1 MB   (historical blob at repo root)
```

That is ~120 MB of binary blobs permanently in history, for an 8k-LOC project.
`.gitignore` had no `bin/` entry — and was itself wrapped in literal markdown
` ``` ` fences, which made its first and last lines meaningless patterns.

*Working tree fixed: `4c72a83` repaired `.gitignore` (`bin/`, `*.exe`, `keys/`,
`*.pem`, `coverage.*`, `*.zip`) and `1c5ac2c` untracked the artifacts.
`git ls-files bin/ keys/ '*.pem' '*.exe' '*.zip'` now returns nothing.*
**History is untouched** — the ~120 MB and the 58 MB `.git` are still there until
the Phase 5.1 rewrite.

**Dependencies carried for dead code:**

- `go-webauthn/webauthn` + `go-tpm` + `fxamacker/cbor` — only the unreachable FIDO2 plugin
- `boombuler/barcode` — no importer anywhere in the tree. (`pquerna/otp` **is**
  used, by the live `modules/identity/infrastructure/mfa/totp.go`.)
- `swaggo/swag v1.8.1` (2022 release) + `http-swagger` + 4× `go-openapi/*`
- Two YAML libraries: `gopkg.in/yaml.v3` **and** `go.yaml.in/yaml/v3`

---

## Correctness findings on the live path

These are the defects in code that actually executes. Full detail lives in the
topic documents; this is the index. **Status** is as of the end of Phase 1;
locations are the review-time locations unless a fix moved them.

| # | Severity | Finding | Location | Status | Topic doc |
|---|----------|---------|----------|--------|-----------|
| 1 | — | ~~DI container returns empty registrars~~ | `cmd/api/container/container.go:29` | **Withdrawn** — unsubstantiated. `794d783` and `0fbab20` both show `registrars: []handlers.RouteRegistrar{authHandler, userHandler}`. The file was nonetheless replaced by `BuildApp` in `ef76759` for the design reason in the topic doc | [DI](../02-composition/dependency-injection.md) |
| 2 | Critical | `/api/v1` prefix is a no-op (shadowed router) | `routes/routes.go:25` | **Resolved** `4fdc609` — `internal/httpx.Mount`; `TestRouteTable` guards it | [Routing](../02-composition/routing-and-versioning.md) |
| 3 | Critical | Rate limiter holds mutex across `next.ServeHTTP` | `plugins/middleware/ratelimit.go:81` | Open — Phase 2.1 | [Rate limiting](../04-security/rate-limiting.md) |
| 4 | High | Rate limiter map never evicts | `ratelimit.go:50` | Open — Phase 2.1 | [Rate limiting](../04-security/rate-limiting.md) |
| 5 | High | `X-Forwarded-For` trusted verbatim | `ratelimit.go:76` | Open — Phase 2.2 | [Rate limiting](../04-security/rate-limiting.md) |
| 6 | High | JWT signing algorithm not pinned | `plugins/auth/jwt.go:110` | Open — Phase 2.4 | [Tokens](../04-security/token-architecture.md) |
| 7 | High | DB password + JWT secret committed | `config/default.yaml:14,51` | **Resolved in the working tree** `b74b358` (`:15`/`:50` are now pointers to `APP_DB_PASSWORD`/`APP_JWT_SECRET`). Still in history; rotation still outstanding | [Secrets](../03-configuration/secret-management.md) |
| 8 | High | RSA private key not git-ignored | `keys/private.pem` | **Resolved** `8652534` + `4c72a83`; `make keys` regenerates it | [Secrets](../03-configuration/secret-management.md) |
| 9 | High | `.gitignore` malformed (markdown fences) | `.gitignore` | **Resolved** `4c72a83` | [Hygiene](../06-quality/repository-hygiene.md) |
| 10 | High | CORS reflects arbitrary `Origin`, no `Vary` | `plugins/middleware/cors.go:34,103` | Open — Phase 2.3 | [Middleware](../04-security/middleware-hardening.md) |
| 11 | High | Env-var config override silently inert | `internal/config/config.go:77` | **Partially resolved** `b74b358` — explicit `BindEnv` at `config.go:88-93` fixes `APP_DB_PASSWORD` and `APP_JWT_SECRET`. `db.user` and `db.name` still have neither a default nor a `BindEnv`, so `APP_DB_USER`/`APP_DB_NAME` remain inert | [Config](../03-configuration/configuration-model.md) |
| 12 | Medium | DSN does not escape credentials | `internal/db/database.go:65-66` | Open — Phase 3.8 | [Persistence](../05-data/persistence-and-pooling.md) |
| 13 | Medium | `sql.DB` pool mis-sized over pgxpool | `database.go:42` | Open — Phase 3.8 | [Persistence](../05-data/persistence-and-pooling.md) |
| 14 | Medium | Double shutdown of registry and pool | `main.go:63,76` | Open — Phase 3.9 | [Lifecycle](../02-composition/lifecycle-and-shutdown.md) |
| 15 | Medium | `GenerateToken` drops `email`, overwrites `name` | `plugins/auth/jwt.go:152` | Open — Phase 2.6 | [Tokens](../04-security/token-architecture.md) |
| 16 | Medium | Registry overwrites plugin without `Close()` | `internal/plugin/registry.go:116` | Open — Phase 3.5/3.6 | [Extensions](../01-modularity/extension-framework.md) |
| 17 | Medium | FIDO2 plugin cannot be initialised from YAML | `plugins/auth/fido2.go:92` | Open — Phase 3.6 | [Extensions](../01-modularity/extension-framework.md) |
| 18 | Medium | Migrations match nothing in the codebase | `migrations/` | **Overstated, and partially resolved** `fbddccb` — `00003`–`00005` match the live identity models. `00001_create_users.sql` always matched `UserDBModel`; only `00002_create_products.sql` is an orphan | [Migrations](../05-data/migration-strategy.md) |
| 19 | Medium | `migrate-*` targets pass empty `--config` | `Makefile` | **Resolved** `657b2dc` — `Makefile:14` `CONFIG?=config/default.yaml` | [Migrations](../05-data/migration-strategy.md) |
| 20 | Medium | `db-migrate` is a TODO next to a working target | `Makefile` | **Resolved** `657b2dc` — target deleted | [Migrations](../05-data/migration-strategy.md) |
| 21 | Medium | Token `Validate` skips `iss`/`aud`/scope | `modules/identity/infrastructure/token/jwt.go:75` | Open — Phase 2.4/2.5. Defect unchanged; the file moved in `5f6edfe` | [Tokens](../04-security/token-architecture.md) |
| 22 | Medium | CRLF everywhere, no `.gitattributes` | repo-wide | **Resolved** `b294de2` + `3dbd1b4`; `gofmt -l .` returns nothing | [Hygiene](../06-quality/repository-hygiene.md) |
| 23 | Low | `go vet` warnings | `examples/extension-demo/main.go:131,183` | **Resolved** `6459348`; `go vet ./...` is clean | [Testing](../06-quality/testing-strategy.md) |
| 24 | Medium | **Zero test files** | repo-wide | **Resolved** `d92480c` + `720e580` — 7 test files. Still no CI to run them | [Testing](../06-quality/testing-strategy.md) |

---

## Root causes

The findings above (23, after finding 1 was withdrawn) are symptoms. There are
four underlying causes:

1. **No enforced dependency rules.** No linter, no architecture test, no CI.
   Nothing prevents a second layout being added beside the first, or a domain
   package acquiring an infrastructure import.
   → [`01-modularity/dependency-rules.md`](../01-modularity/dependency-rules.md)

2. **Composition is optional.** A composition root whose completeness rests on a
   comment ("append it here") rather than on a type means "wired up" is not a
   compile-time property — a handler left out of the list is simply not served,
   and nothing says so.
   → [`02-composition/dependency-injection.md`](../02-composition/dependency-injection.md)
   *Addressed: `BuildApp` returns `(*App, error)` and every module constructor
   returns an error, so misconfiguration fails at boot (`ef76759`). The
   zero-module guard itself is currently unreachable — two modules are appended
   unconditionally — so it documents the rule rather than enforcing it.*

3. **Config carries live objects.** The plugin config contract is
   `map[string]interface{}`, which invites passing Go interfaces that YAML can
   never supply (the FIDO2 bug) and defers all validation to runtime.
   → [`01-modularity/extension-framework.md`](../01-modularity/extension-framework.md)

4. **No feedback loop.** Zero tests, no CI gate, and a `make check` that reports
   the entire repo dirty because of line endings. Nothing catches regressions.
   → [`06-quality/testing-strategy.md`](../06-quality/testing-strategy.md)
   *Partially addressed: `b294de2` made `make check` meaningful (`gofmt -l .` is
   clean) and `d92480c` added the first seven test files. **There is still no CI
   gate**, so nothing runs them automatically — the root cause is not closed.*

---

## Related documents

- [Target architecture](target-architecture.md) — where this should land
- [Design principles](design-principles.md) — the rules that keep it there
- [Remediation plan](../07-roadmap/remediation-plan.md) — sequenced execution
