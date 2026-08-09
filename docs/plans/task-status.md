# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner | State | PR / commit | Review |
| --- | --- | --- | --- | --- |
| A1–A4 | test-guardian / domain / transport | **merged** | #16, #17, #20 | APPROVE-WITH-NOTES ×4 |
| T1  | test-guardian | **merged** | #18 (`8b1b185`) | APPROVE-WITH-NOTES |
| D1  | persistence-engineer | **merged** | #22 | APPROVE-WITH-NOTES |
| D2, D3 | domain-engineer | **merged** | #23 | APPROVE-WITH-NOTES ×2 |
| E9a | domain-engineer | **merged** | #25 (`47d1569`) | APPROVE-WITH-NOTES |
| B1  | test-guardian | **merged** | #26 (`ea0d30f`) | APPROVE-WITH-NOTES |
| B2  | transport-engineer | **merged — PARTIAL** | #30 (`739c6df`) | APPROVE-WITH-NOTES |
| B3  | test-guardian | **merged** | #30 (`1a72344`) | APPROVE-WITH-NOTES |
| D4  | persistence-engineer | **merged** | #29 (`717642b`) | APPROVE-WITH-NOTES |
| B4  | test-guardian | **merged** | #31 (`9c0ede5`) | APPROVE-WITH-NOTES |
| **T3** | platform-engineer | **approved — open PR #33** | `7569689`, `3ed26fa` | APPROVE-WITH-NOTES |
| B2a | transport-engineer | **BLOCKED — E16** | - | - |
| T2  | test-guardian | ready — close the layer gap | - | - |
| T4  | platform-engineer | ready — **fix the lint job** (see E8/Q2) | - | - |
| T5  | platform-engineer | ready — **swagger cold-cache fix** (see SWAGGER) | - | - |
| C1  | docs-scribe | **ready** — Phase B complete | - | - |
| D5  | platform-engineer | **BLOCKED — E11, E13, E14** | - | - |
| D6  | platform-engineer | **BLOCKED — E9b, E9c, E12** | - | - |
| D7  | transport-engineer | **UNBLOCKED** — needs D6 | - | - |
| D8  | platform-engineer | **BLOCKED — E15, E11** | - | - |
| D9a/D9b, D10 | domain / platform | pending | - | - |

**Phase A and Phase B are complete on `main`.** Open PR: **#33** (T3).
`gh pr merge` and pushes to `main` are blocked by the sandbox classifier, so a human
merges; branch pushes and PR creation work.

**Owed at Phase B close:** run **`make coverage-update`** — `internal/authn/authntest`
measures 100% with no `baseline` entry, and BL19's three baselines lag their actuals.

## Merge log
#15 `d63200b` rename · #16 `f594ebc` A1+A2 · #17 `d74ec67` A3 · #18 `0c0a1cf` T1 ·
#20 `a475151` A4 · #22 `21c94bb` D1 · #23 `68402ec` D2+D3 · #25 E9a ·
#26 `5481dbd` **B1 only** (see PROCESS-1) · #29 `73409cc` D4 ·
#30 `42b7a98` **B2+B3 recovered** · #31 `ad5ba26` B4 ·
board: #19/#21/#24/#27/#28/#32 → `ca397f1`

## PROCESS-1 — B2 and B3 were never pushed
PR #26 merged **only** `ea0d30f` (B1). B2 and B3 stacked onto the same branch and **I
never pushed again** before it merged, then reported #26 as all three. They survived only
as local commits. **Found by B4's agent**, not by me. Recovered as #30.
**Standing correction:** push after **every** commit onto a branch with an open PR, and
verify the **PR's commit list**, not the branch's.

## PROCESS-2 — I published a wrong mechanism three times in one area
E7 → "COVERAGE-BLINDSPOT" → retracted. All three statements were about whether a
test-less package is measured, and **the first one was right**. Root cause: two agents and
I all measured on a machine running **go1.25.0**, which has a `covdata` regression that
suppresses coverage lines for test-less packages. We generalised a toolchain bug into a
claim about Go's coverage model, and I wrote it to the board as a *correction* of a
correct entry.
**Standing correction:** before recording a mechanism as general, reproduce it on a
**second toolchain or in CI**. A/B on one machine is not a mechanism.

