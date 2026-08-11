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
| **T3** | platform-engineer | **merged** | #33 | APPROVE-WITH-NOTES |
| B2a | transport-engineer | **superseded by E16-3** (was BLOCKED — E16) | - | - |
| T2  | test-guardian | **merged** (via #35 — PROCESS-4) | #35 (`36784b6`) | **APPROVE** (verdict transferred, see below) |
| T4  | platform-engineer | **APPROVED — MERGE WHEN READY** | **#42** (`799c861`) | **APPROVE-WITH-NOTES**; amendment landed + verified |
| T5  | platform-engineer | **merged** | #36 | not gated by me — landed from a prior session |
| C1  | docs-scribe | **APPROVED — MERGE WHEN READY** | **#43** (`66b69e1`) | **APPROVE-WITH-NOTES** after BLOCK #1 fixed |
| D5  | platform-engineer | **BLOCKED — E11, E13, E14** | - | - |
| D6  | platform-engineer | **BLOCKED — E9b, E9c, E12** | - | - |
| D7  | transport-engineer | **UNBLOCKED** — needs D6 | - | - |
| D8  | platform-engineer | **BLOCKED — E15, E11** | - | - |
| D9a/D9b, D10 | domain / platform | pending | - | - |
| E16-P | test-guardian | **merged** | #37 (`1dc6aa7`) | **APPROVE-WITH-NOTES** |
| E16-1 | persistence-engineer | **BLOCKED — E22 only** (E20 answered) | - | - |
| E16-2 | domain-engineer | **BLOCKED — E24** (empty DTOs) | - | - |
| E16-3 | transport-engineer | **BLOCKED — E24**; byte-identity on GET/PUT/DELETE (E23) | - | - |
| E16-4 | docs-scribe | pending — needs E16-3 merged; carries **E19**, BL40 | - | - |
| E16-5 | unassigned | **UNSCOPED — E21** (`cmd/generator` reproduces the defect) | - | - |

**Phase A and Phase B are complete on `main`.** Open PR: **#41** (this board).
**In flight 2026-08-11:** T4 (`ci/T4-golangci-lint-v2`) and C1 (`feat/C1-authn-contract-docs`),
dispatched in parallel — no dependency edge between them, disjoint file sets, neither touches
`cmd/api`. They are the ONLY two tasks on the board that need no answer from the human first.
`gh pr merge` and pushes to `main` are blocked by the sandbox classifier, so a human
merges; branch pushes and PR creation work.

## WAITING ON YOU — six answers, and what each one unblocks
Nothing below is mine to decide. Everything else on this board is blocked behind one of them.
| Ask | Blocks | One-line question |
| --- | --- | --- |
| **E22** | E16-1, and the whole E16 chain behind it | Does **any** environment have rows in `users`? (`make migrate-status`) |
| **E24** | E16-2, E16-3 | Dropping `name`/`email` empties the write API — option (a), (b) or (c)? |
| **E23** | E16-3's acceptance | Approve **E16-P2** (test-guardian, ~20 lines): capture `put_not_found`/`delete_not_found`? |
| **E19** | E16-1 (as a review objection) | Confirm the E16-ARCH decision supersedes `migration-strategy.md:249-251`? |
| **E21** | E16-5 | `cmd/generator` reproduces the defect into every future module — scope it, or accept it? |
| **E11/E13/E14** | **D5**, and D6/D8 behind it | The Phase D blockers, unanswered since Phase B closed. |
**D5–D10 have now been parked longer than the E16 chain.** E11 is a one-line amendment to R1
plus one archtest rule; it is the cheapest unblock on this board and it releases the entire
notification back half. If you answer one thing today, answer E11.

## PROCESS-6 — RESOLVED, and PROCESS-3 recurred a third time
The stray edit is **gone** — T4 reports `authn-spi-impact-analysis.md` was already clean when
it arrived, and the shared tree is now clean on `main`. Nobody needs to act.
**But the shared tree moved branches again, mid-session, and not by me or by T4.** Its reflog:
`checkout: moving from feat/T2-authn-layer-fence to main`, then `pull --tags origin main`.
T4's only shared-repo commands were read-only (`fetch`, `status`, `branch`, `log`,
`worktree list`, `worktree add`), none of which move a worktree HEAD. **T2's commit
`00fa2e5` is verified safe on its branch** (checked by me, not taken from the report).
Whatever is doing this, per-task worktrees are containing it — both of today's dispatches
required one and neither was disturbed. Keeping the requirement permanently.

## PROCESS-6 (original entry) — a stray edit to a source-of-truth doc, in the shared tree
Found at session start: `docs/plans/authn-spi-impact-analysis.md` is modified in the shared
working tree, uncommitted, its H1 reading `ne# Security SPI …` — two stray keystrokes, not a
semantic edit. Consistent with PROCESS-3's second hazard (agents sharing one checkout).
**I have not touched it** — companion docs are not mine to edit, and reverting it would destroy
the only evidence of how it got there. It is inert as long as work happens in per-task
worktrees, which both of today's dispatches require explicitly. **Ask:** discard it
(`git restore`), or is it yours in progress?

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

## GATE-INTEGRITY — AMENDED: isolating `GOCACHE` alone is NOT enough for `make lint`
Raised by T2's reviewer, with the tool's own diagnosis as evidence. Its first `make lint` from
an isolated worktree **with `GOCACHE` already isolated** printed **`0 issues`** — a false green
that would have let it report BL1 as fixed. golangci-lint keeps its **own** cache, keyed
independently, and said so:
`[runner/path_relativity] Getting relative path (basepath): Rel: can't make …\internal\db\schema_test.go relative to …\scratchpad\wt-t2`
Three such warnings, naming exactly the files whose findings vanished. Setting
**`GOLANGCI_LINT_CACHE`** alongside `GOCACHE` restored the two errcheck findings.
**Status: provisional.** Per PROCESS-2 I am not recording a mechanism from one machine — it
needs a CI or second-toolchain reproduction — but it is stronger than an A/B inference because
the tool printed its own reason. **Adopted in dispatches now** (isolate both; it costs nothing
and the failure mode is silent under-reporting). Every earlier "lint clean from a worktree" on
this board, including reviewers', is suspect to the extent it isolated only `GOCACHE`.

