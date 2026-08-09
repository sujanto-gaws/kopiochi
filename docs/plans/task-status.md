# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner | State | PR / commit | Review |
| --- | --- | --- | --- | --- |
| A1–A4 | test-guardian / domain / transport | **merged** | #16, #17, #20 | APPROVE-WITH-NOTES ×4 |
| T1  | test-guardian | **merged** | #18 (`8b1b185`) | APPROVE-WITH-NOTES |
| D1  | persistence-engineer | **merged** | #22 (`fbc1639`, `cfb3f58`) | APPROVE-WITH-NOTES |
| D2, D3 | domain-engineer | **merged** | #23 | APPROVE-WITH-NOTES ×2 |
| E9a | domain-engineer | **merged** | #25 (`47d1569`) | APPROVE-WITH-NOTES |
| B1  | test-guardian | **merged** | #26 (`ea0d30f`) | APPROVE-WITH-NOTES |
| **B2** | transport-engineer | **PARTIAL — open PR #30** | `739c6df` | APPROVE-WITH-NOTES |
| **B3** | test-guardian | **open PR #30** | `1a72344` | APPROVE-WITH-NOTES |
| **D4** | persistence-engineer | **open PR #29** | `717642b` | APPROVE-WITH-NOTES |
| **B4** | test-guardian | **open PR #31** | `9c0ede5` | APPROVE-WITH-NOTES |
| B2a | transport-engineer | **BLOCKED — E16** | - | - |
| C1  | docs-scribe | ready once #30/#31 land | - | - |
| D5  | platform-engineer | **BLOCKED — E11, E13, E14** | - | - |
| D6  | platform-engineer | **BLOCKED — E9b, E9c, E12** | - | - |
| D7  | transport-engineer | **UNBLOCKED** — needs D6 | - | - |
| D8  | platform-engineer | **BLOCKED — E15, E11** | - | - |
| D9a/D9b, D10 | domain / platform | pending | - | - |

### ⚠ FOUR OPEN PRs — MERGE ORDER MATTERS

| Order | PR | Contents |
| --- | --- | --- |
| **1** | **#30** | **B2 + B3 — recovery of two commits I failed to push (see PROCESS-1)** |
| 2 | **#31** | B4 — the `internal/authn` archtest fence |
| — | **#29** | D4 — notification repositories (independent, any time) |

arch-reviewer verified #30+#31 green **in combination**, so B4 needs no rework once
`authntest` exists. **At #30's merge, run `make coverage-update`** to add
`internal/authn/authntest: 100` to `baseline` — see COVERAGE-BLINDSPOT.

## PROCESS-1 — my error: B2 and B3 were never pushed

PR #26 merged **only** `ea0d30f` (B1). B2 (`739c6df`) and B3 (`1a72344`) stacked onto
`feat/B1-fakeauth` afterwards and **I never pushed again** before it merged — then
reported #26 as "B1+B2+B3". Both commits survived only as local commits in one working
tree. **Found by B4's agent**, not by me, while checking its own dependencies.
Recovered and pushed as **PR #30**. Standing correction: **push after every commit that
lands on a branch with an open PR**, and verify the PR's commit list rather than the
branch's.

---

# ESCALATIONS — OPEN (13)

Ordered by severity.

## E16 — CONFIRMED IDOR, INHERITED FROM THE INTERNAL CORE
Any caller with **any valid access token** can `GET`/`PUT`/`DELETE` **any other user's
row** at `/api/v1/users/{id}`, plus unrestricted `POST /users`. Ids are `BIGSERIAL`, so
enumeration is trivial, and `internal/metrics` templatises the path so it does not even
show as distinct series.

**Nothing mitigates it at any layer** — `grep -rn "RequireRole|RequirePermission|Authorize"`
across `internal/httpx`, `internal/middleware`, `modules/user`, `cmd/api` → **zero
matches; no authorization layer exists in this repository**. The application service
takes `(ctx, id int64)` with no caller argument, so there is no seam a check could
attach to.