---

# ESCALATIONS — OPEN (13)

Ordered by severity.

## E16 — CONFIRMED IDOR, INHERITED FROM THE INTERNAL CORE
Any caller with **any valid access token** can `GET`/`PUT`/`DELETE` **any other user's
row** at `/api/v1/users/{id}`, plus unrestricted `POST /users`. Ids are `BIGSERIAL`;
`internal/metrics` templatises the path, so enumeration is invisible in metrics.

**Nothing mitigates it at any layer.** `grep -rn "RequireRole|RequirePermission|Authorize"`
across `internal/httpx`, `internal/middleware`, `modules/user`, `cmd/api` → **zero matches;
no authorization layer exists in this repository.** The application service takes
`(ctx, id int64)` with no caller argument, so there is no seam a check could attach to.

**Provenance (human's correction).** `modules/user` is the **profile-user module**,
**moved not written** in Phase 3.6b out of `internal/domain/user`,
`internal/application/user` and `internal/infrastructure/*`. Its package doc: *"It was live
the whole time… behaviour is unchanged."* So the defect **predates modularisation** and is
live code, not a demo. `BOILERPLATE.md:279` names `modules/user/transport/user.go` as the
worked example adopters copy — **the exemplar propagates it**.

**Root cause is not a missing `if`:** `auth_users.id` is uuid, `users.id` is BIGSERIAL, and
`grep -rn "auth_user_id|AuthUserID|REFERENCES users"` → zero matches. **There is no value to
compare.** B2 refused three shortcuts, all upheld: `strconv.ParseInt(Subject)` (a uuid never
parses — "an outage that lints clean"), `Subject == chi.URLParam("id")` (always false, and
*reads* like a real check), `Extra["email"]` (nil, and a mutable natural key).

## E16-ARCH — one root problem; two of three instances dissolve
- **Notification already keys on the identity uuid natively** ⇒ **D7 IS NOT BLOCKED**.
- **D8/E15 needs no mapping** — only `auth_users.email` for an identity uuid.
- **`modules/user` is the only module that does not key on the identity uuid.**

**The split is DELIBERATE** — `modules/user/domain`'s doc: *"the profile/CRUD user entity…
distinct from the authentication identity."* So the defect is narrower: **a profile table
with no link to the identity it profiles.** That **strengthens** keying the profile by the
identity uuid — it *completes* the stated design.

**Decide in order:** (1) does `users` stay distinct or become a profile keyed by the identity
uuid? (2) if distinct → standardise its PK on the identity uuid (preferred; `p.Subject == id`
becomes a *real* check) or add `auth_user_id uuid UNIQUE` (tactical, two keys forever).
(3) unblock D7 now. (4) unblock D8/E15 now. (5) close the IDOR — **only this must wait.**

## E18 — CI's coverage gate is red on `main`, and there are TWO failures behind it
1. `modules/identity/infrastructure/persistence/repository` — **57.1% vs its 60% floor**.
   A real measurement (17 tests, all passing). Bounded test work on error paths.
2. **`modules/identity/infrastructure/auditlog` — 0.0% vs the same 60% floor** (see E7).
Both are **latent, not absent**: the `build` job dies at `go test -race` (E8), so the
`coverage floors and ratchet` step is `skipped` in every recent run. **Budget for both the
moment E8 clears.**

## E7 — `auditlog` has no tests and FAILS its floor — original entry REINSTATED
8 functions, one file, no test file. It **is** emitted in the profile at 0.0% and **is**
checked against the pre-existing `modules/*/infrastructure/...` 60% floor. It is not
excused by `requires_database` (which lists only `.../persistence/repository`).
**Confirmed in real CI on `main` at go1.25.0**, which printed
`…/auditlog  coverage: 0.0% of statements`. The floor failure is **real and latent** — CI
has simply never reached the coverage step.
Fix: a test, **or** a reasoned `exempt` entry. Not a lower floor.

## ~~COVERAGE-BLINDSPOT~~ — RETRACTED IN FULL
**Every claim in it was wrong.** It said a test-less package contributes no profile entry
and is therefore invisible to the gate forever. Settled by an A/B across three toolchains
plus real CI logs:
- **go1.25.0** (this machine): `go: no such tool "covdata"` → `grep -c auditlog
  coverage.out` = **0**. This is a **Go 1.25.0 regression**, not Go's coverage model.
- **go1.25.12** and a complete **go1.25.8** SDK: `auditlog  coverage: 0.0%`, **10** profile
  lines, floor evaluated and failed.
- **Real CI on `main`, before T3:** printed `…/auditlog  coverage: 0.0% of statements`.
**Correct mechanism:** a test-less package **with coverable statements** is emitted at 0.0%
and **is** subject to floors. A test-less package with **no coverable statements** (the
`models` packages, `internal/module`, `internal/version` — pure declarations) yields
`[no test files]` and no entries. **Absence from `baseline` is NOT a hard error** —
`tools/coverage/main.go:143` only ratchets `if base, ok := policy.Baseline[pkg]; ok`. The
hard error is the reverse: present in `baseline`, absent from the profile.
Two sub-claims also retracted: **`authntest` is not swallowed** (it reports
`100.0% (floor 90%)` and is checked via its floor, baseline or not), and the repository
packages appear correctly — `NOT CHECKED` without `-with-database`, real numbers in CI.

## GATE-INTEGRITY — `golangci-lint cache clean` is NOT sufficient
A reviewer got **`0 issues`** from a fresh worktree right after `golangci-lint cache clean`,
with `path_relativity` warnings: the shared **Go build cache** held export data from another
worktree's paths, so findings were **silently dropped**. An isolated `GOCACHE` gave the true
two. **Any lint green from a worktree may be false** — including some of mine. Use an
isolated `GOCACHE`.

## E10 — second-factor bypass shape in merged identity code
`auth_models.go` maps three **nullable** columns to plain `string`. **`mfa_secret`:**
`mfa_enabled = true AND mfa_secret IS NULL` → `totp.Validate(code, "")` — an empty secret
base32-decodes fine and yields a **publicly computable** valid TOTP. Fails **open**. Not
reachable through current app paths. `schemacheck` cannot catch this class (BL33).
**Fix:** `NOT NULL` migration after a data check; guard `ValidateCode` on an empty secret;
extend schemacheck to assert `is_nullable == pointer-ness`. **Needs a task.**

## E11 — `infrastructure` may not import `application`, but D5 and D8 must ⚠ blocks D5
Blueprint §5 puts the sender ports in `application/ports.go`, so every sender must import it
**to spell its own method signature** — which R1 forbids and **neither archtest nor depguard
catches**. Amend R1 to `infrastructure → domain, own application's ports, …` and add an
archtest rule permitting `modules/X/infrastructure → modules/X/application` only.
**Must land before D5.**

## E12 — `DispatchBatch (int, error)` cannot support D10's metrics or audit
Per-channel counters, a latency histogram, and an audit event on **security-category
dead-letter** are all computed in `settle` and discarded. Prefer a
`DispatchObserver.Settled(n, outcome, err)` port — optional, nil ⇒ no-op — which also fixes
BL25's panic-visibility gap.

## E13 — no task ships an in-app sender ⚠
`inapp`/`webhook` appear **nowhere** in the task list. With D3's fail-closed behaviour, at
the D6 gate **every in-app notification goes straight to `dead`** — the channel D7 exists to
serve. Add `sender/inapp.go` to D5; record webhook as a v1 non-goal.

## E14 — template shape: two documents, orthogonal axes, both incomplete
Plan D5 = channel axis; blueprint §3 = MIME-part axis. **You need both.** Single-body is
wrong for v1 (no `text/plain` alternative is a deliverability penalty, and the mail it costs
is the security mail). Add `HTMLBody string` to `RenderedMessage`; amend D5's file list to
include `application/ports.go`.

## E15 — nobody owns address resolution ⚠ blocks D8
Proposed **D8a**: `AddressResolver` in `infrastructure/sender/`, adapter in `cmd/api` over
identity's `UserRepository.FindByID`. Needs no `users` involvement. **Blocker inside:**
`identity.New` exposes nothing resolvable. **Rejected:** snapshotting the address at enqueue
— mails a **stale address after an email change**, i.e. the breach alert goes to the
attacker's new address.

## E9b — D6's sweep has no column to run on; the cheap window CLOSED
`next_attempt_at` holds its **pre-claim** value, so a freshly claimed row is
indistinguishable from a stalled one and resetting it is a **double delivery**. D1 merged
before this was decided, so it now costs a **new migration** — my error. Options: (a) add
`claimed_at`; (b) specify `ClaimBatch` to set `next_attempt_at = now` on claim, with the
semantics written into its doc comment. **E9a raised the stakes:** a false positive now also
burns an attempt and can dead-letter a healthy in-flight row.

## E9c — the domain has no port to FIND stalled rows
Needs `ClaimStalled(ctx, n, stalledBefore)` in `repository.go` — a **D2 file**, so neither D4
nor D6 may add it. Blocked behind E9b. **Correction:** the sweep can **no longer** be one
set-based UPDATE — that would be a second, untested copy of the state machine. D6 must do
select → `RecoverStalled` → `Save` per row.

## E17 — the conformance suite guards `detail` only
A textbook **RFC 6750** middleware leaking the reason via `WWW-Authenticate`, the problem
`type` URI, or `title`, with **no `detail` member at all**, passes B3's suite with **zero
findings** — and that is the default shape of most OAuth/OIDC middleware. Also `{}` for every
401 is invariant by vacuity. **Fix:** compare the whole rejection response across cases and
treat an absent `detail` as a finding. **Its own task**, not B4's.

## E8 — `internal/db` — the one-line fix is NOT enough
In the configuration **CI uses** (one shared `TEST_DATABASE_URL`), **two** tests fail; in
isolation `_RefusesCollidingData` passes — which reconciles four contradictory reports.
1. `goose.Down()` reverts only the **newest** migration, and D1's is now newest.
2. `testsupport.ScratchPostgres` is **not scratch** when `TEST_DATABASE_URL` is set
   (`db.go:63-68`), so `_IsReversible` leaves the DB fully migrated and
   `_RefusesCollidingData`'s `UpTo(6)` becomes a no-op.
**The recorded one-line fix ships half a fix:** `_IsReversible` **ends with `goose.Up`**, so
even pinned it still hands the next test a migrated database. **Do both:** pin the rollback
**and** isolate the database (or `DownTo(0)` before `UpTo(6)`).
**Class warning:** any `goose.Down()` caller breaks when a newer migration lands.

---

# CI — what is red on `main`, precisely (verified in run logs)

| Job | State | Notes |
| --- | --- | --- |
| **Vulnerabilities and secrets** | 🟢 **fixed by T3** | Confirmed green in CI run **31325881382**. `gitleaks` was never failing. |
| Migrations up/down/up | 🟢 green | Stays green through pgx v5.9.2. |
| Binary size | 🟢 green | Reported, not gated. |
| **Swagger spec** | 🔴 **red once after T3 merges, then self-heals** | See SWAGGER below. **Not a T3 regression.** |
| **Build, vet, test** | 🔴 red | Three causes, two not yet reached: E8's two failures; the notification concurrency **flake**; then E18's two floor failures once E8 clears. |
| **golangci-lint** | 🔴 red | Has **never once run the configured linters**. See below. |

## SWAGGER — new, needs a task (T5)
`setup-go`'s module cache is keyed on `go.sum`, so **any** `go.sum` change gives
`Cache is not found`; `swag init --parseDependency` then hits `go: downloading …` mid-parse
and aborts with `cannot find all dependencies`. Reproduced green locally with a warm cache.
**Fix:** one `go mod download` step before `generate` in the swagger job. It will bite the
first post-merge `main` run and then recover.

## LINT — root cause found, needs a task (T4)
`golangci/golangci-lint-action@v6` with `version: latest` resolves to **golangci-lint
v1.64.8**, the last v1, built with **go1.24** — hence `the Go language version (go1.24) used
to build golangci-lint is lower than the targeted Go version`. Bumping the `go` directive
cannot help: `1.25.0` already exceeded `go1.24`. Stacked behind it, `.golangci.yml:13`
declares `version: "2"`, which v1.64.8 **cannot parse at all**.
**Fix:** `golangci/golangci-lint-action@v8` (v7+ is required for golangci-lint v2) with
`version: v2.12.2` — **pin it**; `latest` is what produced the silent v1 pin.
**Scope it as "make the job run AND clear what it finds"**, not a one-line YAML edit: the
linter has never executed against this tree. Expect at least BL1's two errcheck findings.

---

# RESOLVED
**E1** T1 rescued `tools/coverage`. **E2** SPI rationale holds. **E3** corrected changelog
scope. **E4** uniformity claim narrowed. **E5** `request_id` accepted. **E6** `docs/plans/`
tracked. **E9a** arrow added as `RecoverStalled`; `Transition` unexported.
**depguard/authntest** permitted under `list-mode: lax`.
**T3** the vulnerability gate — `GO-2026-5856` (crypto/tls, reachable from `httpx.Serve`)
cleared by the toolchain bump; `GO-2026-5004` (SQL injection in pgx) cleared by
`v5.9.1 → v5.9.2`. **pgx was reachable in the production call graph**
(`main → serve → NewRouter → database/sql → pgx stdlib → SanitizeSQL`) but **not
exploitable**: every caller is gated on `QueryExecModeSimpleProtocol`, which our config
never selects (`BuildDSN` sets only `sslmode` and `application_name`; `QueryExecMode`
appears nowhere in the repo).

---

# DOC AMENDMENTS I CANNOT MAKE
Phase B's title overclaims; `authn-spi-impact-analysis.md` §7 is a **trap** (two areas where
the fence has four, and self-refuting); plan `Config.Auth` → `Config.AuthMiddleware` (D7 too);
blueprint §4.1 jitter formula and §6-vs-§4.1; plan D2/D3 "+ `github.com/google/uuid`"; plan
D6 `max_attempts >= 1`; plan D4 `persistence/repository/`; plan B1 file list; plan B4
coverage clause; §8.1's known-false "expected finding"; ADR 005 line 60.