**THIRD OBSERVATION, and this one was blind.** C1's agent — which I never told about the cache
mechanism — ran `make lint`, got **`0 issues`**, and printed `path_relativity` warnings naming
`internal/db/schema_test.go`: the exact file holding BL1's two known-real findings, which T4 was
concurrently fixing. It reported the `0 issues` in good faith as a passing check. An agent that
did not know the mechanism reproduced its precise signature.
**I am still NOT promoting this out of PROVISIONAL.** All three observations are on this one
machine, and PROCESS-2 exists because I published a wrong mechanism three times in this exact
area off single-machine evidence. Three same-machine observations are still one machine. What it
does establish beyond doubt: **`make lint` returning `0 issues` is not, by itself, evidence of
anything on this effort** — including in reports I have already accepted. Dispatches keep
requiring both caches isolated.

## PROCESS-4 — I dispatched a task that was already written, and it cost a duplicate PR
At session start `git branch` reported **`test/T2-authn-layer-fence: 1 commits ahead`**. I
read that line as an aborted attempt and dispatched T2 as if unstarted — I never opened the
commit. It was T2, finished. It merged as **#35 (`36784b6`)** while my duplicate was being
written, and my branch then conflicted in the one file both touched. Closed as **#39**, not
rebased, not force-merged; the branch stays in history.
Both versions add the same two entries and both keep the fence's `modules/*` recursion; the
merged one spells the key as the existing `authnPkg` constant, mine as `internalPrefix + "authn"`.
**Compounding cause:** the T2 row I trusted said "ready", and it came from the stale board of
PROCESS-3. Two stale sources agreeing is not corroboration when they share an origin.
**Standing correction:** a precondition check is `git log <branch>` and the **contents** of
any branch bearing the task's id — never the board row alone, and never a branch's
ahead-count. "Ahead by N" is a question, not an answer.
**Not wasted:** the agent reproduced the gap empirically — an `internal/authn` import planted
in `modules/user/domain` and `modules/user/application` left `make arch` **green** — which is
independent confirmation that what #35 shipped was necessary rather than defensive.

## PROCESS-5 — two PRs merged ahead of their gate verdict
**#37 (E16-P)** and **#35 (T2)** are on `main` while the arch-reviewer dispatched against them
was still running. Merges are the human's to make and pushes to `main` are human-only, so this
is not a violation of the gate by me — but it does mean the verdicts arrive **after** the code
ships. **Handling:** verdicts still land on the board, and a BLOCK becomes a fix-forward task
against `main` rather than a fix on an open branch. The T2 review is being re-pointed at
`36784b6`, since that is the version that actually shipped.

## PROCESS-3 — two working-tree hazards, both mine to prevent
1. **I began this session on a stale checkout.** The tree sat on `ci/T5-swagger-cold-cache`,
   whose board is 496 lines and predates `5d319c3`; reading the working file gave me a
   **superseded board**, including entries this board has since retracted.
   **Standing correction:** read `git show origin/main:docs/plans/task-status.md` before
   trusting the board — E1's rule, now applied to my own file.
2. **A running agent checked its branch out in the shared working tree**, moving me off my
   branch mid-edit; my board edit landed as an uncommitted change on *its* branch (reverted
   before it could be committed). **Standing correction:** board edits happen in a dedicated
   worktree, and dispatches tell agents to work in their own worktree — `cmd/api`'s
   single-writer rule is useless if two agents share one checkout.

---

# ESCALATIONS — OPEN (17)

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
live code, not a demo. `BOILERPLATE.md:279` (quote: "`modules/user/transport/user.go` or") names `modules/user/transport/user.go` as the
worked example adopters copy — **the exemplar propagates it**.

**Root cause is not a missing `if`:** `auth_users.id` is uuid, `users.id` is BIGSERIAL, and
`grep -rn "auth_user_id|AuthUserID|REFERENCES users"` → zero matches. **There is no value to
compare.** B2 refused three shortcuts, all upheld: `strconv.ParseInt(Subject)` (a uuid never
parses — "an outage that lints clean"), `Subject == chi.URLParam("id")` (always false, and
*reads* like a real check), `Extra["email"]` (nil, and a mutable natural key).

## E16-ARCH — **DECIDED 2026-08-10 (human): option 1** — `users` becomes a profile keyed by the identity uuid
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

**THE DECISION, as given (2026-08-10):** *"E16 — a profile keyed by the identity uuid."*
Option 1. `users` stops being a separate entity and becomes the profile **of** an identity,
keyed by `auth_users.id`. This is now a settled decision: I do not reinterpret it, agents do
not reopen it, and anything that contradicts it gets escalated rather than resolved locally.

**What the decision closes.** `p.Subject == id` becomes a *real* check, because for the first
time there is a value to compare — the exact gap E16 named. Steps (3) and (4) above were
never blocked on it: D7 and D8/E15 remain independent.

**What it does not decide** — three questions it exposes, all escalated below and all
answerable only by the human: **E19** (a merged doc says the opposite), **E20** (does the
profile keep its own `name`/`email`?), **E22** (is there deployed `users` data to back-fill?).