**Provenance (human's correction).** `modules/user` is the **profile-user module**,
**moved not written** in Phase 3.6b out of `internal/domain/user`,
`internal/application/user` and `internal/infrastructure/*`. Its package doc: *"It was
live the whole time… behaviour is unchanged."* So the defect **predates modularisation**
and is live code, not a demo. And `BOILERPLATE.md:279` names
`modules/user/transport/user.go` as the worked example adopters copy — **the exemplar
propagates it**.

**Root cause is not a missing `if`:** `auth_users.id` is uuid, `users.id` is BIGSERIAL,
and `grep -rn "auth_user_id|AuthUserID|REFERENCES users"` → zero matches. **There is no
value to compare.** B2 refused three shortcuts, all upheld at review:
`strconv.ParseInt(Subject)` (a uuid never parses — "an outage that lints clean"),
`Subject == chi.URLParam("id")` (always false, and *reads* like a real check), and
`Extra["email"]` (nil, and email is a mutable natural key).

## E16-ARCH — one root problem; two of its three instances dissolve
The repo has two user identifiers with no defined relationship. But:
- **Notification already keys on the identity uuid natively** (`recipient_id uuid`,
  `RecipientID uuid.UUID`). **⇒ D7 IS NOT BLOCKED** — `Subject` parses straight to
  `uuid.UUID` and compares to `RecipientID`.
- **⇒ D8/E15 needs no mapping** — only `auth_users.email` for an identity uuid.
- **`modules/user` is the only module that does not key on the identity uuid.**

**The split is DELIBERATE** — `modules/user/domain`'s doc: *"the profile/CRUD user
entity… distinct from the authentication identity."* So two entities is intent, and the
defect is narrower: **a profile table with no link to the identity it profiles.** That
**strengthens** option 1 — keying the profile by the identity uuid *completes* the stated
design.

**Decide in this order:** (1) does `users` stay distinct, or become a profile keyed by the
identity uuid? (2) if distinct → standardise its PK on the identity uuid (preferred;
blast radius is `modules/user` + one migration, and `p.Subject == id` becomes a *real*
check) or add `auth_user_id uuid UNIQUE` with a backfill (tactical). (3) unblock D7 now.
(4) unblock D8/E15 now. (5) close the IDOR under (1) — **only this must wait.**

## E18 — CI's coverage gate is ALREADY RED on `main` (NEW)
`modules/identity/infrastructure/persistence/repository` measures **57.1% against its
60% floor**. Independently confirmed by **both** D4's and B4's reviewers at `origin/main`
with a live database, and it is a real measurement (17 tests, all passing, genuinely
executed) — not a skip artifact.

Red only with `-with-database`, which **CI passes (`ci.yml:123`) and the local
`make coverage-check` target does not.** So the local gate is systematically weaker than
CI's. Its uncovered statements are the error paths of an already-tested package, so this
is bounded test work, not a design question. **Blocks every future PR that reaches the
coverage step.**

## COVERAGE-BLINDSPOT — absence from the profile is indistinguishable from a pass
**This corrects E7's recorded mechanism, which was wrong.** A package with **no test
file contributes no entry to the coverage profile at all**, and `floorFor` is only
consulted for packages *present* in the profile. So:
- **`auditlog` is not "an immediate failure the instant E8 is fixed" — it is invisible to
  the gate and will stay invisible forever**, until someone adds a test file or a
  baseline entry. Verified: `grep -c auditlog coverage.out` → `0`.
- The same hole swallowed **both repository packages**: one local run reported
  `all floors met, no regressions` while the profile contained **no data at all** for
  them, and they did not even appear in the `NOT CHECKED` list.
- **It will swallow `authntest` too** unless a baseline entry lands — the exact failure
  mode its 90% floor was chosen to prevent.

**Fix:** add `internal/authn/authntest`, and both `.../persistence/repository` packages,
to `baseline` (absence from `baseline` is a **hard error**, unlike absence from the
profile). Then either add `-with-database` to a `make coverage-check-db` target or accept
the divergence explicitly — but do not leave floors reachable only from CI while the
local gate prints green.

## GATE-INTEGRITY — `golangci-lint cache clean` is NOT sufficient (NEW)
D4's reviewer got **`0 issues`** from a fresh worktree immediately after
`golangci-lint cache clean`, with `path_relativity` warnings — the shared **Go build
cache** (not golangci-lint's own) still held export data recorded against another
worktree's absolute paths, so findings could not be relativised and were **silently
dropped**. An isolated `GOCACHE` gave the true two findings.
**Any lint green reported from a worktree, or on a machine that has built the same
sources from two paths, may be false.** Documented lint steps should use an isolated
`GOCACHE`. This is the second instance of the same class as BL16.

## E10 — second-factor bypass shape in merged identity code
`auth_models.go` maps three **nullable** columns to plain `string`. **`mfa_secret` is the
serious one:** `mfa_enabled = true AND mfa_secret IS NULL` yields `totp.Validate(code, "")`
— an empty secret base32-decodes fine and produces a **publicly computable** valid TOTP.
Fails **open**. Not reachable through current app paths. `schemacheck` cannot catch this
class (names only). **Fix:** `NOT NULL` migration after a data check; independently guard
`ValidateCode` on an empty secret; extend schemacheck to assert
`is_nullable == pointer-ness`. **Needs a task.**

## E11 — `infrastructure` may not import `application`, but D5 and D8 must ⚠ blocks D5
R1 forbids it; blueprint §5 puts `ChannelSender`, `RenderedMessage`, `TemplateRenderer`
and `ErrNonRetryable` in `application/ports.go`, so every sender must import
`application` **to spell its own method signature**. **Neither archtest nor depguard
catches it**, so D5/D8 would ship green while breaking the written table. Amend R1 to
`infrastructure → domain, own application's ports, …` and add an archtest rule permitting
`modules/X/infrastructure → modules/X/application` only. **Must land before D5.**

## E12 — `DispatchBatch (int, error)` cannot support D10's metrics or audit
D10 needs per-channel counters, a latency histogram, and an audit event on
**security-category dead-letter** (a security requirement). The dispatcher sees only
`(int, error)`; everything else is computed in `settle` and discarded. Prefer a
`DispatchObserver.Settled(n, outcome, err)` port — optional, nil ⇒ no-op — which also
fixes BL25's panic-visibility gap.

## E13 — no task ships an in-app sender; every in-app row would dead-letter ⚠
`inapp`/`webhook` appear **nowhere** in the task list. With D3's fail-closed behaviour,
at the D6 gate **every in-app notification goes straight to `dead`** — the channel D7's
read model exists to serve. Add `sender/inapp.go` to D5; record webhook as a v1 non-goal.

## E14 — template shape: two documents, orthogonal axes, both incomplete
Plan D5 = channel axis; blueprint §3 = MIME-part axis. **You need both.** Single-body is
wrong for v1 (no `text/plain` alternative is a deliverability penalty, and the mail it
costs is the security mail). Add `HTMLBody string` to `RenderedMessage` — an **added
field**, not a second render call. Amend D5's file list to include `application/ports.go`.

## E15 — nobody owns address resolution ⚠ blocks D8
Proposed **D8a**: `AddressResolver` in `infrastructure/sender/`, adapter in `cmd/api` over
identity's `UserRepository.FindByID`. Needs no `users` involvement (E16-ARCH).
**Blocker inside:** `identity.New` returns `(*module.Module, error)` and exposes nothing
resolvable. **Rejected:** snapshotting the address at enqueue — mails a **stale address
after an email change**, i.e. the breach alert goes to the attacker's new address.

## E9b — D6's sweep has no column to run on; the cheap window CLOSED
`next_attempt_at` holds its **pre-claim** value, so a freshly claimed row is
indistinguishable from a stalled one, and resetting it is a **double delivery**. D1
merged before this was decided, so it now costs a **new migration**, not a one-line edit
— my error. Options: (a) add `claimed_at`; (b) specify `ClaimBatch` to set
`next_attempt_at = now` on claim, with the semantics written into its doc comment.
**E9a raised the stakes:** a false positive now also burns an attempt and can dead-letter
a healthy in-flight row.

## E9c — the domain has no port to FIND stalled rows
`NotificationRepository` exposes no method returning `sending` rows; `ClaimBatch` selects
`pending` by contract. Needs `ClaimStalled(ctx, n, stalledBefore)` in `repository.go` — a
**D2 file**, so neither D4 nor D6 may add it. **Independently rediscovered by D4's
reviewer.** Blocked behind E9b (the predicate depends on which option wins).
**Correction:** "the sweep runs as one set-based UPDATE" is **no longer achievable** — a
set-based UPDATE that spends an attempt and dead-letters would be a second, untested copy
of the state machine. D6 must do select → `RecoverStalled` → `Save` per row.

## E17 — the conformance suite guards `detail` only
A textbook **RFC 6750** middleware leaking the reason via `WWW-Authenticate`
(`error_description=…`), the problem `type` URI, or `title`, with **no `detail` member at
all**, passes B3's suite with **zero findings** — and that is *the default output shape of
most OAuth/OIDC middleware*. Also: `{}` for every 401 is **invariant by vacuity**.
**Fix (one function, no new exports):** compare the whole rejection response across
cases, and treat an absent `detail` as a finding. B4's reviewer: this does **not** belong
with B4 (different files, different concern) — its own task.

## E7 — `auditlog` has no tests — **RESTATED, see COVERAGE-BLINDSPOT**
8 functions, no test file. **Not** self-revealing: it is invisible to the gate
permanently, not a failure waiting for E8. Fix is a test **or** a reasoned `exempt`
entry — plus a baseline entry so its absence becomes loud.

## E8 — `internal/db` — ⚠ THE ONE-LINE FIX IS NOT ENOUGH (corrected twice)
**Settled after four contradictory reports.** In the configuration **CI actually uses**
(one shared `TEST_DATABASE_URL`), **two** tests fail. Run in isolation on a fresh
database, `_RefusesCollidingData` **passes**. Both earlier single-number answers were
right about different configurations.

**One root cause plus one enabling condition:**
1. `goose.Down()` reverts only the **newest** migration, and D1's is now newest — so
   `_IsReversible` rolls back D1's migration and then asserts 00007's index is gone.
2. `testsupport.ScratchPostgres` is **not scratch** when `TEST_DATABASE_URL` is set
   (`db.go:63-68` returns the shared DSN), so `_IsReversible` leaves the database fully
   migrated and `_RefusesCollidingData`'s `UpTo(6)` becomes a no-op — goose says so:
   `no migrations to run. current version: 7`.

**⚠ The board's previously-recorded one-line fix would ship half a fix.** `DownTo(…, 6)`
at `migrations_test.go:56` clears `_IsReversible`, but that test **ends with `goose.Up`**,
so it still hands the next test a fully-migrated database — proven by running
`_RefusesCollidingData` alone against exactly that state. **The fix must do both:** pin
the rollback **and** give these tests their own database (or `DownTo(0)` before `UpTo(6)`).
**Class warning:** any `goose.Down()` caller breaks when a newer migration lands.
**Also still open:** the CI `golangci-lint` job **cannot start** (go1.24 vs 1.25.0) and has
produced **no findings at all**; `govulncheck` fails on stdlib advisories.

---

# RESOLVED
**E1** T1 rescued `tools/coverage`. **E2** SPI rationale holds. **E3** corrected changelog
scope. **E4** uniformity claim narrowed; `/auth/mfa/verify` an accepted divergence.
**E5** `request_id` accepted. **E6** `docs/plans/` tracked. **E9a** arrow added as
`RecoverStalled`; `Transition` unexported so the ruling is mechanical.
**depguard/authntest** — permitted under `list-mode: lax`; B1's reading was right.

---

# DOC AMENDMENTS I CANNOT MAKE
- **Phase B's title overclaims** — "consumers + conformance guarantee"; B2's consumer
  clause hit a confirmed stop condition. Name what shipped; record B2a.
- **`authn-spi-impact-analysis.md` §7 is now a trap** — its archtest row says two areas
  where the shipped fence has four, and it is self-refuting (line 205 requires an import
  line 202 forbids). A reader could "fix" the rule by deleting two areas.
- Plan `Config.Auth` → shipped `Config.AuthMiddleware` (D7's text too).
- Blueprint §4.1 jitter formula (code keeps it **inside** the cap); §6 vs §4.1 on where
  jitter lives; plan D2/D3 "+ `github.com/google/uuid`"; plan D6 `max_attempts >= 1`;
  plan D4 `persistence/repository/`; plan B1 file list; plan B4 coverage clause;
  §8.1's known-false "expected finding"; ADR 005 line 60.

---

# DISPATCH ADJUSTMENTS — banked

## Next small task (recommended by B4's reviewer, while context is fresh)
**Close the layer gap.** `modules/*` is recursive, so `modules/x/domain` may import
`internal/authn` and **neither** the new fence nor `TestDomainLayerStaysPure` objects —
confirmed by putting it in the tree and watching `make arch` stay green. depguard is
silent too. The rule's own docstring promises this protection. **Fix: add `internal/authn`
to the forbidden maps of `TestDomainLayerStaysPure` and
`TestApplicationLayerDoesNotTouchInfrastructure`.** Note the area cannot simply be
narrowed to `modules/*/transport` — B2 types the middleware at the module root.

## D6
- **Non-cancelled drain context** — `settle` calls `Save(ctx, …)`; a cancelled ctx leaves
  every in-flight row in `sending`.
- Shutdown burns retry budget (`context.Canceled` is retryable by default).
- **Jitter source must be concurrency-safe** — one field on `Service`, `workers`
  concurrent calls; `*math/rand.Rand` is not goroutine-safe.
- Propagate `NewService`'s error; `Validate()` needs `max_attempts >= 1`.
- Sweep is select → `RecoverStalled` → `Save` per row (E9c).

## D7 (unblocked)
- `Subject` → `uuid.Parse` → compare to `RecipientID`. Cross-user ⇒ **404 byte-identical
  to genuinely-not-found**, or the 404 is a 403 with extra steps.
- Protected preference combos ⇒ problem+json **422**.
- **Pagination precedent is D4's keyset cursor** — measured on a 20k-row mailbox: the
  planner decomposes the row-value comparison into an index bound plus a cheap tiebreak
  filter, 4 buffer hits at depth. Use it.
- **N4:** `MarkRead` does **not** restate `channel = 'inapp'`, unlike `ListForRecipient`
  and `MarkAllRead` — a recipient can mark their own email/webhook row read and get
  success. Not a security issue; know it before writing the handler test.

## C1
Carry E3's approved scope verbatim (lead with the transport-level break: `text/plain` →
`application/problem+json`; body → RFC 9457 with **no `error` member at all**;
`WWW-Authenticate` now sent where none ever was). Scope the claim to routes behind
`AuthRequired` and **state the `/auth/mfa/verify` carve-out positively**. Document
`request_id`. Every claim checked against **merged code**.

---

# BACKLOG
- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings (blame `6eed3aa`);
  the only lint findings repo-wide. Also a **dead duplicate** of
  `tools/schemacheck/schema_test.go`.
- **BL3** `jwks.go:7` imports `infrastructure/token` in production, contradicting R1;
  unenforced. Related to E11.
- **BL4** RSA keypair parsed twice at boot; deliberate.
- **BL5** `identity.New` exposes only `(*module.Module, error)` — see E15.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern.
- **BL12** Two stale worktrees under `.claude/worktrees` with stale config; exclude them
  for **config files**, not just code.
- **BL13** `/auth/mfa/verify` OAuth2-shaped 401 — accepted permanent divergence.
- **BL16** golangci-lint cache trap — **and see GATE-INTEGRITY, which is worse.**
- **BL17** `-require-profile=false` exits 0 on an empty profile; `tolerance = 0.05`
  softens **floors** as well as the ratchet; `tools/...` is exempt so the gate does not
  gate itself.
- **BL18** `cmd/api/protected_routes_test.go:157` asserts only `rec.Code != 401`.
- **BL19** Baselines lag actuals: `modules/identity/transport` 15.8 vs 18.9,
  `internal/httpx` 93.9 vs 94.0, `modules/user/transport` (100%) absent entirely, and no
  `modules/*/transport` floor pattern exists. Run `make coverage-update` at Phase B close.
- **BL24** The module is **at-least-once** by design — a dropped connection after DATA
  re-sends, so a user can get two password-changed mails.
- **BL25** Panic visibility: a panicking sender is recovered into an ordinary retryable
  failure, so `DispatchBatch` returns nil and D6 gets **no error to log**. Fix with E12's
  observer.
- **BL26** `internal/testsupport` at 8.5%, exempt, now load-bearing tree-wide.
- **BL27** Do **not** add `-trimpath` to `go test` — B1's import test uses `runtime.Caller`.
- **BL28** `dependency-rules.md`'s `## Enforcement` snippet is stale (v1 syntax, omits both
  transport rules) — a second, contradicting statement of the rules in the same file.
- **BL29 / BL30** Verification hygiene: a reviewer's mutation silently failed to apply and
  went green; a `python` board edit failed while the surrounding git pipeline reported
  success. **Verify the artifact, not the exit code around it.**
- **BL31** `TestModelsMatchMigratedSchema` **skips locally** even with Postgres live; CI
  has an explicit guard that fails the build if it skips, so only local runs are blind.
- **BL32** Two tools, one string, two meanings: `internal/authn` means "and everything
  below" in `arch_test.go` but "exactly this" in `policy.json`. Both right for their job;
  a maintainer reading one and editing the other will get it wrong. Cheapest fix is
  notation — spell the archtest areas with an explicit `/...` suffix.
- **BL33** `schemacheck` compares **column names only**, in both directions — it
  structurally cannot catch a tag whose *semantics* diverge from the column (the
  `default:true` class D4 found). Covering it needs `SQLDefault` vs `column_default`.
- **BL34** Shared-`TEST_DATABASE_URL` leaks state between test binaries; the full suite in
  parallel produces spurious `goose_db_version`/`pg_type` races. `-p 1` avoids it. Same
  root fragility as E8's enabling condition.
- **BL35** Arch failures are reported twice per violation (production package plus its
  `[p.test]` variant), so CI reads as 2N violations for N problems. Shared with all
  pre-existing rules.

---

# DEVIATIONS ACCEPTED
- **A1 (4), A2 (1), A2 copy-on-write, A3 (2), A4 (2)** — adjudicated; A4's `Scopes`-nil and
  empty-`sub` rejection both confirmed correct, the latter a strict tightening.
- **T1 (1)** — **refused** my instruction to baseline five unmeasurable packages.
  Reviewer: *"I would have blocked the PR had it complied."*
- **D1 (2)** + the `nullzero` payload fix added on review.
- **D2 (2)**, **D3 (8)** — all ACCEPT except the template shape (E14).
- **E9a (1)** — `Transition` unexported; with it exported, `sending→pending` *is* the bare
  arrow the ruling forbade.
- **B1 (4)** — two depguard globs instead of the one I specified (**mine was dead**).
- **B2 (3)** — `AuthMiddleware` name kept; tests added not simplified; **ownership check
  not delivered** (E16).
- **B3 (5)** — five invalid minters; **≥2** cases required; parsed media type.
- **D4 (4)** — `persistence/repository/` path; **`MarkRead` uses `coalesce`** rather than
  `read_at IS NULL`, because my instruction **contradicted the merged port doc** (which
  makes already-read a *no-op*, not an error) — reviewer confirmed the instruction was
  wrong and the shipped form preserves everything D7's 404 needs; the **`Enabled`
  model fix** (a `default:true` tag made `false` unrepresentable — every "turn this off"
  stored "on"; reviewer audited all 20 `default:` tags in the tree and confirmed it was
  the **unique** such case); `ClaimBatch` result order not asserted (`UPDATE … RETURNING`
  has no defined order).
- **B4 (3)** — rule added to the existing `arch_test.go`; **`cmd/**` deliberately absent**
  from the permitted list, so naming `authn.Middleware` in `container.go` will fail
  `make arch` (correct, but it will surprise someone); §7 left uncorrected.