---

# DISPATCH ADJUSTMENTS — banked

**T2 (ready)** — close the layer gap: `modules/*` is recursive in B4's fence, so
`modules/x/domain` may import `internal/authn` and **neither** the fence nor
`TestDomainLayerStaysPure` objects (confirmed by putting it in the tree). Add `internal/authn`
to the forbidden maps of both existing rules. **Do not** narrow the area to
`modules/*/transport` — B2 types the middleware at the module root.

**Makefile `-` removal** — the leading `-` on `coverage-check`'s test step now **swallows a
genuine failure** (`make: [coverage-check] Error 1 (ignored)`), and its justifying comment
cites the covdata limitation that go1.25.12 repaired. Two-line task; **sequence it after E8**
or `make coverage-check` becomes unrunnable locally the day it lands.

**D6** — non-cancelled drain context; shutdown burns retry budget; **jitter source must be
concurrency-safe** (`*math/rand.Rand` is not); propagate `NewService`'s error;
`max_attempts >= 1`; sweep is per-row (E9c).

**D7** — `Subject` → `uuid.Parse` → compare to `RecipientID`; cross-user ⇒ **404
byte-identical** to genuinely-not-found; protected combos ⇒ **422**; pagination precedent is
D4's keyset cursor (measured on 20k rows: index bound + cheap tiebreak filter, 4 buffer hits);
**`MarkRead` does not restate `channel = 'inapp'`**, unlike its siblings.

