# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner                | State | PR / commit | Review | Updated |
| --- | -------------------- | ----- | ----------- | ------ | ------- |
| A1  | test-guardian        | **merged** | #16 (`9e50896`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A2  | domain-engineer      | **merged** | #16 (`6288dcd`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A3  | transport-engineer   | **merged** | #17 (`4a969f2`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A4  | transport-engineer   | **merged** | #20 (`5a0b56e`) | APPROVE-WITH-NOTES | 2026-08-08 |
| T1  | test-guardian        | **merged** | #18 (`8b1b185`) | APPROVE-WITH-NOTES | 2026-08-08 |
| D1  | persistence-engineer | **merged** | #22 (`fbc1639`, `cfb3f58`) | APPROVE-WITH-NOTES | 2026-08-09 |
| D2  | domain-engineer      | **merged** | #23 (`049f4b0`, `468dbc0`) | APPROVE-WITH-NOTES | 2026-08-09 |
| D3  | domain-engineer      | **merged** | #23 (`13ab921`) | APPROVE-WITH-NOTES | 2026-08-09 |
| E9a | domain-engineer      | approved — **PR #25 open** | `47d1569` | APPROVE-WITH-NOTES | 2026-08-09 |
| B1  | test-guardian        | approved — **PR #26 open** | `ea0d30f` | APPROVE-WITH-NOTES | 2026-08-09 |
| B2  | transport-engineer   | **PARTIAL** — approved, PR #26 | `739c6df` | APPROVE-WITH-NOTES | 2026-08-09 |
| B3  | test-guardian        | approved — PR #26 | `1a72344` | APPROVE-WITH-NOTES | 2026-08-09 |
| B2a | transport-engineer   | **BLOCKED — E16** (carry-forward of B2's unmet clause) | - | - | 2026-08-09 |
| B4  | test-guardian        | ready — dispatch next | - | - | 2026-08-09 |
| C1  | docs-scribe          | pending (needs Phase B) | - | - | - |
| D4  | persistence-engineer | ready | - | - | 2026-08-09 |
| D5  | platform-engineer    | **BLOCKED — E11, E13, E14** | - | - | 2026-08-09 |
| D6  | platform-engineer    | **BLOCKED — E9b, E9c, E12** | - | - | 2026-08-09 |
| D7  | transport-engineer   | **UNBLOCKED** — needs D6 + Phase B only | - | - | 2026-08-09 |
| D8  | platform-engineer    | **BLOCKED — E15, E11** | - | - | 2026-08-09 |
| D9a/D9b | domain / platform | pending | - | - | - |
| D10 | platform-engineer    | pending — owns `policy.json` cleanups | - | - | 2026-08-09 |

**Open PRs — I cannot merge** (`gh pr merge` and pushes to `main` are blocked by the
sandbox classifier; branch pushes and PR creation work):

| PR | Contents | Gate |
| --- | --- | --- |
| **#25** | E9a — `sending→pending` stall recovery | APPROVE-WITH-NOTES |
| **#26** | Phase B: B1 + B2 (partial) + B3 | all three APPROVE-WITH-NOTES |

## Merge log

| PR | Merged as | Contents |
| --- | --- | --- |
| #15 | `d63200b` | package rename — the Phase A base |
| #16 | `f594ebc` | A1, A2, + the human's docs/roster commit |
| #17 | `d74ec67` | A3 — `httpx.Unauthorized` |
| #18 | `0c0a1cf` | T1 — `tools/coverage` |
| #19 / #21 / #24 | `f6e93ff` / `19008c6` / `e6d0d93` | board |
| #20 | `a475151` | A4 — identity implements the contract |
| #22 | `21c94bb` | D1 — notification migration + bun models |
| #23 | `68402ec` | D2 + D3 — notification domain + application |

---

# ESCALATIONS — OPEN (12)

Ordered by severity, not by number.

## E16 — ⚠ CONFIRMED IDOR IN MERGED PRODUCTION CODE
**Raised by:** transport-engineer (B2) as a stop condition; **confirmed independently by
arch-reviewer** at the schema level. **Predates this effort** — `git blame` puts the
handler bodies at `68f75e22` / `14febc19`, 2026-04-02/03.

**The exposure.** Any caller holding **any valid access token for any account** can,
against **any other user's row**:

| Verb | Route | Effect |
| --- | --- | --- |
| `GET` | `/api/v1/users/{id}` | read `id`, `name`, `email` (`transport/user.go:91-112`) |
| `PUT` | `/api/v1/users/{id}` | overwrite `name`, `email` (`:127-157`) |
| `DELETE` | `/api/v1/users/{id}` | delete the record (`:169-189`) |
| `POST` | `/api/v1/users` | unrestricted creation behind mere authentication (`:56-78`) |

Ids are **`BIGSERIAL`** — sequential and trivially enumerable; `/users/1,2,3…` walks the
whole table. `internal/metrics` templatises the path, so enumeration does not even
surface as distinct metric series.

**Nothing mitigates it — every layer checked:** route mounting adds no wrapper
(`internal/httpx/routes.go:61-65`); the middleware is **authentication only**
(`cmd/api/container.go:103`), and `grep -rn "RequireRole|RequirePermission|Authorize"`
across `internal/httpx`, `internal/middleware`, `modules/user`, `cmd/api` returns
**zero matches — no authorization layer exists anywhere in this repository**; transport
reads the path id and calls straight through; the application service takes
`(ctx, id int64)` with **no caller argument of any kind**, so there is no seam a check
could even attach to; the repository has no owner predicate. Roles/permissions *are* in
the JWT (`jwt.go:188-189`) but the middleware discards them by design
(`middleware.go:58-67`).

**Root cause is not a missing `if`.** `auth_users.id` is `uuid` (`migrations/00003:8`);
`users.id` is `BIGSERIAL` (`migrations/00001:4`);
`grep -rn "auth_user_id|AuthUserID|REFERENCES users"` → **zero matches**. The only FKs in
the tree point at `auth_users`. The Principal carries no email (`Extra` nil by design).
**There is no value to compare**, so remediation requires an ownership-model decision
before any code change.

**B2 refused three shortcuts, all correctly** (reviewer upheld each):
`strconv.ParseInt(Subject)` — a uuid never parses, so all three id routes 404
permanently: "an outage that lints clean"; `Subject == chi.URLParam("id")` — always
false, same outage, and *reads* like a real check to the next maintainer;
`Extra["email"]` — nil, needs an identity-side change, and email is a **mutable natural
key** with no enforced 1:1 correspondence. It also declined a bare `MustFromContext`,
which would have been dead code making the diff *look* complete.

**Decision needed — see E16-ARCH.** Track separately from Phase B: older and larger than
this effort.

## E16-ARCH — the id question: ONE root problem, and two of its three instances dissolve
**arch-reviewer's architectural read**, and it materially changes the plan.

The repo has **two user identifiers with no defined relationship**. That single fact
surfaces at three call sites — but only one is genuinely blocked:

- **Notification already keys on the identity uuid, natively.**
  `migrations/20260808170025:4` `recipient_id uuid NOT NULL`; `application/dto.go:22`
  and `ports.go:41` `RecipientID uuid.UUID`; `notification_preferences.user_id` uuid.
  No mapping layer anywhere.
- **⇒ D7 IS NOT BLOCKED.** `authn.MustFromContext(r).Subject` parses directly to
  `uuid.UUID` and compares to `RecipientID`; the cross-user 404 falls out with no
  mapping. **I previously recorded D7 as blocked — that was wrong.**
- **⇒ D8/E15 needs no mapping either** — only an *address* for an identity uuid, i.e.
  `auth_users.email`, resolvable entirely within identity.
- **`modules/user` is the only module in the tree that does not key on the identity
  uuid.** It is the outlier, not the standard.

**Options, in the order they should be decided:**
1. **First decide the ownership model:** does `users` remain a distinct entity, or become
   a **profile keyed by the identity uuid**? Everything else follows. `auth_users`
   already carries `username`, `email`, `name`, `roles`, `permissions`; `users` carries
   `name`, `email`. The overlap is total, and the second copy of a person's email is
   exactly the staleness hazard E15 already rejected ("mails a stale address after an
   email change"). Merging or demoting `users` closes B2, D7, D8/E15 **and** the IDOR.
2. If `users` stays distinct → **standardise its PK on the identity uuid** (preferred;
   blast radius is `modules/user` + one migration, and it deletes the IDOR's root cause
   because `p.Subject == id` becomes a *real* check), **or** add `auth_user_id uuid
   UNIQUE` with a backfill and a documented nullability answer (tactical, buys time,
   leaves two keys per user permanently).
3. **Unblock D7 now** — independent of 1 and 2.
4. **Unblock D8/E15 now** via the identity-side resolver — independent of 1 and 2.
5. Close the IDOR under whichever answer 1 produces. **Only this must wait.**

Reviewer: this is the **oldest** unresolved question in the effort — it predates the
authn SPI work entirely — and its cost compounds, because every task touching a user
identity now pays a stop-and-escalate tax.

## E10 — second-factor bypass shape in merged identity code
`auth_models.go` maps three **nullable** columns to plain `string`, collapsing NULL to
`""`. **`mfa_secret` is the serious one:** a row with `mfa_enabled = true AND mfa_secret
IS NULL` yields `totp.Validate(code, "")` — an empty secret base32-decodes fine and
produces a **publicly computable** valid TOTP. It fails **open**. `password_hash` fails
*closed* (bcrypt rejects `""`) but read-then-save silently rewrites NULL to `''`, and the
fast rejection is an enumeration timing signal.
Not reachable through current app paths — needs a partly-failed setup or a manual DB
edit. **`tools/schemacheck` cannot catch this class**: it compares column *names* only,
never types or nullability.
**Remediation, in order:** `NOT NULL` in a new migration after a data check;
independently guard `ValidateCode` on an empty secret (cheap, fails closed, no schema
dependency); extend schemacheck to assert `is_nullable == pointer-ness`, turning the
class into a CI failure. **Outside every task's file list — needs a task.**

## E11 — `infrastructure` may not import `application`, but D5 and D8 must ⚠ blocks D5
R1 (`dependency-rules.md`, guardrail 5, blueprint §3) forbids `infrastructure →
application`. But blueprint §5 puts `ChannelSender`, `RenderedMessage`,
`TemplateRenderer` and `ErrNonRetryable` in `application/ports.go` — so every sender must
import `application` **just to spell its own method signature**.
**Neither gate catches it:** archtest only checks application→infrastructure; depguard
has no infrastructure rule at all. D5 and D8 would ship green while silently breaking the
written table.
**Reviewer recommends amending the rule, not moving the code** — an adapter implementing
a port declared by the inner ring is textbook hexagonal; control flow still points
inward, and R1's real intent is that infrastructure must not *invoke use cases*. Amend R1
to `infrastructure → domain, own application's ports, bun, pgx, external clients`, and
add an archtest rule permitting `modules/X/infrastructure → modules/X/application` while
forbidding another module's application. **Predates D3; must land before D5.**

## E12 — `DispatchBatch (int, error)` cannot support D10's metrics or audit
D10 needs `notification_{sent,failed,dead}_total{channel}`, a latency histogram, and —
per blueprint §6 — an `internal/audit` event on **security-category dead-letter**, a
security requirement. The dispatcher only sees `(int, error)`: it cannot know sent vs
failed vs dead, on which channel, or that a security row died. All of it is computed
inside `settle` and discarded.
**Settle now or pay at D10** as a forced change to a contract merged two PRs earlier,
plus a D6 rewrite. Reviewer prefers an **observer port** over a stats struct because it
also fixes the panic-visibility gap (BL25) and gives per-row channel/category:
`DispatchObserver.Settled(n, outcome, err)`, optional (nil ⇒ no-op, like `jitter`),
called once per row.

## E13 — no task ships an in-app sender; every in-app row would dead-letter ⚠
`inapp` and `webhook` appear **nowhere** in the plan's task list. Blueprint §3 lists
`sender/inapp.go` and `sender/webhook.go`; D5 ships only `log.go`, D8 only
`email_smtp.go`. With D3's correct fail-closed behaviour, an unregistered channel
dead-letters immediately — so at the D6 merge gate **every in-app notification goes
straight to `dead`**, the exact channel D7's read model and route table exist to serve.
Fails visibly: rows still list, each `Status: dead`, `notification_dead_total{inapp}`
alarms permanently, security rows fire the dead-letter audit.
**Add `sender/inapp.go` to D5's file list** and record **webhook as an explicit v1
non-goal** in blueprint §13, or webhook rows dead-letter too.

## E14 — template shape: the two documents are on orthogonal axes, BOTH incomplete
Plan D5 says `<key>.<channel>.tmpl` (**channel** axis); blueprint §3 says
`*.subject.tmpl` / `*.html.tmpl` / `*.text.tmpl` (**MIME-part** axis). You need both. D3
shipped a hybrid (`RenderedMessage{Subject, Body}`) that papers over the hole.
**Single-body is not correct for v1** — `text/html` with no `text/plain` alternative is a
measurable deliverability penalty, and the mail it costs you is the security mail.
Proposed: `security.password_changed.email.{subject,html,text}.tmpl`,
`…inapp.body.tmpl`; and D8 needs an **added field**, not a second render call
(`multipart/alternative` needs both parts in one `Send`): add `HTMLBody string` to
`RenderedMessage`, optional, empty ⇒ text only, leaving log/in-app/webhook untouched.
**Do not reopen D3** — amend D5's file list to include `application/ports.go`.

## E15 — nobody owns address resolution ⚠ blocks D8
Nothing turns `RecipientID` into an email address. D3 already committed to an answer in
prose (`ports.go:35-38`: resolution is the sender's job) and the reviewer recommends
**ratifying** it. Proposed task **D8a, gating D8**: declare `AddressResolver` in
`infrastructure/sender/` (`Resolve(ctx, recipientID) (string, error)`) with a "no such
user" sentinel mapping to `ErrNonRetryable` while DB failure stays retryable; adapter in
`cmd/api` over identity's `UserRepository.FindByID`.
**Per E16-ARCH this needs no `users` involvement** — the recipient is the identity uuid
and the address is `auth_users.email`.
**Blocker inside the blocker:** `identity.New` returns `(*module.Module, error)` and
exposes nothing resolvable. Widening it mirrors D6's own
`New(...) (*module.Module, Service, error)` shape.
**Rejected, recorded:** snapshotting the address at enqueue — copies PII into `payload`
and mails a **stale address after an email change**, i.e. "your password was changed"
goes to the address the attacker just replaced.

## E9b — D6's sweep has no column to run on; the cheap window has CLOSED
"Rows in `sending` older than a threshold" has no predicate. `next_attempt_at` holds its
**pre-claim** value, so a row enqueued 10 minutes ago and claimed 2 seconds ago already
looks stalled — resetting it while the original worker is still sending is a **double
delivery**. Verified: `notifications` has only `next_attempt_at`, `created_at`, `sent_at`.
**D1 merged in #22 before this was decided, so it now costs a NEW migration**, not a
one-line edit. My error — I flagged the time-pressure and did not hold the PR.
Options: (a) new migration adding `claimed_at`; (b) specify `ClaimBatch` to set
`next_attempt_at = now` on claim (no schema change, but the semantics must be written
into the `ClaimBatch` doc comment, which currently does not say so).
**E9a raised the cost of getting this wrong:** a false positive now *also* burns an
attempt and can dead-letter a healthy in-flight row. The predicate must be exact.

## E9c — the domain has no port to FIND stalled rows (NEW)
**Raised by arch-reviewer during E9a's gate.** E9a completed the *entity* side, but
`NotificationRepository` (`domain/repository.go:108-155`) declares `Enqueue`,
`ClaimBatch`, `Save`, `ListForRecipient`, `MarkRead`, `MarkAllRead` — **no method returns
rows in `sending`**, and `ClaimBatch` selects `status = 'pending'` by contract. So the
sweep has no source of entities.
Needs roughly `ClaimStalled(ctx, n int, stalledBefore time.Time) ([]*Notification, error)`
— which lives in `repository.go`, a **D2 file**, so under guardrail 9 neither D4 nor D6
may add it.
**Blocked behind E9b**, because the second parameter's meaning depends on the option
chosen: under (a) `claimed_at < stalledBefore`, under (b) `next_attempt_at <
stalledBefore` plus a `ClaimBatch` contract amendment. Writing it first bakes in the
wrong predicate. Sequence: settle E9b → small D2-follow-on adds the port → D4 implements
→ D6 consumes.
**Also correct the E9a record:** "the sweep still runs as one set-based UPDATE" is **no
longer achievable**. A set-based UPDATE that spends an attempt and dead-letters on
exhaustion would be a second, untested copy of the state machine. D6 must do
select-stalled → `RecoverStalled` → `Save` per row.

## E17 — the conformance suite guards `detail` only; the likeliest replacement leaks elsewhere (NEW)
**Raised by arch-reviewer during B3's gate.** B3 is real — the reviewer broke each of its
properties in production code and watched it fail, and it catches a principal-drop
regression **no other test in this repository catches**. But it constrains one member of
one artifact.
The reviewer built a textbook **RFC 6750** middleware that leaks the reason into
`WWW-Authenticate` as `error_description="the access token expired at …"`, into the
problem `type` URI, and into `title`, with **no `detail` member at all** — and ran it
through the suite: **zero findings. It passes.** That is *the default output shape of
most OAuth/OIDC middleware*, i.e. exactly the replacement B3 exists to constrain.
Second, smaller: a middleware emitting `{}` for every 401 is **invariant by vacuity** —
`checkOneRejection` returns `("", true)` for valid JSON with no `detail` key.
**Fix (one function, no new exports):** extend `checkDetailIsInvariant` to compare the
whole rejection response — challenge header value, `type`, `title`, `detail` — across
cases, and treat an absent `detail` as a finding. Owner: B4, or its own task.
**Not a reason to hold Phase B** — the suite is strictly better than what preceded it,
which was nothing.

## E7 — `auditlog` has no tests and trips a pre-existing 60% floor
`modules/identity/infrastructure/auditlog`: 8 functions, no test file, matches the
**pre-existing** `modules/*/infrastructure/...` floor. Invisible today only because CI
never reaches the coverage step (E8); an immediate failure the instant E8 is fixed.
Confirmed the **sole** such package. Fix is a test or a reasoned `exempt` entry — **not a
lower floor**.

## E8 — CI red — ⚠ SEVERITY CORRECTED, materially better than first reported
**Two earlier reports were wrong and are superseded.**
- **`00007` IS reversible.** Proven at review by `DownTo(6)` restoring every prior index
  and constraint, and by a full `DownTo(0)` + `Up` cycle. My earlier "migration appears
  not to be reversible" was wrong; I had propagated it into PR #26's body and corrected
  it there.
- **`TestCaseInsensitiveIdentifiers_RefusesCollidingData` passes today.** The T1-era
  report of it failing is not reproducible at `e6d0d93`. Superseded.
- **The actual failure is a stale test.** `goose.Down()` reverts only the **newest**
  migration, and **D1 made notifications the newest** — so
  `TestCaseInsensitiveIdentifiers_IsReversible` now rolls back D1's migration and then
  asserts 00007's index is gone. One-line fix at `internal/db/migrations_test.go:56`:
  `goose.Down(...)` → `goose.DownTo(sqlDB, dir, preCaseInsensitiveVersion)` (that
  constant already exists at line 26).
- **CI's migrations job is GREEN** — it runs a full up → reset → up, independently
  proving rollback safety. Only the `go test -race` job is red, on this one unit test.
- **Class warning:** *any* `goose.Down()`-based test breaks the moment a newer migration
  lands. Worth grepping for other callers before more migrations land.
**Still open, and still needing owners:**
1. The stale test above.
2. **The CI `golangci-lint` job cannot start** (built with go1.24, targets 1.25.0) — it
   has produced **no findings at all**. Local `make lint` is the repo's only lint
   coverage, so every green reported in this effort rests on one machine.
3. `govulncheck` fails on stdlib advisories. Likely a toolchain bump.

---

# ESCALATIONS — RESOLVED

**E1** — `tools/coverage` never committed; T1 authorised and merged (#18). Root cause was
the bare `coverage/` pattern (arch-reviewer right; my "correction" wrong — I read the
working file, not the committed blob). **Standing rule:** cite `git show <ref>:<path>`,
never the working file.
**E2** — SPI rationale holds as stated; A4 unblocked.
**E3** — corrected changelog scope approved (see C1 below).
**E4** — uniformity claim narrowed to the middleware-protected surface;
`/auth/mfa/verify` is an accepted permanent divergence (BL13). Honoured by A4.
**E5** — `request_id` accepted in the canonical body; §8.2's example is illustrative.
**E6** — `docs/plans/` tracked and on `main`.
**E9a** — **add the `sending→pending` arrow** (human ruling), exposed only as
`RecoverStalled(now, retryAfter, maxAttempts)` so recovery costs an attempt. Shipped in
PR #25; `Transition` unexported to make the ruling mechanical rather than advisory.
**Depguard/authntest question** — settled at B3's gate: under `list-mode: lax` the
`internal/authn` allow prefix beats the `internal` deny, so `internal/authn/authntest`
**is** permitted in transport tests. B1's reading was right; the reviewer withdrew its
contrary note from B2. No `.golangci.yml` change needed for B3.

---

# DOC AMENDMENTS I CANNOT MAKE (I do not edit the plan or companion docs)

- **Phase B's title overclaims.** "consumers + conformance guarantee" — B3 delivers the
  guarantee, but B2's consumer clause hit a confirmed stop condition (E16). Reviewer:
  *"six months from now the title is the only thing anyone reads."* Amend to name what
  shipped, and record B2's unmet clause as an explicit carry-forward (B2a).
- **Plan B2/D7 field name.** Plan says `Config.Auth`; shipped is `Config.AuthMiddleware`
  (renaming would force an edit to single-writer `cmd/api`). D7's text at plan line 289
  also says `Config.Auth` — align before D7 is written.
- **Blueprint §4.1** backoff formula reads `min(base*2^n, cap) + jitter` (jitter *on top
  of* the cap); shipped code keeps it **inside** (`[d/2, d)`). If the doc stands, D10's
  docs pass will "reconcile" the code and reintroduce over-cap waits.
- **Blueprint §6** attributes jitter to the dispatcher; §4.1 puts it in the domain. The
  blueprint contradicts itself. D6 still owns and injects the source.
- **Plan D2/D3** "stdlib + `internal/platform`" should read "+ `github.com/google/uuid`".
- **Plan D6** config `Validate()` omits `max_attempts >= 1`. Without it a zeroed field
  dead-letters every notification on its first transient failure.
- **Plan D4** file list says flat `persistence/{notification_repo,preference_repo}.go`;
  must be `persistence/repository/`.
- **Plan B1** file list omits `.golangci.yml` and `dependency-rules.md`, both shipped.
- **Plan B4** coverage clause — T1 already landed the `internal/authn` floor+baseline.
- **`authn-spi-impact-analysis.md` §7** is self-contradictory and stale in two places:
  line 202 forbids `internal/testsupport` importing `internal/authn` while line 205
  *requires* `FakeAuth` to return `authn.Middleware` — impossible; and line 206's
  `mintExpired` single minter cannot drive the three rejection paths the same cell
  demands. **Plan B3/B4's wording is authoritative in both cases.**
- **`authn-spi-impact-analysis.md` §8.1** "Expected finding: the current rejections are
  already inconsistent with each other" is known false (E3).
- **ADR 005** line 60 still shows `transport → application, chi`, understating the layer
  rule after A4.

---

# DISPATCH ADJUSTMENTS — banked

## B4 (ready now)
- Owns the **archtest import fence** (only `modules/*`, `internal/httpx`,
  `internal/testsupport`, `internal/authn/authntest` may import `internal/authn`) and the
  **deliberate-violation proof** — a blocking criterion in its own right.
- **`internal/authn/authntest` matches NO policy pattern** (the `internal/authn` floor is
  non-recursive; `exempt` lists `testsupport` but not `authntest`), so it lands
  **unfloored at 100%**. B4 decides exempt vs floor. Reviewer: *"a package whose entire
  purpose is to be trustworthy has no coverage gate at all — precisely where 'distrust
  green' matters most."* If B4's scope does not reach `policy.json`, say so and it
  becomes its own task.
- **Re-verify** `internal/authn: 100` after B2/B3.
- Candidate owner for **E17**'s one-function fix.
- Do **not** re-add the `internal/authn` floor/baseline — T1 landed it.

## D4 (ready now)
1. **Path deviation mandatory**, mirroring D1: repositories in `persistence/repository/`.
   `policy.json` lists `modules/*/infrastructure/persistence/repository` under
   `requires_database`; a flat layout draws the 60% floor with tests that skip.
2. **`ON CONFLICT (idempotency_key) DO NOTHING` does not compile** against a partial
   unique index. Required form:
   `ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`
   (the `WHERE` goes **before** `DO`). Verified: `INSERT 0 0` on duplicate,
   `INSERT 0 1` for a NULL key.
3. **`ClaimBatch` must bind the caller's `now`, not SQL `now()`** — otherwise D3's
   injected `Clock` is bypassed and its fixed-clock tests describe behaviour the
   repository does not have.
4. **Register both tables in `tools/schemacheck`** (`schema_test.go`, `cases` slice). D1
   could not — its acceptance forbade another package importing the models; that clause
   expires here. Until then CI's drift guard does not cover this module, and the same
   test is CI's tripwire that a database was reachable at all.
5. **`""` ↔ NULL bridging** for `IdempotencyKey` **and** `LastError` (domain `string`,
   model `*string`, deliberate on both sides). Round-trip test: two rows with
   `IdempotencyKey: ""` must both persist and read back as `""`.
6. **`MarkRead` negative case** — `MarkRead(ctx, otherUsersID, id, now)` returns
   `ErrNotFound` and leaves `read_at` NULL. D7's cross-user 404 rests on it.
7. **Concurrency test:** two goroutines on **separate connections**
   (`SetMaxOpenConns >= 2` — assert it, or it passes for the wrong reason via silent
   serialisation). No ordering assertion: `ORDER BY next_attempt_at` has no tiebreaker.
8. Claim plan verified on 50k rows: `Index Scan using idx_notifications_claim`, **no
   Sort**, `Limit` above `LockRows`.

## D6
- **Pass a NON-CANCELLED drain context** — `settle` calls `Save(ctx, …)`; a cancelled ctx
  fails the save and leaves every in-flight row in `sending`.
- **Shutdown burns retry budget** — cancelled ctx reaching `Send` fails as
  `context.Canceled`, retryable by default. Special-case it or drain fresh.
- **Jitter source must be concurrency-safe.** One long-lived field on `Service`, and D6
  runs `workers` concurrent `DispatchBatch` calls. `*math/rand.Rand` is **not**
  goroutine-safe. Inject `math/rand/v2`'s race-free global or a mutex-guarded source.
- **Propagate the error from `NewService`** — it fails closed on nil deps and duplicate or
  unknown sender channels.
- Config `Validate()` must include `max_attempts >= 1`.
- The sweep is **select → `RecoverStalled` → `Save` per row**, not set-based (E9c).

## D7 (unblocked)
- `MustFromContext(r).Subject` → `uuid.Parse` → compare to `RecipientID`. No mapping.
- Cross-user id ⇒ **404, not 403**, and the body must be **byte-identical** to the
  genuinely-not-found case, or the 404 is a 403 with extra steps and the enumeration
  oracle is back.
- Preference PUT rejecting protected combos ⇒ problem+json **422**.
- Cursor pagination: **there is no existing repo pattern** — D2's keyset `Cursor`
  (`(created_at, id)`, `DefaultListLimit 20`, `MaxListLimit 100`) is the precedent.

## C1
- Carry **E3**'s approved scope verbatim: lead with the transport-level break —
  content-type `text/plain` → `application/problem+json`; body → RFC 9457 object **with no
  `error` member at all** (zero field-name overlap, so `body.error` clients break outright
  rather than degrade); `WWW-Authenticate: Bearer realm="api"` now sent where **none was
  ever sent**. Keep "key off `status`, never `detail`", augmented not replaced. **Do not
  repeat** §8.1/§8.3's "collapses several unstable shapes into one".
- **E4:** scope the claim to routes behind `AuthRequired`; **state the `/auth/mfa/verify`
  carve-out positively** — silent omission is the failure mode.
- **E5:** document `request_id` as an RFC 9457 extension member.
- Every claim checked against **merged code**.

---

# BACKLOG — pre-existing, do NOT spawn unplanned work

- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings (blame `6eed3aa`);
  the only lint findings repo-wide, confirmed at every task. Also a **dead duplicate**:
  it declares a second `TestModelsMatchMigratedSchema` superseded by
  `tools/schemacheck/schema_test.go`. Deleting it removes both findings.
- **BL3** `modules/identity/transport/jwks.go:7` imports `infrastructure/token` in
  production, contradicting R1; A4's depguard rule does not catch it (denies
  `internal/**`, not sibling layers). Related to E11.
- **BL4** RSA keypair parsed twice at boot; deliberate (`container.go:83-90`).
- **BL5** `identity.New` exposes only `(*module.Module, error)` — see E15.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern; `.zip` ignored by nothing.
- **BL12** Two stale worktrees under `.claude/worktrees` (`phase-0-hygiene`,
  `phase-3-consolidate`) with stale `.gitignore`/`CLAUDE.md`/tooling — source of one wrong
  root-cause diagnosis. Agents must exclude them for **config files**, not just code.
- **BL13** `/auth/mfa/verify` OAuth2-shaped 401 — **accepted permanent divergence (E4)**.
- **BL16** **`golangci-lint` cache trap** — serves results keyed to deleted directories and
  prints a false `0 issues.`. **Always `golangci-lint cache clean` first.**
- **BL17** coverage tool notes: `-require-profile=false` exits 0 on an empty profile (must
  never appear in a Makefile or workflow); `tolerance = 0.05` softens **floors** as well as
  the ratchet; **floors are enforced only for packages present in the profile** — exactly
  what hid E7; `tools/...` is exempt so the gate does not gate itself.
- **BL18** `cmd/api/protected_routes_test.go:157` asserts only `rec.Code != 401`, and
  `Recovery` converts a panic to 500 — so a `MustFromContext` regression **passes** it. B3
  closes the underlying hole for identity's middleware; this assertion is still weak.
- **BL19** Coverage baselines lag actuals in three places, leaving unratcheted slack:
  `modules/identity/transport` 15.8 vs **18.9**, `internal/httpx` 93.9 vs 94.0, and
  **`modules/user/transport` (new, 100%) has no baseline entry and no
  `modules/*/transport` floor pattern exists at all**. Run `make coverage-update` at the
  end of Phase B; D10 owns the file.
- **BL24** The notification module is **at-least-once** by design: default-retryable means
  an SMTP DATA accepted but a connection dropped before the 250 re-sends, so a user can get
  two password-changed mails. Correct trade; D8/D10 need it on record.
- **BL25** Panic visibility gap: a panicking sender is recovered into an ordinary retryable
  failure, so `DispatchBatch` returns nil and D6 gets **no error to log** — only
  `LastError`, no stack, 512-byte cap. Fix belongs with E12's observer hook.
- **BL26** `internal/testsupport` sits at 8.5% coverage and is `exempt`, but it is now
  load-bearing for transport tests across the tree.
- **BL27** Do **not** add `-trimpath` to `go test` in CI — `testsupport.RepoRoot` resolves
  via `runtime.Caller`, and B1's import-enforcement test depends on it. Currently safe:
  `-trimpath` appears only on the release build (`ci.yml:342`).
- **BL28** `dependency-rules.md`'s `## Enforcement` YAML snippet (~lines 149-183) is stale:
  v1 syntax, omits both transport depguard rules, and gives `platform-independence` a
  different glob set. A second, contradicting statement of the rules in the same file.
- **BL29** Mutation-testing hygiene: a reviewer's patch silently failed to apply and the
  test went green, briefly looking like a real gap. **Print the patched line, not just the
  test result.**

---

# DEVIATIONS ACCEPTED

- **A1 (4), A2 (1), A2 copy-on-write, A3 (2 vs §8.2 — pre-authorised by E5), A4 (2)** — all
  adjudicated; see PR history. A4's `Scopes`-nil and empty-`sub` rejection were both
  confirmed correct, the latter a strict tightening.
- **T1 (1)** — **refused** my instruction to baseline five unmeasurable packages. Reviewer:
  *"I would have blocked the PR had it complied."* The instruction was mine and it was wrong.
- **D1 (2)** — models at `persistence/models/` (triple-supported); disposable Postgres on a
  non-default port. Plus the `nullzero` payload fix added on review before merge.
- **D2 (2)** — `BackoffWithJitter` additive to the plan's exact `Backoff` signature; `uuid`
  import per the **enforced** rule over the plan's prose.
- **D3 (8)** — all ACCEPT except the template shape, reopened as E14.
- **E9a (1)** — `Transition` unexported. Zero consumers; makes the human's ruling mechanical
  rather than advisory. Reviewer endorsed: with it exported, `sending→pending` *is* the bare
  arrow the ruling forbade.
- **B1 (4)** — two depguard globs instead of the one I specified (**mine was dead** — it
  reported `0 issues.` on a plainly violating file); `dependency-rules.md` edited to keep the
  doc true; an import-enforcement test beyond the task's letter; one vacuous alias test
  dropped on staticcheck's correct advice.
- **B2 (3)** — field kept as `AuthMiddleware` (renaming would force an edit to single-writer
  `cmd/api` for zero enforcement gain); tests **added** rather than simplified (the package
  had none); **ownership check not delivered** — confirmed stop condition, carried to B2a.
- **B3 (5)** — five invalid minters instead of four (reuses the shared `rejectionCases`
  table); suite requires **≥2** invalid cases (invariance over one is a lie); content-type
  compared as a parsed media type; `suite_test.go` added (the acceptance criterion cannot be
  met without it). The "`Co-Authored-By` is unusual here" concern was **false** — 52 of 126
  commits carry it.
