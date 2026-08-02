# Current State Assessment

**Date:** 2026-08-02
**Method:** Full repository review — `go build ./...`, `go vet ./...`, import-graph
sweep, `git ls-files`, manual reading of every file on the startup path.
**Last verified:** 2026-08-02, after Phases 0, 1, and 2.

> **This is a snapshot, and it has moved on.** Phases 0, 1, and 2 of the
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

**As of Phase 2:** every security finding on the live request path is closed.
The rate limiter is a token bucket that releases its lock before the downstream
handler, evicts idle keys, and caps the table (`dcc6e5d`, `d130519`). Client IP
is resolved once, from trusted-proxy CIDRs only, and consumed from the request
context by the limiter and the logger (`333968c`). CORS is allowlist-only with
no reflection outside the list and an always-present `Vary: Origin` (`87381d2`).
Security response headers are set on every response (`0968aae`). Tokens are
pinned to RS256 with `iss`/`aud`/`exp` validation and an explicit token class
required at every call site (`e0da81e`, `946c1c8`), and the HS256 plugin is
**deleted** (`0cf07d9`). Config validates at load, rejects placeholder secrets,
and redacts `DB.Password` through `secret.String` (`acc057d`).

The dead-code and duplicate-framework problem is unchanged: two extension
frameworks coexist (`internal/extension/` vs `internal/plugin{,s}/`), and a
substantial body of code is still reachable only by the compiler. Phase 2 made
that tree slightly smaller — `internal/plugins/auth/jwt.go` (177 LOC) is gone —
but the frameworks themselves are Phase 3.