**C1 (ready)** — carry E3's approved scope verbatim; scope the claim to routes behind
`AuthRequired`; **state the `/auth/mfa/verify` carve-out positively**; document `request_id`.
Every claim checked against **merged code**.

---

# BACKLOG
- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings; also a **dead duplicate**
  of `tools/schemacheck/schema_test.go`.
- **BL3** `jwks.go:7` imports `infrastructure/token` in production, contradicting R1.
- **BL4** RSA keypair parsed twice at boot; deliberate.
- **BL5** `identity.New` exposes only `(*module.Module, error)` — see E15.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern.
- **BL12** Two stale worktrees under `.claude/worktrees`; exclude for **config files** too.
- **BL13** `/auth/mfa/verify` OAuth2-shaped 401 — accepted permanent divergence.
- **BL16** golangci-lint cache trap — and see GATE-INTEGRITY, which is worse.
- **BL17** `-require-profile=false` exits 0 on an empty profile; `tolerance = 0.05` softens
  **floors** as well as the ratchet; `tools/...` is exempt so the gate does not gate itself.
- **BL18** `cmd/api/protected_routes_test.go:157` asserts only `rec.Code != 401`.
- **BL19** Baselines lag actuals: `modules/identity/transport` 15.8 vs 18.9, `internal/httpx`
  93.9 vs 94.0, `modules/user/transport` (100%) absent, no `modules/*/transport` floor.
