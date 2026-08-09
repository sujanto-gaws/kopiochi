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
| D2  | domain-engineer      | **approved — in PR #23** | `049f4b0`, `468dbc0` | APPROVE-WITH-NOTES | 2026-08-09 |
| D3  | domain-engineer      | **approved — in PR #23** | `13ab921` | APPROVE-WITH-NOTES | 2026-08-09 |
| B1  | test-guardian        | next up (Phase B) | - | - | 2026-08-09 |
| B2  | transport-engineer   | pending | - | - | - |
| B3  | test-guardian        | pending | - | - | - |
| B4  | test-guardian        | pending — **respecify** | - | - | - |
| C1  | docs-scribe          | pending | - | - | - |
| D4  | persistence-engineer | ready — needs #23 merged | - | - | 2026-08-09 |
| D5  | platform-engineer    | **BLOCKED — E11, E13, E14** | - | - | 2026-08-09 |
| D6  | platform-engineer    | **BLOCKED — E9a, E9b, E12** | - | - | 2026-08-09 |
| D7  | transport-engineer   | pending (needs D6 + Phase B) | - | - | - |
| D8  | platform-engineer    | **BLOCKED — E15, E11** | - | - | 2026-08-09 |
| D9a | domain-engineer      | pending | - | - | - |
| D9b | platform-engineer    | pending | - | - | - |
| D10 | platform-engineer    | pending — **see E12** | - | - | 2026-08-09 |

**Open PR: #23** (D2+D3). **I cannot merge** — `gh pr merge` and pushes to `main`
are blocked by the sandbox classifier. Branch pushes and PR creation work.

## Merge log

| PR | Merged as | Contents |
| --- | --- | --- |
| #15 | `d63200b` | package rename — the Phase A base |
| #16 | `f594ebc` | A1, A2, + the human's docs/roster commit |
| #17 | `d74ec67` | A3 — `httpx.Unauthorized` |
| #18 | `0c0a1cf` | T1 — `tools/coverage` |
| #19 / #21 | `f6e93ff` / `19008c6` | board |
| #20 | `a475151` | A4 — identity implements the contract |
| **#22** | **`21c94bb`** | **D1 — notification migration + bun models** |

**Phase A is complete.** Contract, canonical 401, identity migrated atomically,
coverage gate real. Reviewer's assessment: *coherent, not complete* — `modules/user`
is still outside the contract (B2) and there is a conformance *instance* rather than
a *guarantee* (B3).

## Escalations — OPEN (9)

### E9a — the transition table forbids the arrow D6 needs
D6's stuck-`sending` sweep is a `sending → pending` move; D2's mandated table forbids
it. D2 implemented the table as specified and reported the conflict.
**arch-reviewer recommends adding the arrow, not bypassing in SQL** — and its
argument overturned my earlier lean: a pure-SQL sweep **does not consume the retry
budget**, so a notification that reliably hangs the worker resets every 5 minutes
*forever*, reintroducing the infinite-retry failure mode `RecordFailure` exists to
prevent. Expose it only as `RecoverStalled(now, retryAfter, maxAttempts)` — parallel
to `RecordFailure`, incrementing `Attempts` and dead-lettering on exhaustion — so no
worker can legally abandon a send for free. `sent→pending` and `dead→sending` stay
forbidden. The sweep still runs as one set-based UPDATE.
**Now load-bearing beyond D6:** D3's `settle` deliberately leaves rows in `sending`
when an entity transition fails. The sweep is no longer optional.

### E9b — ⚠ D6's sweep has no column to run on, and the cheap window has CLOSED
"Rows in `sending` older than a threshold" has no predicate. `next_attempt_at` holds
its **pre-claim** value, so a row enqueued 10 minutes ago and claimed 2 seconds ago
already looks stuck — resetting it while the original worker is still sending is a
**double delivery**. Verified: `notifications` has only `next_attempt_at`,
`created_at`, `sent_at`. No `claimed_at`/`updated_at`.
**D1 merged in #22 before this was decided, so this now costs a NEW migration**, not
a one-line edit. My error — I flagged the time-pressure but did not hold the PR.
Options: (a) new migration adding `claimed_at`; (b) specify `ClaimBatch` to set
`next_attempt_at = now` on claim (no schema change, but the semantics must be written
into the `ClaimBatch` doc comment, which currently does not say so).