**One question it does NOT reopen — settled by existing binding policy, verified.** The
reshape lands as a **new migration**; `00001_create_users.sql` is not edited.
ADR-010 §"Applied migrations are immutable" (*"a mistake is corrected by a new migration,
never by editing an existing one"*, and the ADR declares itself binding) and
`MIGRATIONS.md:343-347` both say so, and E16-P verified the practice matches the policy:
`git log --diff-filter=MRD -- migrations/` returns **empty output** — no migration file in
this repo has ever been modified, renamed or deleted. The `users` constraint change of
`00007` is the counter-precedent: it added a file rather than editing `00001`.

**Decomposition** (E16-P → E16-1 → E16-2 → E16-3 → E16-4, each merged before the next; a
uuid PK cannot be split across PRs without breaking `go build ./...` in between, so these
are sequenced, not parallel):
- **E16-P** *(merged/in-review, #37)* — goldens recording the defect: cross-user
  GET/PUT/DELETE currently return **200 / 200 / 204**, with B's row overwritten and then
  deleted. `get_cross_user` (200 + body) vs `get_not_found` (404 + `{"error":…}`) quantifies
  the enumeration oracle the fix must close.
- **E16-1** persistence — new migration + `UserDBModel`; `autoincrement` is invalid for a
  uuid PK. Must land with the model in one commit or `tools/schemacheck` fails.
- **E16-2** domain — `ID int64` → `uuid.UUID` through `domain/{user,dto,repository}.go` and
  `application/service.go`, including deleting the `id <= 0` guard, which has no uuid analogue.
- **E16-3** transport — `strconv.ParseInt` ×3 → uuid parse, `@Param id path int` ×3, and the
  ownership check itself. **The cross-user response must be byte-identical to the
  genuinely-not-found response**, or the 404 is a 403 with extra steps and the oracle
  survives (the D7 precedent).
- **E16-4** docs — `BOILERPLATE.md`, `SWAGGER.md`, `README.md`, `MIGRATIONS.md` examples,
  plus **E19**'s contradiction and BL40.

## E23 — the enumeration oracle is captured for GET only; PUT and DELETE are unguarded ⚠ small, must land before E16-3
Raised by E16-P's reviewer. The probe recorded `get_not_found`, so after the fix
`get_cross_user` can be checked byte-for-byte against it. **There is no `put_not_found` and no
`delete_not_found`.** The obligation applies to all three vulnerable verbs: a cross-user `PUT`
must be indistinguishable from a `PUT` against a genuinely absent id, and likewise `DELETE`.
**`204 vs 404` enumerates the id space exactly as well as `200 vs 404` does** — and today the
recorded cross-user answers are 200 and 204 while `modules/user/transport/user.go` maps
`domain.ErrUserNotFound` to 404 on both routes, so the difference is real, exploitable, and
currently unrecorded.
**Proposed E16-P2** (test-guardian, same file, ~20 lines): add `put_not_found` and
`delete_not_found` to `routeCases()` in `modules/user/transport/ownership_golden_test.go`,
extend `TestIDORIsPresentToday` with the matching `require.NotEqual` pairs, and add the
reviewer's N2 hardening — a `// E16: invert to require.Equal when the ownership check lands`
marker on each assertion, so the refactor's checklist can grep for them instead of relying on a
comment being read.
**Why I am asking rather than dispatching:** it is a **new task**, and new tasks are yours. It
is cheap, owner-consistent (that file is test-guardian's), and blocks nothing today since E16-1
is already paused on E20/E22 — so approving it costs nothing, and declining it costs the guard
on two of the three vulnerable verbs. **E16-3's acceptance criteria are amended on this board to
require byte-identity on all three verbs regardless of how E23 is answered.**

## E19 — a MERGED doc contradicts the settled decision ⚠ needs a ruling before E16-1
`docs/architectures/05-data/migration-strategy.md:249-251` states that `users` *"moves with
the profile-user module **rather than being reshaped**"* — the opposite of the decision. The
same file already knows the problem (`:220-221`: the `BIGSERIAL`/uuid mismatch means *"any
table backing them must use uuid to match"*) and warns at `:240` that the `users`/`products`
migrations cannot be edited in place if any environment has applied them.
Found by E16-P. **I do not edit docs and I do not adjudicate a merged doc against a
decision.** Left standing, this becomes a legitimate review objection against E16-1.
**Ask:** confirm the decision supersedes `:249-251`, and E16-4 rewrites that paragraph.

## E20 — **ANSWERED 2026-08-10 (human): drop `name` and `email` from the profile.**
Settled. The profile stops carrying a second copy of a person's identity data; `auth_users`
owns `email` and `name`, and E15's staleness hazard is closed at the source rather than
managed. Nothing downstream reads the profile's copies — E16-P established that their only
consumer is the `/api/v1/users` JSON echoing back what was posted.

**What the profile table becomes.** Verified against `migrations/00001_create_users.sql:3-9`,
which is the whole table: `id`, `name`, `email`, `created_at`, `updated_at`. Remove two, rekey
the first, and the profile is **`id uuid` (= `auth_users.id`) + `created_at` + `updated_at`**.

**Three consequences that follow mechanically, and are not open questions:**
1. `GetUserByEmail` (`application/service.go:54-55`, `domain/repository.go:11`) becomes
   impossible and is deleted, not ported. It was already unreachable over HTTP — transport's
   `UserService` interface never declared it.
2. `domain.Validate`'s body was entirely name/email rules; it empties out.
3. **`00007`'s users work must be undone by the new migration**
   (`00007_case_insensitive_identifiers.sql:73-78`): it dropped `users_email_key` and
   `idx_users_email` and created `idx_users_email_lower` **on `lower(email)`**. That index
   cannot survive the column. ⚠ **Reversibility trap for E16-1:** `00007`'s own `Down`
   (`:83-87`) runs `ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email)`, so if the
   new migration's `Down` does not restore an `email` column of the right shape, rolling back
   past `00007` **fails outright**. And a `Down` can restore the *column* but never the
   *values* — with rows present, restoring `email VARCHAR(255) UNIQUE NOT NULL` cannot succeed.
   **This is a second, independent reason E22 must be answered before E16-1 is dispatched.**
   Class warning already on this board (E8): any `goose.Down()` caller breaks when a newer
   migration lands.

## E21 — `cmd/generator` reproduces the defect into every future module ⚠ unscoped, no owner
`cmd/generator/main.go:202-206` hardcodes `PrimaryKey = {ID, int64, id}` with nothing
overriding it; `:714` `if id <= 0`; `:910` `bun:"id,pk,autoincrement"`; `:979/1051/1094`
`@Param id path int`; `:988/1061/1103` `strconv.ParseInt`. `BOILERPLATE.md:279` (quote: "`modules/user/transport/user.go` or") points
adopters at `modules/user/transport/user.go` as the worked example.
**Fixing `modules/user` without the generator means the next generated module ships the same
IDOR shape** — and E16's severity rests on exactly that amplification.
This is a **new task, outside every existing task's file list** ⇒ scope escalation. Natural
owner is platform-engineer (`cmd/**`), which then serialises against the `cmd/api`
single-writer rule. **Ask:** in scope now, after E16-3, or a recorded non-goal?

## E24 — dropping the columns empties the module's write API ⚠ NEW, blocks E16-2/E16-3
Direct consequence of E20, verified in `modules/user/domain/dto.go:8-25`:
`CreateUserRequest` is `{Name, Email}` and `UpdateUserRequest` is `{Name, Email}` — **every
field either one has is a column you just dropped.** They become empty structs. `UserResponse`
becomes `{id, created_at, updated_at}`.

So after E16-2/E16-3 the module offers: `POST /api/v1/users` with an empty body, `PUT
/api/v1/users/{id}` that can change nothing, `GET` returning two timestamps, `DELETE`. **A
`PUT` with no writable field is not a route, it is a 200 that lies.** I will not quietly ship
that, and I will not redesign a public API surface on my own authority.

**Also in play: `modules/user` is the exemplar.** `BOILERPLATE.md:279` (quote: "`modules/user/transport/user.go` or") names
`modules/user/transport/user.go` as the worked CRUD example adopters copy (the same
amplification that makes E16 severe, and the subject of E21). An exemplar whose entity has no
fields teaches nothing about CRUD.

**Options, decision-ready:**
- **(a) Keep four routes, shrink the DTOs.** `POST` takes no body and creates the *caller's own*
  profile with `id` from the Principal — not from a body, which also removes the "unrestricted
  creation behind mere authentication" leg of E16. `PUT` is deleted or answers 405.
  Smallest diff; leaves a CRUD exemplar with nothing to C or U.
- **(b) Reduce the surface honestly:** `GET /api/v1/users/me` + `DELETE`, drop `POST`/`PUT`.
  The profile becomes an existence record. Cleanest semantics; biggest route-table change; the
  exemplar stops being a CRUD example at all.
- **(c) Give the profile real profile-only fields now** — `display_name`, `avatar_url`,
  `locale`, `timezone` — so the module stays a meaningful CRUD exemplar and the four routes keep
  their meaning. Note `display_name` is *not* `name` returning by the back door: it would be
  profile-owned presentation data, with `auth_users.name` remaining the identity's legal/display
  name. Largest scope; needs you to name the fields, since inventing a schema is not mine.

**Recommendation withheld deliberately** — (b) and (c) are opposite answers to "what is this
module *for*", which the plan does not settle and I am not entitled to settle.

## E22 — is there deployed `users` data? ⚠ blocks E16-1's back-fill decision
Repo-side facts are settled (E16-P): **no seed data anywhere** — no seed SQL, no
`docker-entrypoint-initdb.d`, no Makefile seed target; every integration test calls
`TruncateAll` at **setup**, and `internal/testsupport/db.go:145-169` discovers tables
dynamically, so a reshaped `users` is truncated with no helper edit. A clean break therefore
costs **nothing** in dev/test.
A back-fill is also *possible* if rows exist: `email` is joinable both ways —
`idx_users_email_lower` and `idx_auth_users_email_lower`, compatible types — though it would
match on a **mutable natural key**, which B2 already refused for the ownership check.
**Only you know whether a deployed environment holds rows.** ADR-010:113-117 leaves the
disposition explicitly open; `MIGRATIONS.md` says verify with `make migrate-status` against
every environment first. **Ask:** clean break (drop/recreate), or back-fill by `lower(email)`
with a documented fallback for profiles that match no identity?

**ADDENDUM after E20 was answered — the question narrows but does NOT dissolve.**
With `name` and `email` dropped, a surviving `users` row carries **no information except that a
profile exists** for that identity, and that is reconstructible from `auth_users`. So a
back-fill would preserve almost nothing. **But E20 also made rows actively dangerous:** the new
migration's `Down` must restore `email VARCHAR(255) UNIQUE NOT NULL` for `00007`'s own `Down` to
run at all, and **with rows present that restore cannot succeed** — a `Down` can recreate a
column but never its values. So:
- **No rows anywhere ⇒ clean break**, and reversibility is honest because there is nothing to
  lose.
- **Rows in some environment ⇒** you are choosing between a one-way migration (documented as
  irreversible, contradicting `CLAUDE.md`'s reversible-migrations convention) and a `Down` that
  fabricates placeholder emails to satisfy a `NOT NULL UNIQUE` restore — which I would refuse to
  ship without you saying so explicitly.
**The ask is now a yes/no:** does any environment have rows in `users`? `make migrate-status`
per environment, per `MIGRATIONS.md`. Answer that and E16-1 goes out immediately.

## E7 — CORROBORATED on a second toolchain, unprompted
T4 ran `make coverage-check` on go1.25.12 and got
`modules/identity/infrastructure/auditlog: 0.0% < floor 60.0%`, `exit status 1`. It then
stashed its own changes, re-ran against pristine `origin/main`, and got the **byte-identical**
failure. This is exactly the mechanism the retracted COVERAGE-BLINDSPOT entry denied, arrived at
by an agent that was not asked about it and had no stake in it. E7 stands, and the retraction
was right. **Still not T4's to fix** — it needs a test or a reasoned `exempt`, never a lowered
floor, and it needs an owner from you.

## E25 — a STANDING AGENT DEFINITION carries a convention the merged code contradicts ⚠ NEW
C1 was briefed — by its own agent definition, not by my dispatch — that provider modules return
`(*module.Module, RootInterface, error)`. **It refused to document it**, because merged code says
otherwise: `modules/identity/module.go:88` returns `(*module.Module, error)`, and the
composition root builds its own verifier at `cmd/api/container.go:95-105`. It documented what
exists and flagged the conflict. That is exactly the right call and I am recording it as such.
**Why this is yours and not mine:** the error is in `.claude/agents/*.md`, which I do not modify —
proposed changes to agent definitions are escalations by my own constitution. Left standing, every
future docs task starts from a false premise about the constructor shape, and constructor shape is
a settled decision on this effort. **Pending arch-reviewer's independent confirmation of the
merged signature** (asked for in #43's gate); if confirmed, **ask:** correct the agent definition.

## C1 — BLOCK #1, and the reviewer earned its keep
Two must-fix findings, both false claims in adopter-facing text, both surgical. Returned verbatim
to docs-scribe on the same branch. **This is BLOCK #1; a second consecutive BLOCK on C1 escalates
with both reports.**

**BF-1 — a security falsehood, and exactly what Adjustment 7 existed to prevent.**
`08-authn/README.md:486-489` listed "handlers read the caller through the contract" as one of
"four properties worth copying" from `modules/user` — the module the doc calls **the template**.
Merged code says the opposite in terms, in the very package cited:
`modules/user/transport/user_test.go:179-182` — *"no handler in this package reads the Principal"*;
`ownership_golden_test.go:293-296` — *"the handler never reads that Principal. Not once, in any of
the four routes... it is the finding"*. The cited line sits inside a **test probe middleware**, not
a handler. **The absent property is E16's IDOR.** An adopter copying the template would have
believed exactly the ownership-awareness whose absence is the vulnerability — published in a
document that would have outlived the fix chain.

**BF-2 — enforcement theatre, in the file that teaches enforcement.** `BOILERPLATE.md:361` claimed
four rules "all enforced by `make arch` and `make lint`". Two are enforced by **nothing**: no rule
constrains what a module's `Config` may hold (a config holding a verifier passes both tools), and
"apply the middleware in the handler" is not expressible in either. The reviewer enumerated all
seven `arch_test.go` tests as evidence. In a repo whose own arch test opens with a warning about
vacuous greens, that is a material defect.

**What passed, and it is worth recording:** 40+ of the 79 citations spot-checked across all six
areas — **every one exact**. E3, E4, E5 and adjustments 4/5/6 all landed. Section 8.2 reproduced
verbatim including its line wrap. The replacement recipe was traced end to end and **would work**,
with both caveats real. All three declared deviations **allowed** — the root `CHANGELOG.md` and the
index edit independently confirmed as forced by real gaps, matching my own check.

**My rulings on the non-blocking notes:** fold in the `:173-174` overgeneralisation (same class as
BF-1 — a property of one implementation stated as a property of the contract), since the file is
open anyway; **do not** add the BL13 identifier — it is an internal board ID, meaningless and
unresolvable to the adopters the document addresses; leave `:345-348` as written.

## C1 — APPROVE-WITH-NOTES at `66b69e1`. Both blocking findings discharged on their merits.
Fix scope verified by me: `git diff d4eef29 66b69e1` touches **only** `BOILERPLATE.md` (+4/-1) and
`08-authn/README.md` (+18/-7); PR total still the same four files. No code, no `CHANGELOG.md`, no
agent definition.

**The deviation, and why I accepted it.** The owner changed a **fourth** location I had not
authorised: the lead-in at `:476`, from `Four properties worth copying:` to `Three properties worth
copying from it, and one rule that goes with them:`. Its argument is that BF-1 is **not discharged
by rewriting the bullet alone** — bullets 1-3 all cite `modules/user`, so leaving "Four properties
worth copying" above a bullet that now cites `modules/identity` still tells an adopter that reading
the Principal is something `modules/user` does. *"A weaker form of the exact falsehood the finding
blocks on."* It also observed that the reviewer's own model, `BOILERPLATE.md:370-373`, sits under a
**rules** list, so matching the model implies matching its frame. **The count word was
load-bearing** and I did not see it; the owner did. Accepted, and put to the reviewer to confirm the
finding is discharged.

**A second, smaller overreach I also accepted:** it dropped `make lint` from `BOILERPLATE.md:361`
entirely, beyond the reviewer's prescribed text, on the ground that no depguard rule denies
cross-module imports or names `internal/authn`, so `make arch` carries both enforced bullets alone.
Consistent with BL43, but a factual claim beyond the prescription — flagged to the reviewer to
confirm or reject.

**This is BLOCK #1 only.** A second consecutive BLOCK escalates to the human with both reports
rather than going back to the owner a third time; I have told the reviewer so, and asked it to say
explicitly if a remaining issue is small enough to carry as a note instead.

**RE-REVIEW OUTCOME — no must-fix remains; `#43` is ready for your merge.**
- **BF-1 closed, verified by sweep not by reading.** `grep` for `FromContext` across all three
  changed docs returns only citations pointing at `modules/identity`, `internal/authn` and
  `internal/testsupport`; `user_test.go` appears nowhere. **Nothing in the document now attributes
  Principal-reading to `modules/user`.**
- **The fourth-location deviation ACCEPTED by the reviewer, in my favour and the owner's.** Its
  words: the re-review shape *"was a forecast of the minimal edit, not a boundary"*, and — the part
  worth keeping — *"had the owner shipped three locations I would have had to block a third time on
  the residual count-word, which is the outcome your escalation rule exists to avoid."* A remedy
  that stops one line short of closing its own finding is not a smaller change, it is an unfinished
  one.
- **The `make lint` removal was not merely allowed, it was CONFIRMED CORRECT — and would have been
  a finding if left in.** The reviewer read all five depguard rules (`.golangci.yml:36-146`): none
  denies module → module, and `internal/authn` appears only as an **allow** (`:93`, `:130`), never
  as a denial. The repo corroborates the owner verbatim at `tools/archtest/arch_test.go:349-353`:
  *"depguard's domain-purity rule denies only bun/chi/viper/zerolog, and the fence sees a permitted
  area."* So `make arch` carries both enforced bullets alone.
- Scope confirmed: four hunks, two files; `CHANGELOG.md` and `docs/architectures/README.md`
  untouched by the fix commit. All three new citations byte-exact, `(Principal, bool)` correct,
  `(see §5)` resolves. **92 citations scanned, 16 byte-checked by hand, zero line-shift damage.**
- `make arch` re-run green by the reviewer.

**Both of the owner's pushbacks this task were right** — the `RootInterface` refusal (E25) and the
count-word. Recorded because it bears on how I weight this owner's future deviations.

## E25 — CONFIRMED by the reviewer, independently. Now a decision for you.
A tree-wide grep for `RootInterface` returns **exactly one hit in the whole repository**:
`.claude/agents/docs-scribe.md:33`. It exists nowhere in the Go tree. Both module constructors
return `(*module.Module, error)` (`modules/identity/module.go:88`, `modules/user/module.go:75`),
and the composition root builds its own verifier (`cmd/api/container.go:95-105`).
**A standing agent definition carries a false convention about constructor shape** — a settled
decision on this effort. C1 refused to write it and documented what exists instead; the reviewer
confirms that was right. Pre-existing at base `2dd621c`, not attributable to #43.
**Ask:** correct `.claude/agents/docs-scribe.md:33`. I do not edit agent definitions.

## T4 — APPROVE-WITH-NOTES. No must-fix. One comment amendment in flight, then merge.
The reviewer verified every functional claim independently and went beyond the agent: it **proved
depguard actually executes** by planting deliberate violations (both `domain-purity` and
`platform-independence` fired). **depguard has now run in CI for the first time in this repo's
history** — the authn/httpx fence's editor-time half is live, and B4-class proofs can rely on it.
Scope confirmed: 2 files, merge-base exactly `2dd621c`, net **+2 assertions**, no nolint,
`.golangci.yml` byte-identical to base, genuine pin at `ci.yml:276`. Both deviations allowed.

**Amendment LANDED as `799c861` and verified by me, not taken on report:** `git diff 133f183 origin/ci/T4-golangci-lint-v2` is **one file, +4/-3, every changed line inside the `#` comment block** — no workflow logic, no Go file. The comments at `ci.yml:268,271` no longer assert the old failure was silent. The agent rewrapped rather than pasting the reviewer's literal text, and found a **second** "silent" in the PR body I had not spotted. Three other uses of "silent" elsewhere in `ci.yml` are pre-existing, about unrelated jobs, and correct in context — left alone. **#42 is ready for your merge.**

## LINT — I HAD THIS WRONG, AND THE AGENT CORRECTED ME
I recorded that `latest` produced a **silent** v1 pin. **False.** Main @ `2dd621c`, run
`31355178976`, job `93353420746`: conclusion **failure**, 12 seconds, and it announced itself —
`Installing golangci-lint binary v1.64.8...`, then
`Error: can't load config: the Go language version (go1.24) used to build golangci-lint is lower
than the targeted Go version (1.25.12)`, `exit with code 3`, `Ran golangci-lint in 89ms`.
**It was a loud non-result, not a silent green.** That is a different failure of process: the job
was red and nobody acted on it, for an unknown number of runs. Raised by T4's agent against my
dispatch's premise, confirmed independently by the reviewer from the run logs.
**The stacking is confirmed too:** it died at config load on the Go-version check, so the
`version: "2"` parse error was **never reached** — cause 2 is latent, not co-operative.

## GATE-INTEGRITY — PROMOTED OUT OF PROVISIONAL, AND NARROWED
PROCESS-2 demanded a second toolchain or CI before recording a mechanism as general. The reviewer
delivered something better than a second measurement: **a negative control plus a stdlib root
cause.**
- **Warm cache, cross-drive** (`C:` worktree, `D:` repo): `0 issues` — false green, first run.
- **NEGATIVE CONTROL — warm cache, same drive:** findings **survive**, 2 errcheck, rc=1.
- **Root cause, machine-independent:** `filepath.Rel` errors only when two paths have different
  volume names. golangci-lint caches issues with absolute paths, relativises them to the current
  basepath, and **drops** any issue whose relativisation errors — warning as it goes, which is why
  the tool named the vanishing files itself.
**Narrowed claim, now recorded as confirmed:** the false green requires a warm
`GOLANGCI_LINT_CACHE` shared across trees on **different Windows drive letters**. It **cannot occur
on the Linux CI runner** — single root, `Rel` always succeeds. This is an explained causal chain
with a passing negative control, not the unexplained one-machine correlation PROCESS-2 was written
about. **Promoting it.**
**This effort's own topology is the hazard:** every agent worktree lives on the `C:` temp drive
while the repo is on `D:` — precisely the cross-volume topology that produces the false green.
Live and recurring, not incidental. Both caches stay isolated in every dispatch.

**New second-order hazard from the control:** same-drive worktrees do not go green but report
findings **attributed to the other tree's path**, so a developer can be shown findings that do not
exist in the tree in front of them.

## E7 — MY RECORDED CAUSE WAS WRONG; THE FLOOR IS UNSATISFIABLE AS THE TREE STANDS
I let "covered by integration tests, needs a database" stand as the reason `auditlog` reports 0.0%.
**All three parts of that are false**, and it would have misdirected whoever fixes E7:
1. `modules/identity/infrastructure/auditlog/` contains **exactly one file, `auditlog.go`, and no
   `_test.go` files at all.** 0.0% is not a database artefact — **the package has no tests.**
   `-with-database` cannot change it.
2. `tools/coverage/policy.json:51-54` — `requires_database` lists only
   `modules/*/infrastructure/persistence/repository` and `tools/schemacheck`. **`auditlog` is not on
   it** and is never exempted, with or without the flag.
3. CI's own log, on the Linux runner **with a live Postgres**, prints
   `.../auditlog  coverage: 0.0% of statements` — a bare line, no `ok`, i.e. no test binary.
**E7's fix is tests for `auditlog`, or a reasoned policy change. Never a database, never a lowered
floor.** The floor is unsatisfiable as the tree stands.

## E18 — CONFIRMED WITH FILE:LINE: THE COVERAGE GATE HAS NEVER EXECUTED
`.github/workflows/ci.yml:122-123` (`go run ./tools/coverage ... -with-database`) is **unreachable**
while `.github/workflows/ci.yml:91-92` (`go test -race`) exits 1 on E8's two `internal/db`
failures. The coverage gate is **dead in CI today**, independent of E7. E8 is the keystone: clear
it and two latent floor failures surface at once.

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
**C1 adds three, all in `authn-spi-impact-analysis.md`, found by citation-checking it against
merged code:** §7 names two permitted importers where enforcement has four areas plus two layer
denials, and says depguard would "allow" when it in fact never denies (asymmetric, BL43); §8.1's
"Expected finding" is false on both counts against `9e50896`'s goldens; §8.2's example body omits
both `request_id` (E5) and the real response's `X-Content-Type-Options: nosniff`.
**C1's proposal, which I am forwarding not deciding:** a one-line struck-through correction in
each, because agents are still reading that document as a source and re-deriving the errors from
it — C1 is the second agent to have to be warned off §7 by dispatch. **Ask:** authorise a doc-fix
task, or leave the board as the record?
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

## RESOLVED THIS SESSION — `secret_test.go` was never a mystery
The third `path_relativity` warning named `internal/platform/secret/secret_test.go`, and I left it
open as "a stale finding from some other tree state". **It is neither stale nor hidden.**
`internal/platform/secret/secret_test.go:26` carries `//nolint:staticcheck // S1025: exercising the
%s verb is the test`, with a four-line justification. golangci-lint's `nolint` processor runs
**after** cache restore and path relativity, so the raw S1025 issue is in the cached set — which is
why the warning named the file — and is suppressed before reporting. Cold-cache run of that
package: `0 issues`. The suppression is documented and correct: taking staticcheck's advice would
delete the assertion the test exists to make. **Nothing to chase, nothing to fix.**

# BACKLOG
- **BL53** **My own board carried a fragile citation, and a reviewer caught it.** This file cites
  `BOILERPLATE.md:279` in three places (E16's provenance and E21). All still resolve — C1's insert
  is one contiguous block at line 315+, so `:279` is byte-identical before and after — but the next
  `BOILERPLATE.md` edit *above* line 315 breaks E16 and E21's provenance chain **silently**.
  **Mitigated now:** each of the three citations carries the quoted target text alongside the line
  number, so a shift is detectable rather than invisible. **Standing correction:** cite actively
  edited recipe files (`BOILERPLATE.md`, `SWAGGER.md`) by heading or quoted text, not by line alone.
  Line citations into *code* stay — code citations are checked by reviewers every task.
- **BL54** `docs/architectures/README.md`'s 11 ADR links are percent-encoded (`adr/NNN%20-%20….md`).
  They resolve in a renderer and are pre-existing. Recorded **so the next reviewer does not
  re-chase them** — a link checker will flag them every time.
- **BL48** **`make lint` IS FAIL-OPEN.** `Makefile:100-104`: if `golangci-lint` is not on `PATH`,
  the `else` branch echoes a message and the target **exits 0**. Guardrail 8's `make lint` is
  therefore a **no-op pass** on any machine without the binary. Together with the cross-volume
  cache false green, that is **two independent ways `make lint` reports success while checking
  nothing** — one local, one cross-tree. Every historical "lint clean" on this board is worth
  exactly as much as the evidence that the linter ran. Pre-existing; not T4's file list.
- **BL49** `Makefile:103`'s install hint is the **v1 module path**
  (`.../golangci-lint/cmd/golangci-lint@latest`). Following it now installs a v1 that cannot parse
  `version: "2"` and cannot run against go1.25 — i.e. the hint reproduces the exact failure T4 just
  fixed in CI. Correct is the `/v2/` path pinned to `v2.12.2`.
- **BL50** `make lint` pins no version, so a locally newer v2 can report findings CI does not.
- **BL51** `internal/db/schema_test.go` **never runs anywhere.** `testDSN()` (`:99-107`) reads
  `APP_DB_*` (default `postgres`/`postgres`/`kopiochi`); the build job sets only
  `TEST_DATABASE_URL` against a `kopiochi`/`kopiochi_test` service (`ci.yml:62-63`), and the
  `APP_DB_*` block lives in the **migrations** job (`ci.yml:144-151`), which does not run this test.
  So it skips in CI and locally. This is the mechanism behind BL1's dead-duplicate observation.
  Consequence accepted at T4's merge: its `Close()` fix runs on the skip path, its `DROP SCHEMA`
  fix is compile-verified only and **executes nowhere**. Wire it or delete it — your call.
- **BL52** `golangci-lint-action@v8` is a **floating major tag** while the linter itself is pinned.
  Consistent with the repo's existing posture (`ci.yml` already notes no action is SHA-pinned and
  that pinning is Phase 5 hygiene). Recorded, not a finding against T4.
- **BL46** `SWAGGER.md:280,365,485` still document 401 as `@Failure 401 {object}
  map[string]string` — stale against problem+json since Phase A. Found by C1, out of its file
  list. Note for D10-swagger, which touches that surface.
- **BL47** `modules/identity/transport/helpers.go:9-14` describes itself as duplicating
  `internal/infrastructure/http/handlers`, a package deleted in 3.6b. A stale doc comment in the
  file that BL45 says took live copies of `modules/user`'s writers — check both together.
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

- **BL38** `modules/user/domain/service.go` is **dead** — nothing implements or references it,
  and its signatures are incompatible with `application.Service`. Cheapest handling is
  deletion during E16-2 rather than porting it to uuid (found by E16-P).
- **BL39** `modules/*/transport` has **no coverage entry of any kind** — no floor, no
  baseline, not exempt, not in `requires_database`. `matchPattern` (`tools/coverage/main.go:292-322`)
  is **exact-arity** without a trailing `/...`, so `modules/*/domain` can never match a
  4-segment path, and both check branches are `ok`-guarded — a package with no entry reports
  any figure, **including 0%, and passes**. `modules/user/transport` measures 100% and
  contributes nothing either way. Extends BL19; a `modules/*/transport` floor *is* expressible
  today, it simply is not written. Do it after E16-3, not before — the refactor will move the number.
- **BL40** `internal/metrics`' docs and tests are premised on **enumerable numeric user ids**
  (`SERVER.md:279-281`: "a crawler walking `/api/v1/users/1 … /9999999`"). A uuid path defuses
  the worked example. Docs work, folded into E16-4.
- **BL41** Two stale strings found in passing by E16-P, deliberately not fixed:
  `modules/identity/infrastructure/persistence/models/auth_models.go:14` says it "maps to the
  `users` table" while its tag says `auth_users`; `scripts/init.sh:171-172` still removes
  `internal/domain/user`, a path deleted in 3.6b.

- **BL42** The domain rule is **documented as an allowlist and implemented as a denylist**.
  `tools/archtest/arch_test.go:335-339` says the domain layer may use "the standard library and
  `internal/platform`, and nothing else", but the mechanism is a fixed forbidden map — a domain
  package importing an unlisted third-party module passes today. Pre-existing; T2 closed the
  specific `internal/authn` hole correctly. Fixing the general shape (invert to an allowlist, or
  reword the docstring to promise only what it enforces) is a decision, not a chore.
- **BL43** **depguard now lags the arch test by one `make arch`.** `.golangci.yml`'s
  `domain-purity` and `application-purity` still deny only bun, chi, viper, zerolog and pgx —
  not `internal/authn` — so an author gets editor-time feedback for the ORM but not for the
  authentication contract. T2 was right not to touch it (not in its file list). The
  file-glob vs import-graph split is deliberate per the config's own header, so aligning them is
  a judgement call.
- **BL44** The shipped T2 docstring (`36784b6`) says depguard's domain-purity "denies only
  bun/chi/viper/zerolog"; the reviewer enumerated **five** — pgx is also denied. Harmless
  imprecision in a comment, in the enforcement layer where comments have misled before (it was a
  fence docstring that made the B4 gap look covered). Fold into any future edit of that file.

- **BL45** `modules/user/transport/helpers.go` sets `Content-Type: application/json`
  **unconditionally before `WriteHeader`**, so `DELETE /api/v1/users/{id}` answers **204 with a
  representation header and an empty body**. RFC 9110 says a 204 has no content. Found by
  E16-P's goldens — the probe photographing an oddity nobody had noticed, which is what a probe
  is for. `modules/identity/transport/helpers.go` reportedly took **live copies** of these
  writers; check it for the same shape before anyone fixes one of them.

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

---

# T2 — the verdict transferred, and why that is sound

The review was dispatched against **#39** (`00fa2e5`), which I then closed as a duplicate of
what shipped in **#35** (`36784b6`). The **APPROVE** transfers, and this is the evidence rather
than an assumption: `tools/archtest/arch_test.go:163` declares
`const authnPkg = internalPrefix + "authn"`, so the two versions add **the same map key with the
same value to the same two maps**. Diffing both files with comments stripped leaves exactly two
differing lines — `authnPkg:` versus `internalPrefix + "authn":` — i.e. two spellings of one
constant expression. Nothing else differs outside comments.

What the reviewer established, which now stands as the record for `36784b6`:
- **The gap was real, reproduced against the base tree**, not inferred: probes in
  `modules/user/domain` and `modules/user/application` left `make arch` **green** at `6fc7592`.
- **The rule bites, and the prefix match is exactly right** — it also catches
  `internal/authn/authntest` (the mandatory `/` means a future `internal/authnz` is *not*
  swept in). Verified by execution, and BL35's doubled output accounted for.
- **No legitimate importer is newly denied** — all six `internal/authn` importers enumerated via
  `go list` over test-inclusive imports; none is a domain or application package.
- **The fence was not narrowed:** `authnAreas`, `underArea` and `TestMayImportAuthnSemantics`
  byte-identical to base, and `modules/user/module.go:17,53` confirms the module-root
  `authn.Middleware` that makes the `modules/*` recursion load-bearing.
- **Docstring deviation ACCEPTED** on the reviewed branch, with the reasoning that a bare map
  entry would read as duplication of the fence and invite deletion — the fence's own docstring
  is what made the B4 gap look covered. **Caveat I am recording rather than papering over:** the
  shipped commit carries *different* docstring text, which the reviewer never read. See BL44.

**Not carried forward as work:** N2 (unnormalized `imp` in the layer tests — checked, no gap)
and N5 (the new entries have no pinning test, which is true of every other entry in those maps
and introduces no new asymmetry).

---

# E16-P — APPROVE-WITH-NOTES, and what the review established

**The verdict applies to `main`:** the reviewer confirmed `git diff --stat 1dc6aa7 <main> -- modules/user/transport/`
is empty, so the merged bytes are the reviewed bytes (PROCESS-5 notwithstanding).

- **Nothing production changed, structurally.** All seven diff entries are `A` — zero `M`, zero
  `D` — so "no test weakened, skipped or deleted" is proven by the shape of the diff rather than
  by inspection. No `.golangci.yml`, no `policy.json`, no route table, no `cmd/api`.
- **The goldens are honest recordings, not aspirations:** regenerated with `-update`, then
  `git status --porcelain` came back empty.
- **The `-update` absorption claim is TRUE, proven by simulation.** The reviewer patched
  `GetUser()` to answer the not-found response for every id but 1 — a stand-in for the ownership
  check — and ran the suite **with `-update`**. `TestCurrentUserRouteShapes` silently
  re-recorded, as a camera should; **`TestIDORIsPresentToday` failed both subtests anyway**,
  because it never touches `*updateGolden`. The fix cannot land unnoticed.
- **The byte-identity obligation is genuinely checkable:** under the simulated fix `status`,
  `content_type` and `body` all become equal between `get_cross_user` and `get_not_found`, and
  the only remaining difference is `path` — a property of the request, not of the answer.
- **All four deviations ACCEPTED**, the wider golden schema most emphatically: without
  `store_after`, `delete_cross_user`'s `204 ""` is indistinguishable from a legitimate
  self-delete, and *which row is gone* is the entire point of that case.
- **Fixture honesty upheld**, and better than required: `TestIDORIsPresentToday`'s first subtest
  asserts the cross-user body equals the owner's body, which is true **independent of any
  fixture convention**. That is the honest way to state an IDOR in a codebase that cannot
  express ownership.
- **Arch clean on current `main`** with T2's fence merged — the transitive `testsupport → authn`
  edge does not trip the new domain/application denials.
- **Security:** no key material in the goldens, a consequence of choosing `testsupport.FakeAuth`
  over minting real RS256 tokens.

**Minor, recorded not actioned:** the agent cited `:65-83` for the fixture-convention comment;
it is at `:68-83` (E1's citation discipline extends to line numbers). `get_owner` is called "the
control" at `:145`, which oversells it — with no link between the two id spaces it traverses the
same code as `get_cross_user` and proves nothing about ownership *today*; its value is entirely
prospective.

**Lint tooling — a real improvement in what this board can claim.** The reviewer's local
`golangci-lint` is **v2.12.2 built with go1.25.12**, which parses the v2 config, so the tree has
now actually been linted end to end. **BL1's two findings are therefore confirmed *complete*,
not merely *reported*.** CI's v1.64.8 still cannot start.
