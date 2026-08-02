# Remediation Plan

**Status:** Proposed
**Date:** 2026-08-02

A sequenced plan to move from [current state](../00-overview/current-state.md) to
[target architecture](../00-overview/target-architecture.md). Ordered by
dependency and risk, not by severity alone — some critical fixes depend on
groundwork.

Effort is expressed in relative sizes (S ≈ hours, M ≈ 1–2 days, L ≈ 3–5 days) for
one engineer familiar with the codebase.

---

## Phase 0 — Stop the bleeding (½ day)

Small, independent, zero-risk changes that unblock everything else.

| # | Task | Effort | Doc |
|---|------|--------|-----|
| 0.1 | Add `.gitattributes` (`* text=auto eol=lf`), `git add --renormalize .` as a **standalone commit** | S | [hygiene](../06-quality/repository-hygiene.md) |
| 0.2 | Repair `.gitignore` — remove markdown fences, add `bin/`, `keys/`, `*.pem`, `*.exe`, `coverage.*`, `*.zip` | S | [hygiene](../06-quality/repository-hygiene.md) |
| 0.3 | `git rm --cached bin/ .vscode/settings.json`; delete `claude-agents_1.zip` | S | [hygiene](../06-quality/repository-hygiene.md) |
| 0.4 | Create `.githooks/pre-commit` blocking `*.pem`, `keys/`, `bin/`, `.env` | S | [secrets](../03-configuration/secret-management.md) |
| 0.5 | **Rotate** the DB password and JWT secret; remove both from `config/default.yaml` | S | [secrets](../03-configuration/secret-management.md) |
| 0.6 | Add `CONFIG ?= config/default.yaml` to the Makefile; delete the `db-migrate`/`db-seed` TODO targets | S | [migrations](../05-data/migration-strategy.md) |

**Exit criteria:** `gofmt -l .` returns nothing; no secret or binary is added by a
fresh `git add .`; rotated credentials are live.

> 0.1 must land before any other formatting-touching change, or every subsequent
> diff will be whole-file noise.

---

## Phase 1 — Make the application work (3–4 days)

The application currently serves no business routes. Nothing else matters until
that is true.