### E10 — second-factor bypass shape in merged identity code
`auth_models.go` maps three **nullable** columns to plain `string`, collapsing NULL to
`""`. **`mfa_secret` is the serious one:** a row with `mfa_enabled = true AND
mfa_secret IS NULL` yields `totp.Validate(code, "")` — an empty secret base32-decodes
fine and produces a **publicly computable** valid TOTP code. It fails **open**.
`password_hash` fails *closed* (bcrypt rejects `""`) but read-then-save silently
rewrites NULL to `''`, and the fast rejection is an enumeration timing signal.
Not reachable through current app paths — needs a partly-failed setup or a manual DB
edit. **`tools/schemacheck` cannot catch this class at all**: it compares column
*names* only, never types or nullability.
Remediation, in order: `NOT NULL` in a new migration after a data check; independently
guard `ValidateCode` on an empty secret (cheap, fails closed, no schema dependency);
extend schemacheck to assert `is_nullable == pointer-ness`, turning the class into a
CI failure. **Outside every task's file list — needs a task.**

### E11 — `infrastructure` may not import `application`, but D5 and D8 must ⚠ blocks D5
R1 (`dependency-rules.md`, guardrail 5, blueprint §3) forbids `infrastructure →
application`. But blueprint §5 puts `ChannelSender`, `RenderedMessage`,
`TemplateRenderer` and `ErrNonRetryable` in `application/ports.go` — so every sender
must import `application` just to **spell its own method signature**.
**Neither gate catches it:** archtest only checks application→infrastructure;
depguard has no infrastructure rule at all. D5 and D8 would ship green while silently
breaking the written table.
**Reviewer recommends amending the rule, not moving the code** — an adapter
implementing a port declared by the inner ring is textbook hexagonal; control flow
still points inward, and R1's real intent is that infrastructure must not *invoke use
cases*. Amend R1 to `infrastructure → domain, own application's ports, bun, pgx,
external clients`, and add an archtest rule permitting `modules/X/infrastructure →
modules/X/application` while forbidding another module's application, so the
allowance is explicit rather than accidental. **Predates D3; must land before D5.**

### E12 — `DispatchBatch (int, error)` cannot support D10's metrics or audit
D10 needs `notification_{sent,failed,dead}_total{channel}`, a latency histogram, and
— per blueprint §6 — an `internal/audit` event on **security-category dead-letter**,
which is a security requirement. The dispatcher only sees `(int, error)`: it cannot
know sent vs failed vs dead, on which channel, or that a security row died. All of it
is computed inside `settle` and discarded.
**Settle now or pay at D10** as a forced change to a contract merged two PRs earlier,
plus a D6 rewrite. Reviewer prefers an **observer port** over a stats struct because
it also fixes the panic-visibility gap and gives per-row channel/category:
`DispatchObserver.Settled(n, outcome, err)`, optional (nil ⇒ no-op, like `jitter`),
called once per row. D10 then adds audit emission with no further contract change.

### E13 — no task ships an in-app sender; every in-app row would dead-letter ⚠
`inapp` and `webhook` appear **nowhere** in the plan's task list. Blueprint §3 lists
`sender/inapp.go` and `sender/webhook.go`, but D5 ships only `log.go` and D8 only
`email_smtp.go`. With D3's correct fail-closed behaviour, an unregistered channel
dead-letters immediately — so at the D6 merge gate **every in-app notification goes
straight to `dead`**, the exact channel D7's read model and route table exist to
serve. It fails visibly (rows still list, each `Status: dead`,
`notification_dead_total{inapp}` alarms permanently, security rows fire the
dead-letter audit).
**Add `sender/inapp.go` to D5's file list** (trivial always-succeeds sender) and
record **webhook as an explicit v1 non-goal** in blueprint §13, or webhook rows
dead-letter too.

### E14 — template shape: the two documents are on orthogonal axes and BOTH are incomplete
Plan D5 says `<key>.<channel>.tmpl` (channel axis); blueprint §3 says
`*.subject.tmpl` / `*.html.tmpl` / `*.text.tmpl` (MIME-part axis). **You need both.**
D3 shipped a hybrid (`RenderedMessage{Subject, Body}`) that papers over the hole.
Reviewer: **single-body is not correct for v1** — `text/html` with no `text/plain`
alternative is a measurable deliverability penalty, and the mail it costs you is the
security mail. Proposed naming
`security.password_changed.email.{subject,html,text}.tmpl`,
`security.password_changed.inapp.body.tmpl`; and D8 needs an **added field**, not a
second render call (`multipart/alternative` needs both parts in one `Send`):
add `HTMLBody string` to `RenderedMessage`, optional, empty ⇒ text only, leaving log /
in-app / webhook untouched. **Do not reopen D3** — amend D5's file list to include
`application/ports.go` for that one field. Costs one field with zero consumers now;
three files after D5 and D8 ship.

