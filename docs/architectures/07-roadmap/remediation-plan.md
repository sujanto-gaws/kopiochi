# Remediation Plan

**Status:** In progress — Phases 0, 1, and 2 ✅ complete; Phases 3–5 not started
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2

A sequenced plan to move from [current state](../00-overview/current-state.md) to
[target architecture](../00-overview/target-architecture.md). Ordered by
dependency and risk, not by severity alone — some critical fixes depend on
groundwork.

Effort is expressed in relative sizes (S ≈ hours, M ≈ 1–2 days, L ≈ 3–5 days) for
one engineer familiar with the codebase.

Status legend: ✅ done · ◐ partially done · ⚠️ cannot be verified from the
repository · ⏳ not started.

| Phase | Status |
|---|---|
| **Phase 0** — Stop the bleeding | ✅ Complete (0.5's credential *rotation* excepted — see ⚠️ below) |
| **Phase 1** — Make the application work | ✅ Complete — exit criteria met |
| **Phase 2** — Close the security gaps | ✅ Complete — exit criteria met (`-race` coverage ⚠️ unavailable, see below) |
| **Phase 3** — Consolidate the structure | ⏳ Not started |
| **Phase 4** — Build the safety net | ⏳ Not started — no CI exists |
| **Phase 5** — Cleanup and hardening | ⏳ Not started |

---

## Phase 0 — Stop the bleeding (½ day) — ✅ COMPLETE

Small, independent, zero-risk changes that unblock everything else.

| # | Status | Task | Effort | Doc |
|---|--------|------|--------|-----|
| 0.1 | ✅ `b294de2` | Add `.gitattributes` (`* text=auto eol=lf`), `git add --renormalize .` as a **standalone commit** (`gofmt -s` followed in `3dbd1b4`) | S | [hygiene](../06-quality/repository-hygiene.md) |
| 0.2 | ✅ `4c72a83` | Repair `.gitignore` — remove markdown fences, add `bin/`, `keys/`, `*.pem`, `*.exe`, `coverage.*`, `*.zip` | S | [hygiene](../06-quality/repository-hygiene.md) |
| 0.3 | ✅ `1c5ac2c` | `git rm --cached bin/ .vscode/settings.json`. (The listed `claude-agents_1.zip` appears in no commit; `*.zip` is ignored regardless.) | S | [hygiene](../06-quality/repository-hygiene.md) |
| 0.4 | ✅ `9c302ad`, `1d3379e` | Create `.githooks/pre-commit` blocking `*.pem`, `keys/`, `bin/`, `.env` | S | [secrets](../03-configuration/secret-management.md) |
| 0.5 | ◐ / ⚠️ | Remove the DB password and JWT secret from `config/default.yaml` — ✅ `b74b358`, with `BindEnv` so `APP_*` actually works, and `252efb2` for `.env.example`. **Rotating** the exposed credentials is ⚠️ **outstanding**: it happens outside the repository and no commit can demonstrate it | S | [secrets](../03-configuration/secret-management.md) |
| 0.6 | ✅ `657b2dc` | Add `CONFIG ?= config/default.yaml` to the Makefile; delete the `db-migrate`/`db-seed` TODO targets | S | [migrations](../05-data/migration-strategy.md) |

**Exit criteria:** ✅ `gofmt -l .` returns nothing. ◐ No secret or binary is added
by a fresh `git add .` — true for binaries, keys, and archives, but `.qwen/` is
still **tracked** and still absent from `.gitignore`. ⚠️ Rotated credentials are
live — unverifiable from the repository; must be confirmed by the environment
owner. Until then, treat the exposed DB password and JWT secret as valid.

> 0.1 must land before any other formatting-touching change, or every subsequent
> diff will be whole-file noise.

---

## Phase 1 — Make the application work (3–4 days) — ✅ COMPLETE

The application served no business routes under `/api/v1`. Nothing else mattered
until that was fixed.

| # | Status | Task | Effort | Doc |
|---|--------|------|--------|-----|
| 1.1 | ✅ `d92480c` | Write the five priority-1 failing tests (empty container, route table, rate-limit concurrency, env override, schema drift). Four pass; the rate-limit concurrency test is deliberately `t.Skip`ped until Phase 2.1, since the defect it asserts is still live | M | [testing](../06-quality/testing-strategy.md) |
| 1.2 | ✅ `05b1051` | Add `internal/module` (`Deps`, `Module`). Shipped `Deps` carries `DB` and `Logger`; no `Clock` | S | [extensions](../01-modularity/extension-framework.md) |
| 1.3 | ✅ `5f6edfe`, `6d0c1b7` | Add `modules/identity/module.go` over the existing identity internals (move, don't rewrite). The move was from `internal/{domain,application,infrastructure}/auth/**` — this also completes what Phase 3.3 was written to do | M | [layout](../01-modularity/module-layout.md) |
| 1.4 | ✅ `ef76759` | Replace the container with `BuildApp`, including the zero-module guard. The guard is present but unreachable — two modules are appended unconditionally above it | M | [DI](../02-composition/dependency-injection.md) |
| 1.5 | ✅ `4fdc609` | Rewrite `routes.Setup` as `httpx.Mount`, passing the `/api/v1` group router into `Module.Routes`; `RouterGroup`/`RouteRegistrar` and `internal/infrastructure/http/routes/` deleted | S | [routing](../02-composition/routing-and-versioning.md) |
| 1.6 | ✅ `fbddccb` | Write identity's real migrations matching the bun models. **Shipped as `00003_create_auth_users`, `00004_create_auth_refresh_tokens`, `00005_create_auth_mfa_backup_codes`** — not the `app_user`/`role`/`refresh_token`/`mfa_secret` set named when this plan was written. There is no `role` table (roles and permissions are `TEXT[]` columns on `auth_users`) and no `mfa_secret` table (it is a column) | M | [migrations](../05-data/migration-strategy.md) |
| 1.7 | ✅ `40887de` | Split `/api/health` into `/healthz` + `/readyz` (with a real DB ping); `/health` kept as a deprecated alias | S | [routing](../02-composition/routing-and-versioning.md) |

**Exit criteria — all met:** ✅ `POST /api/v1/auth/login` returns a token against
a migrated database (`cmd/api/login_e2e_test.go`, `720e580`); ✅ `TestRouteTable`
passes; ✅ tests 1.1(a) and 1.1(b) are green.

> Known follow-up from this phase: `make generate` is broken. `cmd/generator`
> still writes to `internal/infrastructure/http/routes/routes.go`, which 1.5
> deleted.

---

## Phase 2 — Close the security gaps (2–3 days) — ✅ COMPLETE

Could run in parallel with Phase 1 after 1.4, since the middleware work is
independent of module wiring; in the event it ran after Phase 1.

| # | Status | Task | Effort | Doc |
|---|--------|------|--------|-----|
| 2.1 | ✅ `dcc6e5d`, `d130519` | Rewrite the rate limiter: token bucket, lock released before `next.ServeHTTP`, TTL eviction, `max_keys` cap. Clock injected for deterministic tests. `max_keys` **rejects new keys** rather than evicting — evict-oldest is gameable | M | [rate limiting](../04-security/rate-limiting.md) |
| 2.2 | ✅ `333968c` | `RealIP` from trusted-proxy CIDRs only; rate limiter and logs consume the resolved IP. Shipped as `internal/middleware/clientip.go`, replacing chi's `RealIP`; empty trusted list (the default) means trust nothing | S | [middleware](../04-security/middleware-hardening.md) |
| 2.3 | ✅ `87381d2` | CORS: allowlist-only default, no origin reflection outside the list, always `Vary: Origin`, reject `*` + credentials in `Config.Validate()`, stop 403-ing non-browser clients, preflight-only 204 | S | [middleware](../04-security/middleware-hardening.md) |
| 2.4 | ✅ `e0da81e` | Pin the JWT algorithm (`jwt.WithValidMethods`, RS256 exactly); validate `iss`, `aud`, `exp` with `auth.token_leeway` (default 30s) | S | [tokens](../04-security/token-architecture.md) |
| 2.5 | ✅ `946c1c8` | Introduce token classes (`cls`: `access`/`mfa`/`id`); `Validate(token, want Class)`; MFA tokens cannot access the API | M | [tokens](../04-security/token-architecture.md) |
| 2.6 | ✅ `0cf07d9` | Standardise on RS256; delete `internal/plugins/auth/jwt.go` — with its registration, the `APP_JWT_SECRET` binding, its placeholder check, the `plugins.auth.jwt` config block, and its `.env.example` entries | S | [tokens](../04-security/token-architecture.md) |
| 2.7 | ✅ `6d0c1b7`, `ef76759` | Auth middleware fails closed — a module needing auth fails to construct without a verifier. **Already done in Phase 1**; no Phase 2 commit was needed | S | [middleware](../04-security/middleware-hardening.md) |
| 2.8 | ✅ `0968aae` | Add security response headers: `nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, route-scoped CSP (`default-src 'none'` for the API, relaxed for `/swagger/*`), HSTS gated on `server.enable_hsts` (default false) | S | [middleware](../04-security/middleware-hardening.md) |
| 2.9 | ✅ `acc057d` | Config: `BindEnv` for `db.user`/`db.name` as well, placeholder rejection, `secret.String` on `DB.Password`, `Validate()` called from `Load` — and the timeout inversion **fixed**, `request_timeout` 60s → 25s | M | [config](../03-configuration/configuration-model.md) |

**Exit criteria — all met:** ✅ `TestRateLimitAllowsConcurrentRequests` passes,
unskipped and in the default suite; ✅
`TestValidate_RejectsMFATokenAsAccessToken` passes; ✅ the server refuses to
start with a placeholder `db.password` (`TestLoad_RejectsPlaceholderSecrets`).

Six new test files landed with the phase — `cors_test.go`,
`ratelimit_tokenbucket_test.go`, `clientip_test.go`, `security_headers_test.go`,
`secret_test.go`, and `jwt_test.go` — taking the suite from 7 files to 13.

> 2.1 was the highest-impact single change in this plan — it capped the server at
> one concurrent request.

> ⚠️ **`-race` coverage is outstanding and cannot be obtained here.** `go test
> -race` fails on every package with `0xc0000139`; the local toolchain is
> mingw-w64 GCC 8.1.0 (2018) and Go's Windows race runtime needs far newer. This
> matters concretely: `dcc6e5d` — the concurrency fix — shipped with a data race
> of its own on `p.now` / `p.initialized` / `p.burst`, found by **inspection**
> and fixed in `d130519`. Phase 4.4's CI runs on Linux and would cover it. Until
> then, concurrency correctness here is review-enforced. See
> [testing strategy](../06-quality/testing-strategy.md#race-detection-is-outstanding).

> **Not closed by this phase, and not to be read as closed:** `make generate` is
> still broken (it exits 0 while leaving the tree non-compiling);
> `.qwen/settings.json` and `.qwen/settings.json.orig` are still tracked and
> still absent from `.gitignore`; and `BuildApp`'s zero-module guard is still
> unreachable. None of these is a Phase 2 task; all three remain open.

---

## Phase 3 — Consolidate the structure (4–5 days) — ⏳ NOT STARTED

Now that the application works and is safe, remove the duplication. Doing this
earlier risks deleting something that turns out to be load-bearing.

> **Three tasks withdrawn.** This phase previously also contained 3.1 (split
> `internal/utils`), 3.3 (move `extensions/identity/**` → `modules/identity/**`),
> and 3.4 (consolidate three aquaculture fragments and write the missing
> repository, estimated L). `internal/utils`, `extensions/`, and any aquaculture
> file appear in no commit of this repository, so 3.1 and 3.4 had no subject and
> have been deleted. The identity move in 3.3 was in fact performed in Phase 1
> (`5f6edfe`, `6d0c1b7`) — from `internal/{domain,application,infrastructure}/auth/**`,
> not from `extensions/` — and is done. Numbering is preserved so existing
> references still resolve.

| # | Task | Effort | Doc |
|---|------|--------|-----|
| ~~3.1~~ | ~~Split `internal/utils`~~ — **withdrawn**, no such package has ever existed | — | [deps](../01-modularity/dependency-rules.md) |
| 3.2 | Add `depguard` rules + the module-isolation test; wire into CI | S | [deps](../01-modularity/dependency-rules.md) |
| ~~3.3~~ | ~~Move identity into `modules/`~~ — **done in Phase 1** (`5f6edfe`, `6d0c1b7`) | — | [layout](../01-modularity/module-layout.md) |
| ~~3.4~~ | ~~Consolidate aquaculture~~ — **withdrawn**, no aquaculture code has ever existed | — | [layout](../01-modularity/module-layout.md) |
| 3.5 | Convert CORS and rate limiting to direct construction in `internal/httpx` | S | [extensions](../01-modularity/extension-framework.md) |
| 3.6 | **Delete the dead frameworks:** `internal/extension/` (including `internal/extension/identity/`, 1,076 LOC, no importers), `internal/plugin/`, `internal/plugins/`, `examples/extension-demo/`. Must follow 3.5, which removes the last live consumer of `internal/plugin{,s}` | M | [extensions](../01-modularity/extension-framework.md) |
| 3.6b | **Move, do not delete:** `internal/domain/user`, `internal/application/user`, and the matching persistence/handler code become `modules/user/`. These are **live** — `cmd/api/container.go` builds the `user` module from them — so `internal/domain/` and `internal/application/` may only be removed once nothing imports them | M | [layout](../01-modularity/module-layout.md) |
| 3.7 | `go mod tidy`; drop `go-webauthn`, `go-tpm`, `cbor`, and `boombuler/barcode`. **Keep `pquerna/otp`** — `modules/identity/infrastructure/mfa/totp.go` uses it | S | [hygiene](../06-quality/repository-hygiene.md) |
| 3.8 | Fix `BuildDSN` with `net/url`; configure the `sql.DB` pool; add connect timeouts | S | [persistence](../05-data/persistence-and-pooling.md) |
| 3.9 | Lifecycle stack — single ownership, LIFO teardown, `Serve` returns errors instead of `log.Fatal` | M | [lifecycle](../02-composition/lifecycle-and-shutdown.md) |

**Exit criteria:** all business code lives under `modules/`; the import linter
passes; `go build ./...` and the full test suite still pass after every deletion;
module count and binary size both measurably down.

> ⚠️ **3.6 as previously written was dangerous.** Its delete list included
> `internal/domain/` and `internal/application/`, which hold the live
> profile-user stack that `cmd/api/container.go` depends on. Deleting them breaks
> the build. The task has been split: 3.6 deletes only what has no importer, and
> 3.6b *moves* the live code. Verify each path with
> `grep -rn "<import path>" --include=*.go .` before removing it.
>
> 3.6 is the step most likely to be deferred and must not be. Leaving the old
> frameworks in place recreates the original problem.

---

## Phase 4 — Build the safety net (3–4 days) — ⏳ NOT STARTED

| # | Task | Effort | Doc |
|---|------|--------|-----|
| 4.1 | `internal/testsupport` — testcontainers Postgres, fixtures, auth helpers | M | [testing](../06-quality/testing-strategy.md) |
| 4.2 | Unit tests: domain, tokens, config, CORS, rate limiter, recovery | L | [testing](../06-quality/testing-strategy.md) |
| 4.3 | Integration tests: repositories + handlers via `httptest` | L | [testing](../06-quality/testing-strategy.md) |
| 4.4 | CI pipeline: build, vet, lint, `-race` tests, `govulncheck`, `gitleaks`, size check | M | [testing](../06-quality/testing-strategy.md) |
| 4.5 | Migration CI: up → down → up, plus the model/schema drift test | M | [migrations](../05-data/migration-strategy.md) |
| 4.6 | Coverage floors + the no-regression ratchet | S | [testing](../06-quality/testing-strategy.md) |
| 4.7 | Injected loggers, request-scoped logger in context | M | [observability](../06-quality/observability.md) |
| 4.8 | Prometheus metrics + pool stats on an admin port | M | [observability](../06-quality/observability.md) |

**Exit criteria:** CI fails on a real regression; coverage floors enforced.

---

## Phase 5 — Cleanup and hardening (2–3 days) — ⏳ NOT STARTED

| # | Task | Effort | Doc |
|---|------|--------|-----|
| 5.1 | **One coordinated history rewrite** — purge binaries and secrets together | M | [hygiene](../06-quality/repository-hygiene.md) |
| 5.2 | Untrack generated swagger output; regenerate in CI with a no-diff check | S | [hygiene](../06-quality/repository-hygiene.md) |
| 5.3 | Move swagger UI behind a build tag; strip symbols in release builds | S | [hygiene](../06-quality/repository-hygiene.md) |
| 5.4 | Add `docker-compose.yml` and `.env.example`, or delete the Makefile targets referencing them | S | [hygiene](../06-quality/repository-hygiene.md) |
| 5.5 | Key rotation: `kid` headers + JWKS endpoint | M | [tokens](../04-security/token-architecture.md) |
| 5.6 | Refresh-token rotation with reuse detection | M | [tokens](../04-security/token-architecture.md) |
| 5.7 | Audit event stream | M | [observability](../06-quality/observability.md) |
| 5.8 | Transaction helper + Postgres error translation | S | [persistence](../05-data/persistence-and-pooling.md) |

> 5.1 requires team coordination and a re-clone by everyone. Schedule it; do not
> surprise anyone with it.

---

## Dependency graph

```
Phase 0 ──▶ Phase 1 ──▶ Phase 3 ──▶ Phase 5
   │            │           ▲          ▲
   │            └──▶ Phase 2 ┘          │
   └──────────────────▶ Phase 4 ────────┘
                    (starts after 1.4, runs alongside 3)
```

- Phase 0 blocks everything (line endings must settle first).
- Phase 2 needs only 1.4, so it can run in parallel with the rest of Phase 1.
- Phase 4 grows continuously from Phase 1 onward — tests are written with each
  change, not saved for the end.
- Phase 5's history rewrite comes last, when churn has settled.

---

## Milestones

| Milestone | Definition | After |
|---|---|---|
| **M1 — It works** | A real endpoint serves a real request against a real schema | Phase 1 |
| **M2 — It's safe** | No credential leak, no auth bypass, handles concurrency | Phase 2 — **reached**, with the caveat below |
| **M3 — It's coherent** | One layout, one framework, enforced boundaries | Phase 3 |
| **M4 — It's defended** | CI catches regressions; failures are observable | Phase 4 |
| **M5 — It's clean** | Small repo, small binary, rotatable keys | Phase 5 |

---

## Risks

| Risk | Mitigation |
|---|---|
| Phase 3 deletions remove something needed | Phase 1 wires and Phase 4 tests **before** Phase 3 deletes. Confirm no importer with `grep -rn` per path; 3.6's original delete list contained live code |
| History rewrite loses work | Backup mirror; announced freeze; single coordinated rewrite |
| Rotating the JWT secret logs everyone out | Schedule during a maintenance window; communicate |
| Line-ending normalisation conflicts with in-flight branches | Do 0.1 during a quiet period; rebase open branches immediately |

---

## Tracking

Each phase should become an issue with its tasks as a checklist. Each task
references its topic document, so the rationale travels with the work.

Progress is measured by the exit criteria, not by task count — Phase 1 is done
when a request succeeds end to end, regardless of how many boxes are ticked. By
that measure Phase 1 is done: `cmd/api/login_e2e_test.go` drives
`POST /api/v1/auth/login` against a migrated database and gets a token back.

Three caveats on "done" that the ticks do not capture:

- **Phase 0.5's rotation is unverifiable from here.** The repository can show
  that secrets left the working tree; it cannot show that the exposed values were
  rotated. Do not read the ✅ on the rest of Phase 0 as closing that exposure.
  Phase 2.6 deleting the HS256 plugin does not discharge it either — the secret
  is still in history, and pre-`0cf07d9` builds still accept tokens signed with
  it.
- **M2 is reached on the "handles concurrency" criterion by inspection, not by
  measurement.** `TestRateLimitAllowsConcurrentRequests` proves the server no
  longer serialises, but `-race` cannot run in this environment, and a real race
  did ship inside Phase 2 before being caught by review (`d130519`). Read M2 as
  "the known unsafe behaviours are fixed and tested", not as "concurrency is
  verified".
- **Milestones M1 and M2 are reached, M3–M5 are not.** Nothing in Phases 3–5 has
  started, and there is still no CI, so no exit criterion is enforced
  automatically — including Phase 2's own.

---

## Related documents

- [Current state](../00-overview/current-state.md)
- [Target architecture](../00-overview/target-architecture.md)
- [Design principles](../00-overview/design-principles.md)
