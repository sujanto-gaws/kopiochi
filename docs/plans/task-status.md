# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner | State | PR / commit | Review |
| --- | --- | --- | --- | --- |
| A1  | test-guardian | **merged** | #16 (`9e50896`) | APPROVE-WITH-NOTES |
| A2  | domain-engineer | **merged** | #16 (`6288dcd`) | APPROVE-WITH-NOTES |
| A3  | transport-engineer | **merged** | #17 (`4a969f2`) | APPROVE-WITH-NOTES |
| A4  | transport-engineer | **merged** | #20 (`5a0b56e`) | APPROVE-WITH-NOTES |
| T1  | test-guardian | **merged** | #18 (`8b1b185`) | APPROVE-WITH-NOTES |
| D1  | persistence-engineer | **merged** | #22 (`fbc1639`, `cfb3f58`) | APPROVE-WITH-NOTES |
| D2  | domain-engineer | **merged** | #23 (`049f4b0`, `468dbc0`) | APPROVE-WITH-NOTES |
| D3  | domain-engineer | **merged** | #23 (`13ab921`) | APPROVE-WITH-NOTES |
| E9a | domain-engineer | **merged** | #25 (`47d1569`) | APPROVE-WITH-NOTES |
| B1  | test-guardian | **merged** | #26 (`ea0d30f`) | APPROVE-WITH-NOTES |
| B2  | transport-engineer | **merged — PARTIAL** | #30 (`739c6df`) | APPROVE-WITH-NOTES |
| B3  | test-guardian | **merged** | #30 (`1a72344`) | APPROVE-WITH-NOTES |
| D4  | persistence-engineer | **merged** | #29 (`717642b`) | APPROVE-WITH-NOTES |
| B4  | test-guardian | **merged** | #31 (`9c0ede5`) | APPROVE-WITH-NOTES |
| B2a | transport-engineer | **BLOCKED — E16** (carry-forward of B2's unmet clause) | - | - |
| T2  | test-guardian | **ready — recommended next** (close the layer gap) | - | - |
| C1  | docs-scribe | **ready** — Phase B complete | - | - |
| D5  | platform-engineer | **BLOCKED — E11, E13, E14** | - | - |
| D6  | platform-engineer | **BLOCKED — E9b, E9c, E12** | - | - |
| D7  | transport-engineer | **UNBLOCKED** — needs D6 | - | - |
| D8  | platform-engineer | **BLOCKED — E15, E11** | - | - |
| D9a/D9b | domain / platform | pending | - | - |
| D10 | platform-engineer | pending — owns `policy.json` cleanups | - | - |

**Phase A and Phase B are complete and on `main`.** No open code PRs.
`gh pr merge` and pushes to `main` are blocked by the sandbox classifier, so a human
merges; branch pushes and PR creation work.

### ⚠ OWED AT PHASE B CLOSE — one command, closes a real hole
Run **`make coverage-update`** and commit the result. `internal/authn/authntest` measures
**100%** but has **no `baseline` entry**, so if it ever loses its self-test its 90% floor
goes inert and nothing says a word — the exact failure mode the floor was chosen to
prevent. See COVERAGE-BLINDSPOT. Same command also settles BL19's three lagging baselines.

## Merge log

| PR | Merged as | Contents |
| --- | --- | --- |
| #15 | `d63200b` | package rename — the Phase A base |
| #16 | `f594ebc` | A1, A2, + the human's docs/roster commit |
| #17 | `d74ec67` | A3 — `httpx.Unauthorized` |
| #18 | `0c0a1cf` | T1 — `tools/coverage` |
| #20 | `a475151` | A4 — identity implements the contract |
| #22 | `21c94bb` | D1 — notification migration + bun models |
| #23 | `68402ec` | D2 + D3 — notification domain + application |
| #25 | — | E9a — `sending→pending` stall recovery |
| #26 | `5481dbd` | **B1 only** — see PROCESS-1 |
| #29 | `73409cc` | D4 — notification repositories |
| #30 | `42b7a98` | **B2 + B3 — recovered** (see PROCESS-1) |
| #31 | `ad5ba26` | B4 — the `internal/authn` archtest fence |
| #19/#21/#24/#27/#28 | `f6e93ff`/`19008c6`/`e6d0d93`/`8f3cd01`/`1cb2ef6` | board |

## PROCESS-1 — my error: B2 and B3 were never pushed

PR #26 merged **only** `ea0d30f` (B1). B2 (`739c6df`) and B3 (`1a72344`) stacked onto
`feat/B1-fakeauth` afterwards and **I never pushed again** before it merged — then I
reported #26 as "B1+B2+B3". Both commits survived only as local commits in one working
tree. **Found by B4's agent**, not by me, while it checked its own dependencies.
Recovered as **PR #30**, merged `42b7a98`.
**Standing correction:** push after **every** commit that lands on a branch with an open
PR, and verify the **PR's commit list**, not the branch's.

---

# ESCALATIONS — OPEN (13)

Ordered by severity, not by number.

## E16 — CONFIRMED IDOR, INHERITED FROM THE INTERNAL CORE
**Raised by:** transport-engineer (B2) as a stop condition; **confirmed independently by
arch-reviewer** at the schema level.

**PROVENANCE — corrected 2026-08-09 on the human's input, and it matters.**
`modules/user` is the **profile-user business module**, and it is not new code. Its own
package doc says so: *"Before Phase 3.6b this stack was spread across
`internal/domain/user`, `internal/application/user`,
`internal/infrastructure/persistence/{models,repository}` and
`internal/infrastructure/http/handlers`… **It was live the whole time** — which is why
3.6 deleted the dead frameworks but this code was *moved*, not deleted. **Behaviour is
unchanged**; only its address and its constructor are new."*
So this was **kopiochi's internal-core implementation of the user abstraction**, later
relocated into a module. Three consequences:
1. **The defect is older than `modules/user`.** My earlier dating (`68f75e22`,
   2026-04-02) is the authorship of the *moved* code. The gap dates to the internal-core
   era; **modularisation neither introduced nor widened it.**
2. **It is live application code, not a demo.** The package doc's "live the whole time"
   forecloses the most comfortable reading. Severity stands.
3. **This is boilerplate** (`BOILERPLATE.md`; impact-analysis §8.3 — *"Boilerplate stage,
   no external consumers outside our control"*), and `modules/user` is the reference
   module adopters copy — `BOILERPLATE.md:279` points at `modules/user/transport/user.go`
   as the worked example. **An IDOR in the exemplar propagates into every downstream
   project that follows it.** That is an amplifier, not a mitigation.

**The exposure.** Any caller holding **any valid access token for any account** can,
against **any other user's row**:

| Verb | Route | Effect |
| --- | --- | --- |
| `GET` | `/api/v1/users/{id}` | read `id`, `name`, `email` (`transport/user.go:91-112`) |
| `PUT` | `/api/v1/users/{id}` | overwrite `name`, `email` (`:127-157`) |
| `DELETE` | `/api/v1/users/{id}` | delete the record (`:169-189`) |
| `POST` | `/api/v1/users` | unrestricted creation behind mere authentication (`:56-78`) |

Ids are **`BIGSERIAL`** — sequential and trivially enumerable. `internal/metrics`
templatises the path, so enumeration does not surface as distinct metric series.

**Nothing mitigates it — every layer checked:** route mounting adds no wrapper
(`internal/httpx/routes.go:61-65`); the middleware is **authentication only**
(`cmd/api/container.go:103`), and `grep -rn "RequireRole|RequirePermission|Authorize"`
across `internal/httpx`, `internal/middleware`, `modules/user`, `cmd/api` returns
**zero matches — no authorization layer exists anywhere in this repository**; the
application service takes `(ctx, id int64)` with **no caller argument of any kind**, so
there is no seam a check could attach to; the repository has no owner predicate.
Roles/permissions *are* in the JWT (`jwt.go:188-189`) but the middleware discards them
by design (`middleware.go:58-67`).

**Root cause is not a missing `if`.** `auth_users.id` is `uuid` (`migrations/00003:8`);
`users.id` is `BIGSERIAL` (`migrations/00001:4`);
`grep -rn "auth_user_id|AuthUserID|REFERENCES users"` → **zero matches**. The Principal
carries no email (`Extra` nil by design). **There is no value to compare.**

**B2 refused three shortcuts, all correctly** (reviewer upheld each):
`strconv.ParseInt(Subject)` — a uuid never parses, so all three id routes 404
permanently: "an outage that lints clean"; `Subject == chi.URLParam("id")` — always
false, same outage, and *reads* like a real check to the next maintainer;
`Extra["email"]` — nil, needs an identity-side change, and email is a **mutable natural
key**. It also declined a bare `MustFromContext`, which would have been dead code making
the diff *look* complete.

## E16-ARCH — one root problem; two of its three instances dissolve
The repo has **two user identifiers with no defined relationship**. But:
- **Notification already keys on the identity uuid, natively** — `recipient_id uuid`,
  `RecipientID uuid.UUID`, `notification_preferences.user_id` uuid. No mapping layer.
- **⇒ D7 IS NOT BLOCKED.** `authn.MustFromContext(r).Subject` parses directly to
  `uuid.UUID` and compares to `RecipientID`; the cross-user 404 falls out with no
  mapping. **I previously recorded D7 as blocked — that was wrong.**
- **⇒ D8/E15 needs no mapping either** — only `auth_users.email` for an identity uuid.
- **`modules/user` is the only module that does not key on the identity uuid.**

**The split is DELIBERATE, which narrows the options.** `modules/user/domain`'s package
doc states the design outright: *"the profile/CRUD user entity (table: users, PK: int64).
This is distinct from the authentication identity in domain/auth (table: auth_users, PK:
uuid). Use this package for general profile management… Use domain/auth for login,
tokens, MFA, and access control."* So two entities is **intent, not accident** — and that
**strengthens** option 1 rather than contradicting it: a profile record keyed by the
identity it profiles is the stated design, *completed*. The actual defect is narrower
than "two ids": **it is a profile table with no link to the identity it profiles.** A
profile that cannot say whose profile it is cannot support an ownership check, by
construction.

**Options, in the order they should be decided:**
1. **First decide the ownership model:** does `users` remain a distinct entity, or become
   a **profile keyed by the identity uuid**? Everything else follows. `auth_users`
   already carries `username`, `email`, `name`, `roles`, `permissions`; `users` carries
   `name`, `email`. The overlap is total, and the second copy of a person's email is
   exactly the staleness hazard E15 already rejected.
2. If `users` stays distinct → **standardise its PK on the identity uuid** (preferred;
   blast radius is `modules/user` + one migration, and it deletes the root cause because
   `p.Subject == id` becomes a *real* check), **or** add `auth_user_id uuid UNIQUE` with
   a backfill (tactical; leaves two keys per user permanently).
3. **Unblock D7 now** — independent of 1 and 2.
4. **Unblock D8/E15 now** via the identity-side resolver — independent of 1 and 2.
5. Close the IDOR under whichever answer 1 produces. **Only this must wait.**

This is the **oldest** unresolved question in the effort — it predates the authn SPI work
entirely — and its cost compounds: every task touching a user identity pays a
stop-and-escalate tax.

## E18 — CI's coverage gate is ALREADY RED on `main` (NEW)
`modules/identity/infrastructure/persistence/repository` measures **57.1% against its 60%
floor**. Independently confirmed by **both** D4's and B4's reviewers at `origin/main`
against a live database, and it is a real measurement (17 tests, all passing, genuinely
executed) — not a skip artifact.

Red only with `-with-database`, which **CI passes (`ci.yml:123`) and the local
`make coverage-check` target does not** — so the local gate is systematically weaker than
CI's. Its uncovered statements are the error paths of an already-tested package, so this
is bounded test work, not a design question. **Blocks every future PR that reaches the
coverage step.**

## COVERAGE-BLINDSPOT — absence from the profile is indistinguishable from a pass (NEW)
**This corrects E7's recorded mechanism, which was wrong.** A package with **no test file
contributes no entry to the coverage profile at all**, and `floorFor` is only consulted
for packages *present* in the profile.
- **`auditlog` is not "an immediate failure the instant E8 is fixed" — it is invisible to
  the gate and will stay invisible forever**, until someone adds a test file or a baseline
  entry. Verified: `grep -c auditlog coverage.out` → `0`.
- The same hole swallowed **both repository packages**: one local run reported
  `all floors met, no regressions` while the profile contained **no data at all** for
  them, and they did not even appear in the `NOT CHECKED` list.
- **It will swallow `authntest` too** unless a baseline entry lands.
**Fix:** add `internal/authn/authntest` and both `.../persistence/repository` packages to
`baseline` — absence from `baseline` is a **hard error**, unlike absence from the profile.
Then either add `-with-database` to a `make coverage-check-db` target or accept the
divergence explicitly, but do not leave floors reachable only from CI while the local gate
prints green.

## GATE-INTEGRITY — `golangci-lint cache clean` is NOT sufficient (NEW)
D4's reviewer got **`0 issues`** from a fresh worktree immediately after
`golangci-lint cache clean`, with `path_relativity` warnings — the shared **Go build
cache** (not golangci-lint's own) still held export data recorded against another
worktree's absolute paths, so findings could not be relativised and were **silently
dropped**. An isolated `GOCACHE` gave the true two findings.
**Any lint green reported from a worktree, or on a machine that has built the same sources
from two paths, may be false** — including some of mine. Documented lint steps should use
an isolated `GOCACHE`. Second instance of BL16's class.

## E10 — second-factor bypass shape in merged identity code
`auth_models.go` maps three **nullable** columns to plain `string`, collapsing NULL to
`""`. **`mfa_secret` is the serious one:** `mfa_enabled = true AND mfa_secret IS NULL`
yields `totp.Validate(code, "")` — an empty secret base32-decodes fine and produces a
**publicly computable** valid TOTP. It fails **open**. `password_hash` fails *closed*
(bcrypt rejects `""`) but read-then-save silently rewrites NULL to `''`.
Not reachable through current app paths. **`tools/schemacheck` cannot catch this class**
(names only — see BL33).
**Remediation, in order:** `NOT NULL` in a new migration after a data check; independently
guard `ValidateCode` on an empty secret (cheap, fails closed, no schema dependency);
extend schemacheck to assert `is_nullable == pointer-ness`. **Needs a task.**

## E11 — `infrastructure` may not import `application`, but D5 and D8 must ⚠ blocks D5
R1 forbids `infrastructure → application`. But blueprint §5 puts `ChannelSender`,
`RenderedMessage`, `TemplateRenderer` and `ErrNonRetryable` in `application/ports.go` — so
every sender must import `application` **just to spell its own method signature**.
**Neither archtest nor depguard catches it**, so D5 and D8 would ship green while silently
breaking the written table.
**Amend the rule, not the code** — an adapter implementing a port declared by the inner
ring is textbook hexagonal; R1's real intent is that infrastructure must not *invoke use
cases*. Amend R1 to `infrastructure → domain, own application's ports, bun, pgx, external
clients`, and add an archtest rule permitting `modules/X/infrastructure →
modules/X/application` while forbidding another module's. **Must land before D5.**

## E12 — `DispatchBatch (int, error)` cannot support D10's metrics or audit
D10 needs `notification_{sent,failed,dead}_total{channel}`, a latency histogram, and an
`internal/audit` event on **security-category dead-letter** (a security requirement). The
dispatcher sees only `(int, error)`: it cannot know sent vs failed vs dead, on which
channel, or that a security row died — all computed inside `settle` and discarded.
**Settle now or pay at D10** as a forced change to a merged contract plus a D6 rewrite.
Prefer a `DispatchObserver.Settled(n, outcome, err)` port — optional, nil ⇒ no-op, like
`jitter` — which also fixes BL25's panic-visibility gap.

## E13 — no task ships an in-app sender; every in-app row would dead-letter ⚠
`inapp` and `webhook` appear **nowhere** in the plan's task list. Blueprint §3 lists
`sender/inapp.go` and `sender/webhook.go`; D5 ships only `log.go`, D8 only
`email_smtp.go`. With D3's correct fail-closed behaviour, an unregistered channel
dead-letters immediately — so at the D6 merge gate **every in-app notification goes
straight to `dead`**, the exact channel D7's read model and route table exist to serve.
**Add `sender/inapp.go` to D5's file list** and record **webhook as an explicit v1
non-goal** in blueprint §13.

## E14 — template shape: two documents, orthogonal axes, both incomplete
Plan D5 says `<key>.<channel>.tmpl` (**channel** axis); blueprint §3 says
`*.subject.tmpl`/`*.html.tmpl`/`*.text.tmpl` (**MIME-part** axis). **You need both.** D3
shipped a hybrid that papers over the hole. **Single-body is not correct for v1** —
`text/html` with no `text/plain` alternative is a measurable deliverability penalty, and
the mail it costs is the security mail. Add `HTMLBody string` to `RenderedMessage` — an
**added field**, not a second render call (`multipart/alternative` needs both parts in one
`Send`). **Do not reopen D3** — amend D5's file list to include `application/ports.go`.

## E15 — nobody owns address resolution ⚠ blocks D8
Proposed task **D8a, gating D8**: declare `AddressResolver` in `infrastructure/sender/`
(`Resolve(ctx, recipientID) (string, error)`) with a "no such user" sentinel mapping to
`ErrNonRetryable` while DB failure stays retryable; adapter in `cmd/api` over identity's
`UserRepository.FindByID`. **Per E16-ARCH this needs no `users` involvement.**
**Blocker inside the blocker:** `identity.New` returns `(*module.Module, error)` and
exposes nothing resolvable. Widening it mirrors D6's own
`New(...) (*module.Module, Service, error)` shape.
**Rejected, recorded:** snapshotting the address at enqueue — copies PII into `payload` and
mails a **stale address after an email change**, i.e. "your password was changed" goes to
the address the attacker just replaced.

## E9b — D6's sweep has no column to run on; the cheap window has CLOSED
`next_attempt_at` holds its **pre-claim** value, so a row claimed 2 seconds ago already
looks stalled — resetting it while the original worker is still sending is a **double
delivery**. Verified: `notifications` has only `next_attempt_at`, `created_at`, `sent_at`.
**D1 merged before this was decided, so it now costs a NEW migration** — my error; I
flagged the time-pressure and did not hold the PR.
Options: (a) new migration adding `claimed_at`; (b) specify `ClaimBatch` to set
`next_attempt_at = now` on claim, with the semantics written into its doc comment.
**E9a raised the cost of getting this wrong:** a false positive now *also* burns an attempt
and can dead-letter a healthy in-flight row. The predicate must be exact.

## E9c — the domain has no port to FIND stalled rows
`NotificationRepository` exposes no method returning `sending` rows; `ClaimBatch` selects
`pending` by contract. Needs `ClaimStalled(ctx, n, stalledBefore)` in `repository.go` — a
**D2 file**, so under guardrail 9 neither D4 nor D6 may add it. **Independently
rediscovered by D4's reviewer.** Blocked behind E9b (the predicate depends on which option
wins). Sequence: settle E9b → small D2-follow-on adds the port → D4-follow-on implements →
D6 consumes.
**Correction:** "the sweep runs as one set-based UPDATE" is **no longer achievable** — a
set-based UPDATE that spends an attempt and dead-letters would be a second, untested copy
of the state machine. D6 must do select → `RecoverStalled` → `Save` per row.

## E17 — the conformance suite guards `detail` only
A textbook **RFC 6750** middleware leaking the reason via `WWW-Authenticate`
(`error_description=…`), the problem `type` URI, or `title`, with **no `detail` member at
all**, passes B3's suite with **zero findings** — and that is *the default output shape of
most OAuth/OIDC middleware*, i.e. exactly the replacement B3 exists to constrain. Also:
`{}` for every 401 is **invariant by vacuity**.
**Fix (one function, no new exports):** compare the whole rejection response — challenge
header value, `type`, `title`, `detail` — across cases, and treat an absent `detail` as a
finding. B4's reviewer: this does **not** belong with B4 (different files, different
concern). **Its own task.**

## E7 — `auditlog` has no tests — RESTATED, see COVERAGE-BLINDSPOT
8 functions, no test file. **Not self-revealing:** it is invisible to the gate permanently,
not a failure waiting for E8. Fix is a test **or** a reasoned `exempt` entry — plus a
baseline entry so its absence becomes loud.

## E8 — `internal/db` — ⚠ THE ONE-LINE FIX IS NOT ENOUGH (corrected twice)
**Settled after four contradictory reports.** In the configuration **CI actually uses**
(one shared `TEST_DATABASE_URL`), **two** tests fail. Run in isolation on a fresh database,
`_RefusesCollidingData` **passes**. Both earlier single-number answers were right about
different configurations.

**One root cause plus one enabling condition:**
1. `goose.Down()` reverts only the **newest** migration, and D1's is now newest — so
   `_IsReversible` rolls back D1's migration and then asserts 00007's index is gone.
2. `testsupport.ScratchPostgres` is **not scratch** when `TEST_DATABASE_URL` is set
   (`db.go:63-68` returns the shared DSN), so `_IsReversible` leaves the database fully
   migrated and `_RefusesCollidingData`'s `UpTo(6)` becomes a no-op — goose says so out
   loud: `no migrations to run. current version: 7`.

**⚠ The previously-recorded one-line fix would ship half a fix.** `DownTo(…, 6)` at
`migrations_test.go:56` clears `_IsReversible`, but that test **ends with `goose.Up`**, so
it still hands the next test a fully-migrated database — proven by running
`_RefusesCollidingData` alone against exactly that state. **The fix must do both:** pin the
rollback **and** give these tests their own database (or `DownTo(0)` before `UpTo(6)`).
**Class warning:** any `goose.Down()` caller breaks when a newer migration lands.
**Also still open:** the CI `golangci-lint` job **cannot start** (go1.24 vs 1.25.0) and has
produced **no findings at all**; `govulncheck` fails on stdlib advisories.

---

# ESCALATIONS — RESOLVED

**E1** T1 rescued `tools/coverage`; root cause was the bare `coverage/` pattern
(arch-reviewer right, my "correction" wrong — I read the working file, not the committed
blob). **Standing rule:** cite `git show <ref>:<path>`, never the working file.
**E2** SPI rationale holds as stated. **E3** corrected changelog scope approved.
**E4** uniformity claim narrowed; `/auth/mfa/verify` an accepted permanent divergence.
**E5** `request_id` accepted in the canonical body. **E6** `docs/plans/` tracked.
**E9a** arrow added as `RecoverStalled` so recovery costs an attempt; `Transition`
unexported so the ruling is mechanical rather than advisory.
**depguard/authntest** — permitted under `list-mode: lax`; B1's reading was right, and the
contrary note was withdrawn.

---

# DOC AMENDMENTS I CANNOT MAKE

- **Phase B's title overclaims** — "consumers + conformance guarantee"; B2's consumer
  clause hit a confirmed stop condition. Name what shipped; record B2a.
- **`authn-spi-impact-analysis.md` §7 is now a trap** — its archtest row names two areas
  where the shipped fence has four, and it is **self-refuting** (line 205 requires an
  import line 202 forbids). A reader could "fix" the rule by deleting two areas.
- Plan `Config.Auth` → shipped `Config.AuthMiddleware` (D7's text at plan line 289 too).
- Blueprint §4.1's jitter formula (code keeps jitter **inside** the cap); §6 vs §4.1 on
  where jitter lives; plan D2/D3 "+ `github.com/google/uuid`"; plan D6 `max_attempts >= 1`;
  plan D4 `persistence/repository/`; plan B1's file list; plan B4's coverage clause;
  §8.1's known-false "expected finding"; ADR 005 line 60.

---

# DISPATCH ADJUSTMENTS — banked

## T2 — close the layer gap (recommended next; small, and the context is fresh)
`modules/*` is recursive in B4's fence, so **`modules/x/domain` may import
`internal/authn`** and **neither** the fence nor `TestDomainLayerStaysPure` objects —
confirmed by putting it in the tree and watching `make arch` stay green; depguard is silent
too. The fence's own docstring promises exactly this protection.
**Fix:** add `internal/authn` to the forbidden maps of `TestDomainLayerStaysPure` and
`TestApplicationLayerDoesNotTouchInfrastructure`, both already in `tools/archtest/arch_test.go`.
**Do not** narrow the area to `modules/*/transport` — B2 types the middleware at the module
root (`modules/user/module.go`), so `modules/*` recursion is load-bearing.

## D6
- **Non-cancelled drain context** — `settle` calls `Save(ctx, …)`; a cancelled ctx fails the
  save and leaves every in-flight row in `sending`.
- **Shutdown burns retry budget** — `context.Canceled` is retryable by default.
- **Jitter source must be concurrency-safe** — one field on `Service`, `workers` concurrent
  `DispatchBatch` calls; `*math/rand.Rand` is **not** goroutine-safe.
- Propagate `NewService`'s error; `Validate()` needs `max_attempts >= 1`.
- The sweep is select → `RecoverStalled` → `Save` per row (E9c).

## D7 (unblocked)
- `Subject` → `uuid.Parse` → compare to `RecipientID`. Cross-user ⇒ **404 byte-identical to
  the genuinely-not-found case**, or the 404 is a 403 with extra steps and the enumeration
  oracle is back.
- Protected preference combos ⇒ problem+json **422**.
- **Pagination precedent is D4's keyset cursor** — measured on a 20k-row mailbox: the
  planner decomposes the row-value comparison into an index bound plus a cheap tiebreak
  filter, 4 buffer hits at depth. Use it; do not invent an `OFFSET` variant.
- **`MarkRead` does not restate `channel = 'inapp'`**, unlike `ListForRecipient` and
  `MarkAllRead` — a recipient can mark their own email/webhook row read and get success
  rather than `ErrNotFound`. Not a security issue; know it before writing the handler test.

## C1 (ready)
Carry **E3**'s approved scope verbatim: lead with the transport-level break — content-type
`text/plain` → `application/problem+json`; body → RFC 9457 object **with no `error` member
at all** (zero field-name overlap, so `body.error` clients break outright rather than
degrade); `WWW-Authenticate: Bearer realm="api"` now sent where **none was ever sent**.
Keep "key off `status`, never `detail`", augmented not replaced. **Do not repeat**
§8.1/§8.3's "collapses several unstable shapes into one". **E4:** scope the claim to routes
behind `AuthRequired` and **state the `/auth/mfa/verify` carve-out positively**. **E5:**
document `request_id` as an RFC 9457 extension member. Every claim checked against **merged
code**.

---

# BACKLOG

- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings (blame `6eed3aa`); the
  only lint findings repo-wide. Also a **dead duplicate** of `tools/schemacheck/schema_test.go`.
- **BL3** `jwks.go:7` imports `infrastructure/token` in production, contradicting R1;
  unenforced. Related to E11.
- **BL4** RSA keypair parsed twice at boot; deliberate (`container.go:83-90`).
- **BL5** `identity.New` exposes only `(*module.Module, error)` — see E15.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern.
- **BL12** Two stale worktrees under `.claude/worktrees` with stale config; exclude them for
  **config files**, not just code.
- **BL13** `/auth/mfa/verify` OAuth2-shaped 401 — accepted permanent divergence.
- **BL16** golangci-lint cache trap — **and see GATE-INTEGRITY, which is worse.**
- **BL17** `-require-profile=false` exits 0 on an empty profile; `tolerance = 0.05` softens
  **floors** as well as the ratchet; `tools/...` is exempt so the gate does not gate itself.
- **BL18** `cmd/api/protected_routes_test.go:157` asserts only `rec.Code != 401`.
- **BL19** Baselines lag actuals: `modules/identity/transport` 15.8 vs 18.9,
  `internal/httpx` 93.9 vs 94.0, `modules/user/transport` (100%) absent entirely, and no
  `modules/*/transport` floor pattern exists at all. `make coverage-update` settles it.
- **BL24** The notification module is **at-least-once** by design — a connection dropped
  after DATA re-sends, so a user can get two password-changed mails.
- **BL25** Panic visibility: a panicking sender is recovered into an ordinary retryable
  failure, so `DispatchBatch` returns nil and D6 gets **no error to log** — only
  `LastError`, no stack, 512-byte cap. Fix with E12's observer.
- **BL26** `internal/testsupport` at 8.5%, exempt, now load-bearing tree-wide.
- **BL27** Do **not** add `-trimpath` to `go test` — B1's import test uses `runtime.Caller`.
- **BL28** `dependency-rules.md`'s `## Enforcement` snippet is stale (v1 syntax, omits both
  transport rules) — a second, contradicting statement of the rules in the same file.
- **BL29 / BL30** Verification hygiene: a reviewer's mutation silently failed to apply and
  went green; a `python` board edit failed while the surrounding git pipeline reported
  success. **Verify the artifact, not the exit code around it.**
- **BL31** `TestModelsMatchMigratedSchema` **skips locally** even with Postgres live; CI has
  an explicit guard that fails the build if it skips, so only local runs are blind.
- **BL32** Two tools, one string, two meanings: `internal/authn` means "and everything
  below" in `arch_test.go` but "exactly this" in `policy.json`. Both right for their job; a
  maintainer reading one and editing the other will get it wrong. Cheapest fix is notation —
  spell the archtest areas with an explicit `/...` suffix.
- **BL33** `schemacheck` compares **column names only**, in both directions — it structurally
  cannot catch a tag whose *semantics* diverge from the column (the `default:true` class D4
  found). Covering it needs `SQLDefault` vs `column_default`.
- **BL34** Shared-`TEST_DATABASE_URL` leaks state between test binaries; the full suite in
  parallel produces spurious `goose_db_version`/`pg_type` races. `-p 1` avoids it. Same root
  fragility as E8's enabling condition.
- **BL35** Arch failures are reported twice per violation (production package plus its
  `[p.test]` variant), so CI reads as 2N violations for N problems.

---

# DEVIATIONS ACCEPTED

- **A1 (4), A2 (1), A2 copy-on-write, A3 (2), A4 (2)** — adjudicated; A4's `Scopes`-nil and
  empty-`sub` rejection both confirmed correct, the latter a strict tightening.
- **T1 (1)** — **refused** my instruction to baseline five unmeasurable packages. Reviewer:
  *"I would have blocked the PR had it complied."* The instruction was mine and it was wrong.
- **D1 (2)** — `persistence/models/` path; disposable Postgres on a non-default port. Plus
  the `nullzero` payload fix added on review before merge.
- **D2 (2)** — `BackoffWithJitter` additive to the plan's exact signature; `uuid` per the
  **enforced** rule over the plan's prose.
- **D3 (8)** — all ACCEPT except the template shape, reopened as E14.
- **E9a (1)** — `Transition` unexported; with it exported, `sending→pending` *is* the bare
  arrow the ruling forbade.
- **B1 (4)** — two depguard globs instead of the one I specified (**mine was dead** — it
  reported `0 issues.` on a plainly violating file).
- **B2 (3)** — `AuthMiddleware` name kept; tests added not simplified; **ownership check not
  delivered** (E16).
- **B3 (5)** — five invalid minters; **≥2** cases required; parsed media type.
- **D4 (4)** — `persistence/repository/` path; **`MarkRead` uses `coalesce`** rather than
  `read_at IS NULL`, because **my instruction contradicted the merged port doc** (which makes
  already-read a *no-op*, not an error) — reviewer confirmed the instruction was wrong and the
  shipped form preserves everything D7's 404 needs; the **`Enabled` model fix** (a
  `default:true` tag made `false` unrepresentable — every "turn this off" stored "on";
  reviewer audited all 20 `default:` tags and confirmed it was the **unique** such case);
  `ClaimBatch` result order not asserted (`UPDATE … RETURNING` has no defined order).
- **B4 (3)** — rule added to the existing `arch_test.go`; **`cmd/**` deliberately absent** from
  the permitted list, so naming `authn.Middleware` in `container.go` will fail `make arch`
  (correct, but it will surprise someone); §7 left uncorrected.