### E15 — nobody owns address resolution ⚠ blocks D8
Nothing turns `RecipientID` (a bare uuid, deliberately no FK) into an email address.
Confirmed by construction: no address column, D8's file list is
`email_smtp.go` + test, D9's adapter produces an `EnqueueRequest` carrying no address.
D3 already committed to an answer in prose (`ports.go:35-38`: resolution is the
sender's job) and the reviewer recommends **ratifying** it rather than re-litigating —
the alternative forces an address field onto every channel that has none.
Proposed task **D8a, gating D8**: declare `AddressResolver` in
`infrastructure/sender/` (`Resolve(ctx, recipientID) (string, error)`) with a
"no such user" sentinel mapping to `ErrNonRetryable` (unfixable by waiting) while DB
failure stays retryable; adapter in `cmd/api` over identity's
`UserRepository.FindByID`. Note the recipient is the **identity** uuid — the user
module keys on `int64`.
**Blocker inside the blocker:** `identity.New` returns `(*module.Module, error)` and
exposes nothing resolvable. Widening it mirrors D6's own `New(...) (*module.Module,
Service, error)` shape and is cleaner than `cmd/api` reaching into module internals,
which `container.go`'s own stated convention forbids.
**Rejected option, recorded:** snapshotting the address at enqueue. It copies PII into
`payload` for every mail and mails a **stale address after an email change** — so
"your password was changed" goes to the address the attacker just replaced.

### E7 — `auditlog` has no tests and trips a pre-existing 60% floor
8 functions, no test file, matches `modules/*/infrastructure/...` min 60. Invisible
today only because CI never reaches the coverage step (E8); an immediate failure the
instant E8 is fixed. Sole such package. Fix is a test or a reasoned `exempt` — **not a
lower floor**.

### E8 — CI red for 12+ consecutive runs, all pre-existing
1. `internal/db` `TestCaseInsensitiveIdentifiers_RefusesCollidingData` fails on a
   unique-constraint violation, **upstream of the coverage step**, so guardrails 7/8
   stay unverifiable in CI. Highest-value fix in the tree.
2. **The CI `golangci-lint` job cannot start** (built with go1.24, targets 1.25.0) —
   it has produced **no findings at all**. Local `make lint` is the repo's only lint
   coverage, i.e. every green reported here rests on one machine.
3. `govulncheck` fails on stdlib advisories. Likely a toolchain bump.

## Doc amendments I cannot make (I do not edit the plan or companion docs)

- **Blueprint §4.1** backoff formula reads `min(base*2^n, cap) + jitter` (jitter *on
  top of* the cap); shipped code keeps it **inside** (`[d/2, d)`, never exceeding
  cap). If the doc stands, D10's docs pass will "reconcile" the code and silently
  reintroduce over-cap waits.
- **Blueprint §6** attributes jitter to the dispatcher; §4.1 puts it in the domain's
  pure function. The blueprint contradicts itself before D3 is involved. D6 still
  *owns and injects* the source — only the application point moved, same arrangement
  as `Clock`.
- **Plan D2/D3** "stdlib + `internal/platform`" should read "+ `github.com/google/uuid`"
  — the enforced depguard rule is a deny list and permits it; identity's domain
  already imports it.
- **Plan D6** config `Validate()` omits `max_attempts >= 1`. Without it a zeroed field
  dead-letters every notification on its first transient failure.
- **Plan D4** file list says flat `persistence/{notification_repo,preference_repo}.go`;
  it must be `persistence/repository/` (see D4 note).
- **Plan B4** coverage clause — T1 already landed the `internal/authn` floor+baseline.

## D4 dispatch adjustments — banked, ready when #23 merges

1. **Path deviation is mandatory, mirroring D1.** Repositories go in
   `persistence/repository/`, not flat. `tools/coverage/policy.json` lists
   `modules/*/infrastructure/persistence/repository` under `requires_database`; a flat
   layout draws the 60% floor with tests that skip without a database.
2. **`ON CONFLICT (idempotency_key) DO NOTHING` does not compile** against a *partial*
   unique index — `ERROR: there is no unique or exclusion constraint matching the ON
   CONFLICT specification`. Required form:
   `ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING`
   (the `WHERE` goes **before** `DO`). Verified: `INSERT 0 0` on duplicate,
   `INSERT 0 1` for a NULL key.
3. **`ClaimBatch` must bind the caller's `now`, not SQL `now()`** — otherwise D3's
   injected `Clock` is bypassed and its fixed-clock tests describe behaviour the
   repository does not have. Binding a parameter does not change the plan.
4. **Register the two tables in `tools/schemacheck`** (`schema_test.go`, `cases` slice)
   — D1 could not, because its acceptance forbade another package importing the models.
   That clause expires here. Until it lands, CI's drift guard does not cover this
   module, and the same test is CI's tripwire that a database was reachable at all.
5. **`""` ↔ NULL bridging** for both `IdempotencyKey` and `LastError` (domain uses
   `string`, D1's model uses `*string` — deliberate on both sides). Round-trip test:
   two rows with `IdempotencyKey: ""` must both persist (no unique-index collision)
   and read back as `""`.
6. **`MarkRead` must test the negative case** — `MarkRead(ctx, otherUsersID, id, now)`
   returns `ErrNotFound` and leaves `read_at` NULL. D7's cross-user 404 rests on it.
7. **Concurrency test:** the two goroutines must use **separate connections**
   (`SetMaxOpenConns >= 2`) — assert it, or the test passes for the wrong reason via
   silent serialisation. Do not add an ordering assertion: `ORDER BY next_attempt_at`
   has no tiebreaker and ordering among equal timestamps is nondeterministic by design.
8. Claim-path plan verified on 50k rows: `Index Scan using idx_notifications_claim`,
   **no Sort node**, `Limit` above `LockRows` (so `SKIP LOCKED` rows do not consume the
   limit and a second dispatcher still gets a full batch).

## D6 dispatch adjustments — banked

- **Pass a NON-CANCELLED drain context.** `settle` calls `Save(ctx, ...)`; against real
  bun/pgx a cancelled ctx fails the save, leaving every in-flight row in `sending`.
- **Shutdown burns retry budget** — a cancelled ctx reaching `Send` fails as
  `context.Canceled`, retryable by default, consuming one attempt per row. Special-case
  it or drain with a fresh context.
- **Jitter source must be concurrency-safe.** `jitter` is one long-lived field on
  `Service` and D6 runs `workers` concurrent `DispatchBatch` calls. `*math/rand.Rand`
  is **not** goroutine-safe. Inject `math/rand/v2`'s race-free global or a
  mutex-guarded source — never a bare `*math/rand.Rand`. D3's tests cannot catch this.
- **Propagate the error from `NewService`** — it fails closed on nil deps and duplicate
  or unknown sender channels.
- Config `Validate()` must include `max_attempts >= 1`.

## B-phase adjustments — banked

- **B1** must add a sibling depguard rule for tests. A4's `transport-kernel-access`
  exempts `**/*_test.go`, so transport tests may import **anything** under `internal/**`
  (reviewer proved it). A test-scoped rule allowing `authn`, `httpx`, `testsupport`
  closes it with no later revert, and keeps the production rule at exactly two packages.
  **`list-mode: lax` is load-bearing** — under the default, a non-empty allow list
  forbids everything not listed, including chi.
- **B2** `modules/user/module.go:42` is `AuthMiddleware`, not `Config.Auth`. The rename
  is free (alias). Keep the nil fail-closed check and keep one real end-to-end test.
- **B3** `Principal` is not comparable — `got == want` will not compile. Do **not** add
  `Equal` or change fields. Do **not** export A3's `unauthorizedDetail`; hard-coding the
  literal in the suite is *better* (a second independent copy, and A3's constant test
  fails first). B3 also closes the weak e2e success-path guard (BL18). `go-cmp` is in
  `go.sum` but not a direct `go.mod` requirement — using `cmp.Diff` promotes it and must
  be a named line item or I escalate it.
- **B4** respecify: it still owns the archtest import fence and the deliberate-violation
  proof. `internal/authn/authntest` matches **no** policy pattern (the floor is
  non-recursive, and `exempt` lists `testsupport` but not `authntest`) — decide exempt
  vs floor. Re-verify `internal/authn: 100` after B2/B3.
- **C1** carries E3's approved changelog scope, E4's narrowing, E5's `request_id`.

## Backlog — pre-existing, do NOT spawn unplanned work

- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings (blame `6eed3aa`);
  the only lint findings repo-wide, confirmed at every task. Also a **dead duplicate**:
  it declares a second `TestModelsMatchMigratedSchema` superseded by
  `tools/schemacheck/schema_test.go`. Deleting it removes both findings.
- **BL2** Swagger annotations vs the 401 shape (A4 fixed the three it owned).
- **BL3** `modules/identity/transport/jwks.go:7` imports `infrastructure/token` in
  production, contradicting R1; A4's depguard rule does not catch it (denies
  `internal/**`, not sibling layers). Related to E11.
- **BL4** RSA keypair parsed twice at boot; deliberate. Do not "fix".
- **BL5** `identity.New` exposes only `(*module.Module, error)` — see E15.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern; `.zip` ignored by nothing.
- **BL12** Two stale worktrees remain under `.claude/worktrees` (`phase-0-hygiene`,
  `phase-3-consolidate`) with stale `.gitignore`/`CLAUDE.md`/tooling — source of one
  wrong root-cause diagnosis. Agents must exclude them for **config files**, not just
  code. (The two agent worktrees I created have been removed.)
- **BL13** `/auth/mfa/verify` OAuth2-shaped 401 — **accepted permanent divergence (E4)**.
- **BL16** **`golangci-lint` cache trap** — serves results keyed to deleted directories
  and prints a false `0 issues.`. **Always `golangci-lint cache clean` first.**
- **BL17** coverage tool notes: `-require-profile=false` exits 0 on an empty profile
  (must never appear in a Makefile or workflow); `tolerance = 0.05` softens **floors**
  as well as the ratchet; **floors are enforced only for packages present in the
  profile** — exactly what hid E7; `tools/...` is exempt so the gate does not gate itself.
- **BL18** `cmd/api/protected_routes_test.go:157` asserts only `rec.Code != 401`, and
  `Recovery` converts a panic to 500 — so a `MustFromContext` regression **passes** the
  only e2e success-path control. B3 closes it.
- **BL19** Neither notification package has a coverage **baseline** entry (floors apply
  by wildcard), so a slide from 100% to the floor passes unnoticed. D10 must run
  `coverage-update`, not just add floor entries.
- **BL24** The module is **at-least-once** by design: default-retryable means an SMTP
  DATA accepted but a connection dropped before the 250 re-sends, so a user can get two
  password-changed mails. Correct trade (two beats zero), but D8/D10 need it on record.
- **BL25** Panic visibility gap: a panicking sender is recovered into an ordinary
  retryable failure, so `DispatchBatch` returns nil and D6 gets **no error to log** —
  only `LastError` on the row, with no stack and a 512-byte cap. A deploy that panics in
  the SMTP sender silently dead-letters every security mail. Fix belongs with E12's
  observer hook — **not** by widening `LastError`.

## Deviations accepted

- **A1 (4), A2 (1), A2 copy-on-write, A3 (2 vs §8.2, pre-authorised by E5), A4 (2)** —
  all adjudicated; see PR history. A4's `Scopes`-nil and empty-`sub` rejection were both
  confirmed correct, the latter a strict tightening.
- **T1 (1)** — **refused** my instruction to baseline five unmeasurable packages.
  Reviewer: "I would have blocked the PR had it complied." The instruction was mine and
  it was wrong.
- **D1 (2)** — models at `persistence/models/` (triple-supported: repo convention,
  `schemacheck`'s import, and `policy.json`'s `requires_database` glob); disposable
  Postgres on a non-default port because a native server held 5432. Plus the `nullzero`
  payload fix, added on review before merge.
- **D2 (2)** — `BackoffWithJitter` additive to the plan's exact `Backoff` signature
  (the plan's signature has nowhere to inject a source while demanding determinism);
  `uuid` import per the **enforced** rule over the plan's prose.
- **D3 (8)** — all adjudicated ACCEPT except the template-shape one, reopened as E14.
  Notably: `NewService` fails closed rather than substituting Nops; `Enqueue` generates
  the row uuid (a caller-supplied id is an overwrite surface); jitter applied in the
  application because `NextAttemptAt` is written there and a second write would race.