- **BL24** The notification module is **at-least-once** by design.
- **BL25** Panic visibility: a panicking sender is recovered into a retryable failure, so
  `DispatchBatch` returns nil and D6 gets **no error to log**. Fix with E12's observer.
- **BL26** `internal/testsupport` at 8.5%, exempt, now load-bearing tree-wide.
- **BL27** Do **not** add `-trimpath` to `go test`.
- **BL28** `dependency-rules.md`'s `## Enforcement` snippet is stale.
- **BL29/BL30** Verification hygiene: a mutation silently failed to apply and went green; a
  `python` board edit failed while the surrounding git pipeline reported success. **Verify the
  artifact, not the exit code around it.**
- **BL31** `TestModelsMatchMigratedSchema` **skips locally** even with Postgres live.
- **BL32** Two tools, one string, two meanings: `internal/authn` is recursive in
  `arch_test.go`, exact in `policy.json`. Fix by notation (`/...` suffix).
- **BL33** `schemacheck` compares **column names only** — it cannot catch a tag whose
  *semantics* diverge from the column (D4's `default:true` class).
- **BL34** Shared-`TEST_DATABASE_URL` leaks state between test binaries; `-p 1` avoids it.
- **BL35** Arch failures are reported twice per violation (`[p.test]` variant).
- **BL36** `TestNotificationRepo_ClaimBatchIsExclusiveUnderConcurrency` is a **timing flake** —
  failed on `main`'s run, passed on #33's. Its `require.NotEmpty` on both claimers fails when
  the goroutines do not overlap. Fails safe (never a false pass) but it will redden CI at
  random. Needs a fix that forces overlap or drops that assertion.
- **BL37** **No CI job builds the `Dockerfile`**, so a bad `golang:` tag would ship
  undetected. The image is now coupled to `go.mod` by an unenforced comment.

---

# DEVIATIONS ACCEPTED
- **A1 (4), A2 (1)+copy-on-write, A3 (2), A4 (2)** — adjudicated.
- **T1 (1)** — **refused** my instruction to baseline five unmeasurable packages.
- **D1 (2)** + the `nullzero` payload fix on review.
- **D2 (2)**, **D3 (8)** — all ACCEPT except the template shape (E14).
- **E9a (1)** — `Transition` unexported.
- **B1 (4)** — two depguard globs instead of the one I specified (**mine was dead**).
- **B2 (3)** — `AuthMiddleware` kept; tests added; **ownership check not delivered** (E16).
- **B3 (5)** — five invalid minters; **≥2** cases required; parsed media type.
- **D4 (4)** — `persistence/repository/`; **`MarkRead` uses `coalesce`** because **my
  instruction contradicted the merged port doc**; the **`Enabled` model fix** (a `default:true`
  tag made `false` unrepresentable — every "turn this off" stored "on"; all 20 `default:` tags
  audited, it was the **unique** case); `ClaimBatch` order not asserted.
- **B4 (3)** — rule in the existing `arch_test.go`; **`cmd/**` deliberately absent**; §7 left
  uncorrected.
- **T3 (1)** — a 9-line `Dockerfile` comment beyond the version bump, documenting the
  unenforced coupling between the image tag and the `go` directive. Judged justified: the file
  is densely commented by convention, and drift there means a green pipeline over a vulnerable
  image. Also: `go 1.25.12` is pinned in the **`go` directive** rather than a `toolchain` line,
  deliberately — the directive is a hard **security floor** that fails closed under
  `GOTOOLCHAIN=local`, whereas `toolchain` is advisory and silently overridable.