One gap Phase 2 could not close: **`go test -race` does not run in this
environment** (mingw-w64 8.1.0 against Go 1.25; every package exits
`0xc0000139`). A real data race shipped inside Phase 2 and was found by
inspection, not tooling — see
[testing strategy](../06-quality/testing-strategy.md#race-detection-is-outstanding).

---

## What actually runs

The live request path, after Phase 2:

```
cmd/api/main.go
  ├─ config.Load()                    internal/config            main.go:45
  │    └─ Config.Validate()            fails closed on a bad or placeholder config
  ├─ plugin.NewRegistry()             internal/plugin            main.go:56
  │    └─ RegisterBuiltinPlugins()     internal/plugins  → fido2-auth, ratelimit, cors
  ├─ db.NewDB()                       internal/db                main.go:68
  ├─ BuildApp()                       cmd/api/container.go       main.go:80
  │    ├─ identity.New()               modules/identity
  │    └─ newUserModule()              internal/{application,infrastructure}/user
  ├─ server.NewRouter()               internal/infrastructure/http/server
  │    ├─ chi Recoverer, RequestID
  │    ├─ httpx.SecurityHeaders        internal/httpx        (2.8)
  │    ├─ middleware.RealIP            internal/middleware   (2.2, trusted proxies only)
  │    ├─ chi Timeout(request_timeout = 25s)
  │    └─ ZerologRequestLogger         logs the resolved client IP
  ├─ plugin middleware chain          main.go:89  → cors, then ratelimit
  └─ httpx.Mount()                    internal/httpx             main.go:104
       ├─ GET /healthz
       ├─ GET /readyz          (pings the pool, 503 when unreachable)
       ├─ GET /health          (deprecated alias for /healthz)
       ├─ GET /swagger/*
       └─ /api/v1  → identity routes (/auth/*), user routes (/users/*)
```

`jwt-auth` was in this list until `0cf07d9` deleted it. No auth plugin is on the
request path: authentication is the identity module's RS256 token service, wired
as a constructor dependency that cannot be nil.

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

`internal/plugin/` (442 LOC) and the rest of `internal/plugins/` *are* reachable
— `cmd/api/main.go:56-64` initialises the registry — but they are slated for
deletion once CORS and rate limiting become direct construction (Phase 3.5).

*The 1,003 LOC figure recorded here for `internal/plugins/` predates Phase 2 and
no longer holds: `0cf07d9` deleted `internal/plugins/auth/jwt.go` (177 LOC), and
`dcc6e5d`/`87381d2` roughly doubled the CORS and rate-limiter files while
hardening them. The figure has not been re-measured; treat it as stale rather
than as current.*

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
topic documents; this is the index. **Status** is as of the end of Phase 2;
locations are the review-time locations unless a fix moved them.

| # | Severity | Finding | Location | Status | Topic doc |
|---|----------|---------|----------|--------|-----------|
| 1 | — | ~~DI container returns empty registrars~~ | `cmd/api/container/container.go:29` | **Withdrawn** — unsubstantiated. `794d783` and `0fbab20` both show `registrars: []handlers.RouteRegistrar{authHandler, userHandler}`. The file was nonetheless replaced by `BuildApp` in `ef76759` for the design reason in the topic doc | [DI](../02-composition/dependency-injection.md) |
| 2 | Critical | `/api/v1` prefix is a no-op (shadowed router) | `routes/routes.go:25` | **Resolved** `4fdc609` — `internal/httpx.Mount`; `TestRouteTable` guards it | [Routing](../02-composition/routing-and-versioning.md) |
| 3 | Critical | Rate limiter holds mutex across `next.ServeHTTP` | `plugins/middleware/ratelimit.go:81` | **Resolved** `dcc6e5d` — token bucket; the lock is released inside `allow()`. `TestRateLimitAllowsConcurrentRequests` now runs unskipped and passes | [Rate limiting](../04-security/rate-limiting.md) |
| 4 | High | Rate limiter map never evicts | `ratelimit.go:50` | **Resolved** `dcc6e5d` — TTL sweep (`evictExpired`) plus a `max_keys` cap that rejects new keys rather than growing | [Rate limiting](../04-security/rate-limiting.md) |
| 5 | High | `X-Forwarded-For` trusted verbatim | `ratelimit.go:76` | **Resolved** `333968c` — `internal/middleware.RealIP` honours forwarded headers only from trusted-proxy CIDRs (empty = trust nothing); the limiter reads the resolved IP from the context and never a header | [Rate limiting](../04-security/rate-limiting.md) |
| 6 | High | JWT signing algorithm not pinned | `plugins/auth/jwt.go:110` | **Resolved** `e0da81e` + `0cf07d9` — the RS256 service pins the alg with `jwt.WithValidMethods`; the HS256 plugin that held this defect is deleted | [Tokens](../04-security/token-architecture.md) |
| 7 | High | DB password + JWT secret committed | `config/default.yaml:14,51` | **Resolved in the working tree** `b74b358`; `0cf07d9` removed the JWT secret's config key entirely, and `acc057d` now rejects a placeholder `db.password` at startup. Still in history; **rotation still outstanding** | [Secrets](../03-configuration/secret-management.md) |
| 8 | High | RSA private key not git-ignored | `keys/private.pem` | **Resolved** `8652534` + `4c72a83`; `make keys` regenerates it | [Secrets](../03-configuration/secret-management.md) |
| 9 | High | `.gitignore` malformed (markdown fences) | `.gitignore` | **Resolved** `4c72a83` | [Hygiene](../06-quality/repository-hygiene.md) |
| 10 | High | CORS reflects arbitrary `Origin`, no `Vary` | `plugins/middleware/cors.go:34,103` | **Resolved** `87381d2` — allowlist-only, deny by default, `Origin` echoed only after an exact allowlist match, `Vary: Origin` always set, no 403 for non-browser clients. `*`+credentials rejected in `Config.Validate` | [Middleware](../04-security/middleware-hardening.md) |
| 11 | High | Env-var config override silently inert | `internal/config/config.go:77` | **Resolved** `b74b358` + `acc057d` — explicit `BindEnv` for `db.password`, `db.user`, and `db.name`, so all the `APP_DB_*` variables reach `Unmarshal`. The `APP_JWT_SECRET` binding went with the plugin in `0cf07d9`. *ADR-008's "a registered default for every key" is still not done, so a newly added key needs a `BindEnv` of its own* | [Config](../03-configuration/configuration-model.md) |
| 12 | Medium | DSN does not escape credentials | `internal/db/database.go:65-66` | Open — Phase 3.8 | [Persistence](../05-data/persistence-and-pooling.md) |
| 13 | Medium | `sql.DB` pool mis-sized over pgxpool | `database.go:42` | Open — Phase 3.8 | [Persistence](../05-data/persistence-and-pooling.md) |
| 14 | Medium | Double shutdown of registry and pool | `main.go:63,76` | Open — Phase 3.9 | [Lifecycle](../02-composition/lifecycle-and-shutdown.md) |
| 15 | Medium | `GenerateToken` drops `email`, overwrites `name` | `plugins/auth/jwt.go:152` | **Resolved by deletion** `0cf07d9` — the file is gone. The RS256 service carries `name` and `email` as their own claims and never touches `iss` | [Tokens](../04-security/token-architecture.md) |
| 16 | Medium | Registry overwrites plugin without `Close()` | `internal/plugin/registry.go:116` | Open — Phase 3.5/3.6 | [Extensions](../01-modularity/extension-framework.md) |
| 17 | Medium | FIDO2 plugin cannot be initialised from YAML | `plugins/auth/fido2.go:92` | Open — Phase 3.6 | [Extensions](../01-modularity/extension-framework.md) |
| 18 | Medium | Migrations match nothing in the codebase | `migrations/` | **Overstated, and partially resolved** `fbddccb` — `00003`–`00005` match the live identity models. `00001_create_users.sql` always matched `UserDBModel`; only `00002_create_products.sql` is an orphan | [Migrations](../05-data/migration-strategy.md) |
| 19 | Medium | `migrate-*` targets pass empty `--config` | `Makefile` | **Resolved** `657b2dc` — `Makefile:14` `CONFIG?=config/default.yaml` | [Migrations](../05-data/migration-strategy.md) |
| 20 | Medium | `db-migrate` is a TODO next to a working target | `Makefile` | **Resolved** `657b2dc` — target deleted | [Migrations](../05-data/migration-strategy.md) |
| 21 | Medium | Token `Validate` skips `iss`/`aud`/scope | `modules/identity/infrastructure/token/jwt.go:75` | **Resolved** `e0da81e` + `946c1c8` — `iss`, `aud`, and a required `exp` are validated with `auth.token_leeway`, and `Validate(token, want Class)` requires an explicit token class, so an MFA token cannot serve as an access token | [Tokens](../04-security/token-architecture.md) |
| 22 | Medium | CRLF everywhere, no `.gitattributes` | repo-wide | **Resolved** `b294de2` + `3dbd1b4`; `gofmt -l .` returns nothing | [Hygiene](../06-quality/repository-hygiene.md) |
| 23 | Low | `go vet` warnings | `examples/extension-demo/main.go:131,183` | **Resolved** `6459348`; `go vet ./...` is clean | [Testing](../06-quality/testing-strategy.md) |
| 24 | Medium | **Zero test files** | repo-wide | **Resolved** `d92480c` + `720e580`, extended through Phase 2 — 13 test files, and all five priority-1 tests now pass. Still no CI to run them, and **`-race` cannot run on this toolchain** | [Testing](../06-quality/testing-strategy.md) |

**Still open after Phase 2:** findings 12, 13 (DSN escaping and pool sizing —
Phase 3.8), 14 (double shutdown — Phase 3.9), 16, 17 (registry `Close`, FIDO2
initialisation — Phase 3.5/3.6), and the residue of 18 (the orphan
`00002_create_products.sql`). Rotation of the credentials in finding 7 remains
outstanding and cannot be verified from the repository.

Three items outside this table are also still open and should not be read as
closed by Phase 2: `make generate` is broken (`cmd/generator` writes to
`internal/infrastructure/http/routes/routes.go`, which Phase 1.5 deleted — and it
**exits 0** while leaving the tree non-compiling); `.qwen/settings.json` and
`.qwen/settings.json.orig` are tracked and absent from `.gitignore`; and
`BuildApp`'s zero-module guard is unreachable because two modules are appended
unconditionally above it.

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
   clean), `d92480c` added the first seven test files, and Phase 2 added six
   more alongside the code they cover. **There is still no CI gate**, so nothing
   runs them automatically — the root cause is not closed. Phase 2 supplied the
   sharpest evidence yet for that: a data race shipped in `dcc6e5d` and was
   caught by a human re-reading the diff (`d130519`), because `-race` cannot run
   on this toolchain and no pipeline runs it anywhere else.*

---

## Related documents

- [Target architecture](target-architecture.md) — where this should land
- [Design principles](design-principles.md) — the rules that keep it there
- [Remediation plan](../07-roadmap/remediation-plan.md) — sequenced execution