| # | Task | Effort | Doc |
|---|------|--------|-----|
| 1.1 | Write the five priority-1 failing tests (empty container, route table, rate-limit concurrency, env override, schema drift) | M | [testing](../06-quality/testing-strategy.md) |
| 1.2 | Add `internal/module` (`Deps`, `Module`) | S | [extensions](../01-modularity/extension-framework.md) |
| 1.3 | Add `modules/identity/module.go` over the existing identity internals (move, don't rewrite) | M | [layout](../01-modularity/module-layout.md) |
| 1.4 | Replace the empty container with `BuildApp`, including the zero-module guard | M | [DI](../02-composition/dependency-injection.md) |
| 1.5 | Rewrite `routes.Setup` as `httpx.Mount`, passing the `/api/v1` group router into `Module.Routes` | S | [routing](../02-composition/routing-and-versioning.md) |
| 1.6 | Write identity's real migrations (`app_user`, `role`, `refresh_token`, `mfa_secret`) matching the bun models | M | [migrations](../05-data/migration-strategy.md) |
| 1.7 | Split `/api/health` into `/healthz` + `/readyz` (with a real DB ping) | S | [routing](../02-composition/routing-and-versioning.md) |

**Exit criteria:** `POST /api/v1/auth/login` returns a token against a migrated
database; `TestRouteTable` passes; tests 1.1(a) and 1.1(b) go green.

---

## Phase 2 — Close the security gaps (2–3 days)

Can run in parallel with Phase 1 after 1.4, since the middleware work is
independent of module wiring.

| # | Task | Effort | Doc |
|---|------|--------|-----|
| 2.1 | Rewrite the rate limiter: token bucket, lock released before `next.ServeHTTP`, TTL eviction, `max_keys` cap | M | [rate limiting](../04-security/rate-limiting.md) |
| 2.2 | `RealIP` from trusted-proxy CIDRs only; rate limiter and logs consume the resolved IP | S | [middleware](../04-security/middleware-hardening.md) |
| 2.3 | CORS: allowlist-only default, no origin reflection outside the list, always `Vary: Origin`, reject `*` + credentials at config load, stop 403-ing non-browser clients | S | [middleware](../04-security/middleware-hardening.md) |
| 2.4 | Pin the JWT algorithm; validate `iss`, `aud`, `exp` with leeway | S | [tokens](../04-security/token-architecture.md) |
| 2.5 | Introduce token classes (`cls`); `Validate(token, want Class)`; MFA tokens cannot access the API | M | [tokens](../04-security/token-architecture.md) |
| 2.6 | Standardise on RS256; delete `internal/plugins/auth/jwt.go` | S | [tokens](../04-security/token-architecture.md) |
| 2.7 | Auth middleware fails closed — a module needing auth fails to construct without a verifier | S | [middleware](../04-security/middleware-hardening.md) |
| 2.8 | Add security response headers | S | [middleware](../04-security/middleware-hardening.md) |
| 2.9 | Config: `BindEnv` for secrets, placeholder rejection, `secret.String`, `Validate()` incl. the timeout inversion | M | [config](../03-configuration/configuration-model.md) |

**Exit criteria:** `TestRateLimitAllowsConcurrentRequests` passes;
`TestValidate_RejectsMFATokenAsAccessToken` passes; the server refuses to start
with a placeholder secret.

> 2.1 is the highest-impact single change in this plan — it currently caps the
> server at one concurrent request.

---

## Phase 3 — Consolidate the structure (4–5 days)

Now that the application works and is safe, remove the duplication. Doing this
earlier risks deleting something that turns out to be load-bearing.

| # | Task | Effort | Doc |
|---|------|--------|-----|
| 3.1 | Split `internal/utils` into `internal/platform/{paging,crypto,id}` and `internal/httpx`; delete `utils` | M | [deps](../01-modularity/dependency-rules.md) |
| 3.2 | Add `depguard` rules + the module-isolation test; wire into CI | S | [deps](../01-modularity/dependency-rules.md) |
| 3.3 | Move `extensions/identity/**` → `modules/identity/**` (transport rename included) | M | [layout](../01-modularity/module-layout.md) |
| 3.4 | Consolidate the three aquaculture fragments into `modules/aquaculture/`; write the repository that never existed | L | [layout](../01-modularity/module-layout.md) |
| 3.5 | Convert CORS and rate limiting to direct construction in `internal/httpx` | S | [extensions](../01-modularity/extension-framework.md) |
| 3.6 | **Delete** `internal/extension/`, `internal/plugin/`, `internal/plugins/`, `examples/extension-demo/`, `internal/modules/`, `internal/domain/`, `internal/application/`, `extensions/` | M | [extensions](../01-modularity/extension-framework.md) |
| 3.7 | `go mod tidy`; drop `go-webauthn`, `go-tpm`, `cbor` (and `otp`/`barcode` if MFA is deferred) | S | [hygiene](../06-quality/repository-hygiene.md) |
| 3.8 | Fix `BuildDSN` with `net/url`; configure the `sql.DB` pool; add connect timeouts | S | [persistence](../05-data/persistence-and-pooling.md) |
| 3.9 | Lifecycle stack — single ownership, LIFO teardown, `Serve` returns errors instead of `log.Fatal` | M | [lifecycle](../02-composition/lifecycle-and-shutdown.md) |

**Exit criteria:** exactly one module layout exists; the import linter passes; no
0-byte files; module count and binary size both measurably down.

> 3.6 is the step most likely to be deferred and must not be. Leaving the old
> frameworks in place recreates the original problem.

---

## Phase 4 — Build the safety net (3–4 days)

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

## Phase 5 — Cleanup and hardening (2–3 days)

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
| **M2 — It's safe** | No credential leak, no auth bypass, handles concurrency | Phase 2 |
| **M3 — It's coherent** | One layout, one framework, enforced boundaries | Phase 3 |
| **M4 — It's defended** | CI catches regressions; failures are observable | Phase 4 |
| **M5 — It's clean** | Small repo, small binary, rotatable keys | Phase 5 |

---

## Risks

| Risk | Mitigation |
|---|---|
| Phase 3 deletions remove something needed | Phase 1 wires and Phase 4 tests **before** Phase 3 deletes |
| History rewrite loses work | Backup mirror; announced freeze; single coordinated rewrite |
| Rotating the JWT secret logs everyone out | Schedule during a maintenance window; communicate |
| Aquaculture repository is larger than estimated (3.4) | It is genuinely unwritten — treat the estimate as a floor and re-scope after 3.3 |
| Line-ending normalisation conflicts with in-flight branches | Do 0.1 during a quiet period; rebase open branches immediately |

---

## Tracking

Each phase should become an issue with its tasks as a checklist. Each task
references its topic document, so the rationale travels with the work.

Progress is measured by the exit criteria, not by task count — Phase 1 is done
when a request succeeds end to end, regardless of how many boxes are ticked.

---

## Related documents

- [Current state](../00-overview/current-state.md)
- [Target architecture](../00-overview/target-architecture.md)
- [Design principles](../00-overview/design-principles.md)
