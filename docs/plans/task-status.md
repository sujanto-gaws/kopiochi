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
| T4  | platform-engineer | **merged** | #42 (`799c861`) | **APPROVE-WITH-NOTES**; amendment landed + verified |
| T5  | platform-engineer | **merged** | #36 | not gated by me — landed from a prior session |
| C1  | docs-scribe | **merged** | #43 (`66b69e1`) | **APPROVE-WITH-NOTES** after BLOCK #1 fixed |
| **T6** | *(in-session, ungated)* | **merged** | #44 (`cb020d6`) | **none — see PROCESS-7** |
| **T7** | *(in-session, ungated)* | **merged** | #45 (`cf7a4cb`) | **none — see PROCESS-7** |
| **E8-FIX** | *(in-session, ungated)* | **merged** | #46 (`bc0cc2f`) | **none — see PROCESS-7** |
| **E18-FIX** | *(in-session, ungated)* | **merged** | #47 (`7349edd`) | **none — see PROCESS-7** |
| **BL34b** | *(in-session, ungated)* | **merged** | #48 | **none — see PROCESS-7** |
| **GITIGNORE** | *(in-session, ungated)* | **merged** | #49 | **none — see PROCESS-7** |
| **E11-FIX** | *(in-session, ungated)* | **merged** | #51 (`8f82381`) | **none — see PROCESS-7** |
| **E13/E14-FIX** | *(in-session, ungated)* | **merged** | #53 (`0d0b02d`) | **none — see PROCESS-7** |
| **E15-PRE** | *(in-session, ungated)* | **merged** | #55 (`ee9bbd9`) | **none — see PROCESS-7** |
| **E16-P2** | *(in-session, ungated)* | **merged** | #58 (`49ee321`) | **none — see PROCESS-7** |
| **E21-1** | *(in-session, ungated)* | **in review** | **#61** (`af83214`) | **none** |
| **E9b/E9c-FIX** | *(in-session, ungated)* | **in review** | **#63** (`aa784a1`) | **none** |
| D5  | platform-engineer | **UNBLOCKED 2026-08-29** — E11, E13, E14 all answered. Scope grew: `sender/inapp.go`, and its file list must say `domain/message.go`, not `application/ports.go` | - | - |
| D6  | platform-engineer | **UNBLOCKED 2026-08-29** — E9b/E9c answered and implemented (#63); E12 answered and additive, not a dependency | - | - |
| D7  | transport-engineer | **UNBLOCKED** — needs D6 | - | - |
| D8  | platform-engineer | **UNBLOCKED 2026-08-29** — E15 answered; precursor #55. Port declared by the sender, adapter in `cmd/api` | - | - |
| D9a/D9b, D10 | domain / platform | pending | - | - |
| E16-P | test-guardian | **merged** | #37 (`1dc6aa7`) | **APPROVE-WITH-NOTES** |
| E16-1 | persistence-engineer | **BLOCKED — E22 only** (E20 answered, E19 ruled 2026-08-29) | - | - |
| E16-2 | domain-engineer | **UNBLOCKED 2026-08-29** (E24 = option b). **Scope changed:** `CreateUserRequest`/`UpdateUserRequest` are **deleted, not emptied**; `UserResponse` becomes `{id uuid, created_at, updated_at}`; `ID int64`→`uuid.UUID`; drop the `id <= 0` guard | - | - |
| E16-3 | transport-engineer | **UNBLOCKED 2026-08-29** (E24 = option b). **Scope SHRANK:** the route table becomes `GET /api/v1/users/me` alone — POST/PUT/DELETE are removed, so there is no `{id}` param, no `strconv.ParseInt` ×3, no `@Param id path int` ×3, and **the byte-identity obligation is moot on verbs that no longer exist**. The handler reads the Principal | - | - |
| E16-4 | docs-scribe | pending — needs E16-3 merged; carries **E19**, BL40 | - | - |
| E16-5 | platform-engineer | **part 1 merged (#61)**; part 2 **BLOCKED — no authorization primitive exists**, and needs E16-3 first | - | - |

**Phase A and Phase B are complete on `main`.** `main` is **`5030e7f`**.

**2026-08-29 — CI IS GREEN. ALL SEVEN JOBS.** For the first time in this repository's
history. #42 (T4) made the linter run at all; #44 (T6) cleared six stdlib advisories;
#45 (T7) stopped the test binaries colliding in one database; #46 fixed E8, the two
migration tests that had been killing the `build` job before it reached its own gates;
#47 cleared E18's two floors; #48 and #49 closed BL34's second half and the `.gitignore`
fusion. **`main` carries all nine. Open: #50 (this board) and #51 (E11), both 7/7 green.**

**Two CI steps executed for the first time ever**, both previously `skipped` because the
job died at `go test -race`: **`architecture rules` — and it PASSED**, so B4's and T2's
fences had never actually been enforced by CI until 2026-08-29 and they hold; and
**`coverage floors and ratchet`** — see E18.

`gh pr merge` and pushes to `main` are blocked by the sandbox classifier, so a human
merges; branch pushes and PR creation work.

## WAITING ON YOU — one answer
Nothing below is mine to decide. **E22 is now the ONLY thing blocking this board.** Every other
escalation has been answered, and every task that needed no answer from you has been done and
merged. **Phase D is fully unblocked: D5, D6 and D8 are all dispatchable.**

**E22 is the ONLY thing still blocking the E16 chain**, and it is the one item on this board
that cannot be answered from the repository at all: it is a fact about deployed environments,
settled by `make migrate-status`. Everything else in the chain is now scoped and dispatchable.
| Ask | Blocks | One-line question |
| --- | --- | --- |
| **E22** | E16-1, and the whole E16 chain behind it | Does **any** environment have rows in `users`? (`make migrate-status`) |
**Phase D's dependency wall is down.** E11 (#51), E13/E14 (#53) and E15 (#55 + decision) are
all answered, so **D5 and D8 are both dispatchable now**. **D6 alone still waits**, on
E9b/E9c/E12 — and those are the only Phase D asks left. Nothing in Phase D is blocked on
anything the human has not already been asked.

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
**STILL OWED as of 2026-08-29, and NOT discharged by the green gate.** The gate reports
*"all floors met, no regressions"*, which is a statement about floors and the ratchet — **not**
that the baselines are current. Not verified this session.

## Merge log
#15 `d63200b` rename · #16 `f594ebc` A1+A2 · #17 `d74ec67` A3 · #18 `0c0a1cf` T1 ·
#20 `a475151` A4 · #22 `21c94bb` D1 · #23 `68402ec` D2+D3 · #25 E9a ·
#26 `5481dbd` **B1 only** (see PROCESS-1) · #29 `73409cc` D4 ·
#30 `42b7a98` **B2+B3 recovered** · #31 `ad5ba26` B4 ·
board: #19/#21/#24/#27/#28/#32 → `ca397f1` · #41 board
**2026-08-29:** #42 `799c861` T4 · #43 `66b69e1` C1 · #44 `cb020d6` **T6** ·
#45 `cf7a4cb` **T7** · #46 `bc0cc2f` **E8** · #47 `7349edd` **E18+E7**
#48 `2d8d529` **BL34b** · #49 `979df0f` **GITIGNORE** · #50 board · #51 `8f82381` **E11** ·
#52 board · #53 `0d0b02d` **E13+E14** · #54 board · #55 `ee9bbd9` **E15 precursor** ·
#56 board · #57 board · *(open: #58 E16-P2, #59 board)*

## PROCESS-7 — six changes merged with NO arch-reviewer verdict
T6 (#44), T7 (#45), E8-FIX (#46), E18-FIX (#47), BL34b (#48), GITIGNORE (#49), E11-FIX (#51),
four board PRs, E13/E14-FIX (#53) and the E15 precursor (#55) were produced without passing the
gate every earlier task passed.

**#51 and #53 are the two that most warrant a real review.** They are the only ones that change
production Go, and both act on decisions that were the human's to make and were delegated:
#51 moves two types between layers, and #53 adds a refusal to a public method (`Enqueue` now
errors where it previously returned nil) and a field to a domain type. **#53 also changes an
existing test's setup** — justified in its entry under E13, but exactly the kind of change a
gate exists to look at. They are
test/CI/config only — **no production Go file was modified by any of them** — and each
carries its own in-PR verification (real Postgres 16, mutation tests on #46, the actual
coverage gate on #47). But the gate is the gate, and the board must record that these six
are **unreviewed** rather than let the merge log imply otherwise.
**Standing correction:** work done directly in the lead's session is still work, and is
not exempt from the gate because it was convenient.

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

# ESCALATIONS — OPEN (3)
**Was 17. Fourteen were resolved on 2026-08-29** — E7, E8, E9b, E9c, E11, E12, E13, E14, E15,
E18, E19, E21, E23 and E24 — each now headed RESOLVED or ANSWERED below.

**What remains: E22** (the human's, and the only board item unanswerable from the repository),
**E10** and **E17**.

**One NEW question surfaced and deliberately not numbered by me: this repository has no
authorization primitive at all.** It is why the IDOR was expressible, it blocks E21 part 2, and
it is framed under E21 below rather than opened as an escalation, because naming it is yours.

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
- **E16-3** transport — **SUPERSEDED 2026-08-29 by E24 = option (b), and it SHRANK.** The route
  table becomes **`GET /api/v1/users/me` alone**; POST, PUT and DELETE are removed. So: no
  `{id}` path param, no `strconv.ParseInt` ×3, no `@Param id path int` ×3, no ownership check to
  write, and **the byte-identity obligation is moot — it cannot apply to verbs that no longer
  exist.** The handler reads the caller from the Principal. *(Original scope, kept for the
  record: `strconv.ParseInt` ×3 → uuid parse, `@Param id path int` ×3, the ownership check
  itself, and byte-identity of the cross-user response against the genuinely-not-found one.)*
- **E16-3 also owns the goldens, which is easy to miss.** Removing the routes **breaks
  `TestCurrentUserRouteShapes`** — five of its eight cases drive verbs that will 404 or 405.
  E16-P and E16-P2's files stay as the historical record of the defect, and the test is rewritten
  to assert the surface is **gone** rather than guarded. **The six
  `// E16: invert to require.Equal when the ownership check lands` markers now mean something
  different from what they say**: there is no ownership check on those verbs, so each is
  "assert the route no longer exists, or delete the case". Whoever executes E16-3 should read
  this line before greping for them.
- **E16-4** docs — `BOILERPLATE.md`, `SWAGGER.md`, `README.md`, `MIGRATIONS.md` examples,
  plus **E19**'s contradiction and BL40. **Now also carries the exemplar gap:** `BOILERPLATE.md:279`
  points adopters at `modules/user/transport/user.go` as the worked CRUD example, and under E24
  that file stops being one. State it honestly rather than leaving the pointer dangling.

## E23 — **APPROVED 2026-08-29 (human, delegated). E16-P2 implemented in #58 (`49ee321`).**
Every claim in the entry below was verified before approving: `routeCases()` had `get_not_found`
and no counterpart for the other two verbs, while `user.go:145,179` maps `domain.ErrUserNotFound`
to 404 on both routes and the recorded cross-user answers are 200 and 204. **Without these,
a fix could have closed the read oracle, left PUT and DELETE announcing which ids exist, and
passed.**

**One refinement added to the proposal.** Each probe mirrors its cross-user counterpart in every
field except the id — same method, same caller, and **for PUT the same request body**. That makes
E16-3's acceptance **mechanically checkable** rather than a judgement call: once the ownership
check lands, `put_cross_user.json` and `put_not_found.json` must differ in **`path` and nothing
else**, and likewise for delete. With mismatched bodies the two files would differ in two ways
and the argument would move to which difference mattered.

**What the goldens record today** — four differing fields each, and after E16-3 there must be one:

    put     path · status (200 vs 404) · body · store_after
    delete  path · status (204 vs 404) · body · store_after

**A correction to this entry: the side-effect leg was NOT a gap.** E23 does not mention
`StoreAfter`, and a reader could conclude nothing guards it. `routeGolden` already carries
`StoreAfter: repo.snapshot()`, and its doc comment says why better than a summary could — *"for
PUT and DELETE the interesting damage is not in the response at all — a 204 looks innocent until
you notice which row is gone."* E16-P had already provided for it. The new assertions **use** it
rather than duplicating it: a refused write that still wrote would be byte-identical to a refusal
and would still have destroyed the row.

**N2 hardening landed as specified**: six `// E16: invert to require.Equal when the ownership
check lands` markers, on every inverted assertion including the two that predate E16-P2, so the
refactor's checklist greps rather than relying on a comment being read.

**Verified the recording step is real, not a silent `-update`:** both new cases fail with *"run
the test with -update to create it"* before their goldens exist — which is precisely the failure
mode this file's own header warns about (*"rather than run `-update`, see green, and ship"*).

**Currency fix to this entry:** it says E16-1 is "paused on E20/E22". **E20 was answered
2026-08-10** and **E19 was ruled 2026-08-29** — **E16-1 is blocked on E22 alone.**

### E23 (original entry, kept) — the enumeration oracle is captured for GET only; PUT and DELETE are unguarded
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

## E19 — **ANSWERED 2026-08-29 (human, delegated). RULING: the decision supersedes `:249-251`.**
**E16-1's last documentary objection is gone. It is now blocked on E22 alone.**

**Why the ruling goes this way — three reasons, and the third is the strongest.**
1. **It is not a rule.** The sentence sits inside **Path B** of a conditional "Recovering from
   the current state" section — advice for one branch of a question, not an architectural
   commitment.
2. **The section defers to a later decision by its own terms**, closing with *"Confirm which
   applies before writing any new migration."* **E16-ARCH is that confirmation.** The doc asked
   to be superseded.
3. **The same file already states the constraint that forces the reshape** — `:220-221`, that
   the `BIGSERIAL`/uuid mismatch means *"any table backing them must use uuid to match"*. The
   file contradicts itself, and the half agreeing with the decision is the half stating the
   technical constraint. **A reviewer citing `:249` should be pointed at `:220` in the same
   file.**

**E19 under-reports the damage, and E16-4 must repair more than the paragraph.**
**Path A — the branch the doc calls "most likely" — says "Delete both files and start the
module chains at `0001`."** That directly contradicts **ADR-010's binding immutability rule**,
which E16-ARCH already verified is the actual practice (`git log --diff-filter=MRD --
migrations/` returns empty; no migration has ever been modified, renamed or deleted).
Rewriting only `:249-251` would correct the sentence E19 spotted and **leave an instruction to
delete applied migration files standing four lines above it** — the more dangerous of the two.
**E16-4 rewrites the whole "Recovering from the current state" section, not the paragraph.**

**Two further factual errors in that section, verified while ruling on it:**
- `:249` cites `internal/infrastructure/persistence/models/user.go` as a live bun model backing
  `users`. **That file does not exist** — Phase 3.6b moved it. The sentence's supporting
  evidence is gone independently of the decision.
- `:230`'s index SQL does not match what shipped: it shows
  `idx_auth_users_email_lower ... WHERE deleted_at IS NULL`, but `00007` created it with **no**
  `WHERE` clause, and **`deleted_at` exists in no migration in this repo** — there is no
  soft-delete column at all, so the "Soft delete" convention at `:217` describes something the
  schema does not implement.

Neither blocks E16-1; both belong to E16-4's scope, and together they are why the section needs
rewriting rather than patching.

**Not done here, deliberately:** the doc rewrite itself is docs-scribe's file set and should
land **with** the migration it documents, not ahead of it. E16-4 carries it.

### E19 (original entry, kept) — a MERGED doc contradicts the settled decision
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

## E21 — **ANSWERED 2026-08-29 (human, delegated). SPLIT: part 1 in #61; part 2 after E16-3 and BLOCKED.**
**Not a non-goal, and not a rewrite now.**

**The fact that reframes it: the generator is ALREADY broken, and documented as broken in three
places** — `current-state.md:260`, `remediation-plan.md:72`, `BOILERPLATE.md:195` — because it
writes to `internal/infrastructure/http/routes/routes.go`, which Phase 1.5 deleted. **So the
amplification this entry fears is latent, not live** (this board's own E7/E18 distinction):
nobody can currently generate a working module.

**But latent here is worse than inert, and that is what decided the answer.** The commit that
revives `make generate` is **exactly** the commit that arms the amplification, and nothing
connected the two. Whoever fixes it will be looking at deleted routing paths, not at
`PrimaryKey = {ID, int64, id}` or `strconv.ParseInt` — they fix the routes, the generator starts
working, and the IDOR shape goes live in the same change, **silently**. A comment cannot prevent
that; the tool has to stop.

**Part 1 — DONE, #61 (`7723cba`).** The generator exits 2 with an explanation instead of
generating, and the two paths that swallowed failure are fatal. **Verified in code, not taken on
report:** `main.go:267` and `:274` printed `⚠ Warning` and **continued**, so a run that left the
tree non-compiling still exited 0 and printed ✓ lines. Both are now fatal, so removing the
refusal does not bring the silent-success defect back with it. **No override flag** — an escape
hatch would be used, and the point is that reviving the tool requires editing the file and
therefore reading why it was disabled. No CI impact: nothing in `ci.yml` runs `make generate`.

**Part 2 — after E16-3, and BLOCKED on something bigger than E21.** Two reasons it cannot be
done now:
- **There is nothing to mirror.** The generator's shape follows `modules/user`, and under
  E24 = option (b) that module stops being a CRUD exemplar at all. `/me` is not the answer for a
  generic entity either, which is not 1:1 with a user. The safe shape for a generated resource is
  an open question until E16-3 settles it.
- **There is no authorization primitive to emit.** E16 established this repository has **zero**
  `RequireRole`/`RequirePermission`/`Authorize` matches — no authz layer exists anywhere. Writing
  the template now would mean **inventing the authorization model inside a code generator**,
  which is the worst possible place to invent it.

**⚠ THE SECOND POINT IS A NEW ESCALATION IN ITS OWN RIGHT.** "This repository has no
authorization primitive" is bigger than E21, bigger than E16-3, and is the reason the IDOR was
expressible at all. E16-3 closes the surface for one module by deleting it; nothing yet decides
what an authorized route looks like in this codebase. **Recorded here rather than opened as a
numbered escalation only because it deserves the human's framing, not mine.**

**When part 2 runs:** default the PK to uuid (`auth_users` and `notifications` already do; only
the legacy `users`/`products` use `BIGSERIAL`), drop `strconv.ParseInt` and `@Param id path int`
with it, emit handlers that **fail closed** — 501 and an explicit TODO — rather than working
CRUD with no authorization, and repoint the route/wiring updates at `httpx.Mount` and
`cmd/api/container.go`. That list is duplicated into the tool's own refusal message, so it
reaches someone who never opens this board.

**Owner:** platform-engineer (`cmd/**`), as this entry proposed. **Part 1 did not serialise
against the `cmd/api` single-writer rule** — it touches only `cmd/generator`.

### E21 (original entry, kept) — `cmd/generator` reproduces the defect into every future module
`cmd/generator/main.go:202-206` hardcodes `PrimaryKey = {ID, int64, id}` with nothing
overriding it; `:714` `if id <= 0`; `:910` `bun:"id,pk,autoincrement"`; `:979/1051/1094`
`@Param id path int`; `:988/1061/1103` `strconv.ParseInt`. `BOILERPLATE.md:279` (quote: "`modules/user/transport/user.go` or") points
adopters at `modules/user/transport/user.go` as the worked example.
**Fixing `modules/user` without the generator means the next generated module ships the same
IDOR shape** — and E16's severity rests on exactly that amplification.
This is a **new task, outside every existing task's file list** ⇒ scope escalation. Natural
owner is platform-engineer (`cmd/**`), which then serialises against the `cmd/api`
single-writer rule. **Ask:** in scope now, after E16-3, or a recorded non-goal?

## E24 — **ANSWERED 2026-08-29 (human, delegated). OPTION (b), NARROWED: `GET /api/v1/users/me` ONLY.**
**POST, PUT and DELETE are all removed.** E16-2 and E16-3 are unblocked, and **E16-3 gets
smaller, not larger.**

**The argument that decides it: you cannot have an IDOR on a route that takes no id.**
E16-3 was scoped as a guard bolted onto three vulnerable routes, with E16-P and E16-P2 recording
the oracle so the guard could be verified byte-for-byte. **Under (b) those routes do not exist.**
The enumeration oracle disappears by construction rather than by careful assertion, and E16's
second leg — unrestricted `POST /users` behind mere authentication — disappears with it. That is
strictly stronger than any amount of correct 404-matching.

**The fact underneath, verified against the migrations rather than argued:** after E20 the entity
has **no fields at all**. Every column of `users` already exists in `auth_users` —

    users.name        auth_users.name
    users.email       auth_users.email
    users.created_at  auth_users.created_at
    users.updated_at  auth_users.updated_at
    users.id          becomes auth_users.id under E16-ARCH

— so the post-E20 profile is `(id, created_at, updated_at)` with the timestamps duplicating
identity's own. **A write API over an entity with no writable field is not an API.** Option (a)
says so itself — *"a PUT with no writable field is not a route, it is a 200 that lies"* — and
then keeps `POST` anyway; the same objection applies to a `POST` whose body must be empty.

**Why not (c).** It requires inventing product requirements — which fields — and puts a schema
design in front of a confirmed IDOR. If profile fields are wanted later they land in **this
module's own table**, and that is the real reason to keep the table while it is empty: **R2
forbids `modules/user` reading `auth_users`, so the table is a placeholder for a module boundary,
not duplicated data.** Recorded explicitly so nobody later reads "empty table" as "delete the
module".

**Something E24 did not examine: DELETE does not survive either.** Both (a) and (b) keep it.
Deleting a profile that is 1:1 with an identity leaves a **logged-in user whose profile 404s** —
an incoherent state. Account deletion belongs to identity, which owns the credential. Hence
(b) **narrowed** to a single route.

**What it costs, stated plainly.** The repo loses its CRUD worked example: `BOILERPLATE.md:279`
points adopters at `modules/user/transport/user.go`, and until D7 ships notification's transport
there is no full-CRUD replacement. The objection inverts, though — that exemplar currently
**propagates an IDOR into every module copied from it** (E21), and `cmd/generator` reproduces its
shape mechanically. An ownership-scoped `/me` route is a better thing to be copied. **The doc gap
is real and belongs in E16-4's scope as an honest statement, not as a reason to preserve a broken
example.**

**THE ASSUMPTION THIS RESTS ON, and the one thing that would overturn it:** that **no external
client depends on `POST`/`PUT`/`DELETE /api/v1/users`.** E16-P settled the internal picture — the
profile's `name`/`email` have exactly one consumer, the CRUD JSON echoing back what was posted,
and `GetUserByEmail` is unreachable over HTTP — but an out-of-repo consumer is not visible from
here. **Removing routes is a breaking API change.** If such a client exists, this answer becomes
option (a) as a compatibility shim with its degenerate-CRUD cost accepted knowingly. Say so and
it is one board edit to reverse.

### E24 (original entry, kept) — dropping the columns empties the module's write API
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

## E18 — **RESOLVED 2026-08-29 by #47 (`7349edd`). E7 WITH IT.**
Both floors cleared **by tests, not by moving a floor**:

    modules/identity/infrastructure/persistence/repository   57.1% -> 83.0%   (floor 60%)
    modules/identity/infrastructure/auditlog                  0.0% -> 100.0%  (floor 60%)

The board recorded both numbers from a local profile. **CI now confirms them to the
decimal**, and the gate — executing for the first time ever — reports *"all floors met, no
regressions"*.

**E18's severity was understated here.** The repository's gap was not spread thinly:
**three functions carried ZERO coverage — `FindAny`, `Rotate`, `RevokeFamily` — and together
they are the entire refresh-token reuse-detection path.** Rotation alone does not survive
theft; the family sweep is what ends the attacker's session, and none of it was exercised.
Every property is enforced in SQL (a `rowsAffected` check, a family predicate), so no unit
test could have reached it. The sharpest new assertion: after a refused second rotation the
loser's successor must **not exist** — if the `UPDATE` fails after the `INSERT` lands, the
caller just refused walks away with a working token.

**E7's prescription was followed exactly** — *"tests for `auditlog`, or a reasoned policy
change. Never a database, never a lowered floor."* It got tests: 100.0%, asserting the
mapping (a failed login carries the attempted username in `Subject` with **no** `actor_id`;
the reuse record leaks no token or hash; `revocation_failed` present when revocation failed
and absent when it did not; a nil logger across all eight methods).

**Still live, small:** `tools/coverage/policy.json:41`'s `why` for the
`modules/*/infrastructure/...` floor still reads *"Covered by integration tests, which need
a database"*. That string is printed on ANY failure under that pattern and is what
misdirected E7 the first time. It is true of the persistence packages and false of
`auditlog`. Worth splitting the pattern or rewording before it misdirects again.

### E18 (original entry, kept) — CONFIRMED WITH FILE:LINE: THE COVERAGE GATE HAS NEVER EXECUTED
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

## E11 — **ANSWERED 2026-08-29 (human, delegated). Implemented in #51 (`8f82381`).**
**Decision: move the shared port types into `domain`. R1 is NOT amended.**
`RenderedMessage` and `ErrNonRetryable` now live in `modules/notification/domain`.
**D8 is unblocked outright; D5 now waits on E13 and E14 only.**

**Why not this entry's own proposal.** Amending R1 to permit
`modules/X/infrastructure → modules/X/application` **would stop R1 preventing the thing it
exists to prevent.** An import-level rule cannot distinguish *"names a port type it
implements"* from *"calls a use case"*, so the amendment buys the sender its signature and in
the same stroke legalises a sender re-entering the dispatch cycle that drove it. Neither
archtest nor depguard could tell those apart afterwards — they see imports, not symbols. That
trades an enforceable rule for an unenforceable convention, permanently, to solve a naming
problem.

**Why `domain`.** It is already how every other port in this tree works —
`NotificationRepository` and `PreferenceRepository` are declared in `domain` and implemented
in `infrastructure`. Senders are the same shape, and making them the exception is what created
E11. `ErrNonRetryable` was arguably misplaced regardless: it decides dead-versus-pending, a
**domain state transition**, while `domain/errors.go` already owns that vocabulary next to
`ErrInvalidTransition`. The interfaces themselves did **not** move — Go satisfies interfaces
structurally, so a sender never imports `ChannelSender`, only the parameter type.

**Cost.** Eight references, all inside `modules/notification/application`, and no sender
exists yet. After D5/D8 ship, the same decision costs a rewrite of every sender.

**The concession, recorded not hidden.** `Subject` and `Body` are rendered presentation data,
and `domain` now holds a type it never constructs. Judged smaller than making R1
unenforceable; `domain.Channel` already lives there. **Reversible** — the original amendment
is one commit away if this is judged wrong.

**The gap this entry found is now closed.** R1's `infrastructure` row was written down and
enforced by **nothing** — `TestApplicationLayerDoesNotTouchInfrastructure` guards only the
opposite direction. `TestInfrastructureLayerDoesNotTouchApplication` now enforces it, and it
was **proven to fire**: an `application` import planted in
`modules/notification/infrastructure/persistence/repository` fails it with the expected
message, green again once reverted. (Reported twice per violation — BL35, pre-existing.)

### E11 (original entry, kept) — `infrastructure` may not import `application`, but D5 and D8 must
Blueprint §5 puts the sender ports in `application/ports.go`, so every sender must import it
**to spell its own method signature** — which R1 forbids and **neither archtest nor depguard
catches**. Amend R1 to `infrastructure → domain, own application's ports, …` and add an
archtest rule permitting `modules/X/infrastructure → modules/X/application` only.
**Must land before D5.**

## E12 — **ANSWERED 2026-08-29 (human, delegated): approved, with one binding constraint. NOT yet implemented.**
`DispatchObserver`, optional, nil ⇒ no-op, as proposed. Confirmed in code: `settle` computes the
outcome and channel per row and discards both, and `DispatchBatch` returns only `len(claimed)`.

**The constraint, carried from E11's ruling: every parameter must be a DOMAIN type**, so a
metrics adapter in `infrastructure` can satisfy the port without importing `application` (R1):

    Settled(ctx context.Context, n domain.Notification, outcome domain.Status, err error)

`domain.Status` already carries the vocabulary — sent, dead, pending. **If `outcome` were an
application-defined enum the adapter would have to import `application`, which is exactly what
E11 forbade.** The interface itself can stay in `application`: Go satisfies interfaces
structurally, so the implementer never names it — the same reasoning as the E15 ruling.

**`DispatchBatch`'s signature does not need to change.** This entry's title says `(int, error)`
"cannot support" metrics; the answer is that it does not have to. The observer carries what the
return value cannot, and the change stays additive with no breaking edit to a merged signature.

**For BL25, the observer must be able to tell a panic from a failure**, or the visibility gap
survives in a new form. `settleSafely` currently wraps a recovered panic in a bare
`fmt.Errorf`; it should wrap a sentinel the observer can `errors.Is`.

**Not implemented in #63**, deliberately: E12 is independent of D6's chain and touches the
application layer rather than persistence. **`claimed_at` (E9b) has already landed the durable
half of its latency measurement.**

### E12 (original entry, kept) — `DispatchBatch (int, error)` cannot support D10's metrics or audit
Per-channel counters, a latency histogram, and an audit event on **security-category
dead-letter** are all computed in `settle` and discarded. Prefer a
`DispatchObserver.Settled(n, outcome, err)` port — optional, nil ⇒ no-op — which also fixes
BL25's panic-visibility gap.

## E13 — **ANSWERED 2026-08-29 (human, delegated). Implemented in #53 (`0d0b02d`).**
**Decision: ship `sender/inapp.go` in D5 AND refuse unroutable channels at `Enqueue`.
Webhook stays a v1 non-goal.**

**The entry's diagnosis was exact, and verified:** `AllChannels()` returns all three channels
and `Valid()` accepts all three, so an in-app enqueue is legitimate; `dispatch.go:148` then
finds no sender and returns `ErrNonRetryable`, so the row goes **`pending` → `dead` having
never been attempted**. D7's mailbox would have been permanently empty.

**Why the fix is bigger than the entry asked for.** Shipping only `sender/inapp.go` fixes that
instance and leaves the *shape*: webhook is a v1 non-goal, and **a non-goal that `Valid()`
accepts is the same trap one channel over.** Worse, `Enqueue` reported **success** while the
row died — a silent drop wearing the costume of a durable outbox. So the guard is general:
`Enqueue` returns `ErrChannelNotRoutable` for any channel with no registered sender, which
turns a dead row nobody watches into a loud error at the producer.

**Checked before the preference lookup**, deliberately: an unroutable channel is a broken
contract between the service and whoever wired it, and it stays broken whether or not this
user wanted the message. Reporting "delivered nothing, successfully" because the recipient had
it switched off would hide a misconfiguration behind a user's setting.

**The in-app sender is still D5's**, and is the ~10-line no-op the design implies — the row
*is* the notification, as `RenderedMessage`'s own doc says. Note dispatch renders *before*
calling the sender, so an in-app row with a broken template dies with `LastError` set rather
than reaching the mailbox broken. Kept deliberately: a free template smoke-test, at the cost
of rendering in-app content twice (the dispatch-time render is discarded).

**One existing test changed and NOT weakened.**
`TestDispatchBatchDeadLettersWithoutRetrying/"no sender is registered for the channel"` asserts
the *dispatcher's* fail-closed handling, which is still correct and still reachable — a row
queued while a sender was wired and drained after it was removed, exactly what its own comment
describes. It reached that state through `Enqueue`, which now refuses, so it seeds the row
directly via a new `harness.seedRow`. Relaxing it would have dropped coverage of the
fail-closed path while looking like a fix.

### E13 (original entry, kept) — no task ships an in-app sender ⚠
`inapp`/`webhook` appear **nowhere** in the task list. With D3's fail-closed behaviour, at
the D6 gate **every in-app notification goes straight to `dead`** — the channel D7 exists to
serve. Add `sender/inapp.go` to D5; record webhook as a v1 non-goal.

## E14 — **ANSWERED 2026-08-29 (human, delegated). Implemented in #53 (`0d0b02d`).**
**Decision: accept E14's shape — `HTMLBody` is added — with one invariant E14 did not state.**

**`Body`'s doc comment was wrong and is rewritten, not merely supplemented.** It claimed *"one
body per channel, not one per MIME type: a channel that needs several representations is a
channel that names several templates"*. That answers **email versus in-app**. It cannot answer
**the HTML part versus the text part of one email**, because those are two representations of
a *single* message and cannot be two notifications.

**The addition: plain `Body` is MANDATORY and `HTMLBody` is OPTIONAL, never the reverse.**
E14 argued deliverability — correctly — but did not say which field is required, and that is
the half that delivers the benefit. An email sent HTML-only is penalised by spam filters and
unreadable to a text client, and the notification most likely to be filtered is the security
mail, the one that must arrive. Making the plain part the required one means **no template can
produce an HTML-only message even by omission**. Senders emit `multipart/alternative` when
`HTMLBody` is set, text-only when it is not, and must never send HTML alone; in-app and
webhook ignore it.

**E14's file-list amendment is STALE and D5's must be corrected with it.** It says to amend
`application/ports.go`; **#51 moved `RenderedMessage` to `modules/notification/domain/message.go`**.
This also compounds #51's recorded concession: `domain` now holds a second piece of rendered
presentation data. Same trade, same reasoning — it is a field a sender must name.

### E14 (original entry, kept) — template shape: two documents, orthogonal axes, both incomplete
Plan D5 = channel axis; blueprint §3 = MIME-part axis. **You need both.** Single-body is
wrong for v1 (no `text/plain` alternative is a deliverability penalty, and the mail it costs
is the security mail). Add `HTMLBody string` to `RenderedMessage`; amend D5's file list to
include `application/ports.go`.

## E15 — **ANSWERED 2026-08-29 (human, delegated).** Precursor in #55; the rest is D8's.
**Decision, four parts. D8 is unblocked.**

**1. Resolve at dispatch, on every attempt. Never snapshot.** E15's rejection stands, and its
reason is stronger than staleness: **the message most likely to be diverted by an address
change is the one announcing the address change.** A snapshot mails "your email was changed"
to the address the attacker just replaced. `modules/notification/domain`'s own doc already
says resolution is *"the sender's job at dispatch time"*.

**2. The port is declared by its CONSUMER — the email sender — in
`modules/notification/infrastructure/sender`, NOT in `domain`.**
This deliberately differs from E11's outcome, and the difference is real: E11 put
`RenderedMessage` in `domain` because a sender must *name* it to spell a method it
**implements**. Here the sender **consumes** the interface, so Go's convention and R2 agree —
*"cross-module needs are expressed as an interface declared by the consumer and satisfied at
the composition root."* Declaring it in `domain` would put a third piece of foreign data there
after `RenderedMessage` and `HTMLBody`, and this module's doc explicitly disclaims knowing
what a user is.

**3. The stated blocker DISSOLVES — `identity.New` does not change.** E15 says *"`identity.New`
exposes nothing resolvable"*, and `module.Module` does expose only Name/Routes/Migrations/Close.
But the composition root does not need it to: **`cmd/api/container.go:95` already builds its own
`token.NewJWTService`** rather than extracting one from the module. Same move —
`identityrepo.NewUserRepo(deps.DB)`, wrapped in an adapter. The repo is stateless over
`*bun.DB`, so a second instance costs nothing. **This matters because E25 settled
`(*module.Module, error)` as the constructor shape: no exception to it is needed.**
Per E16-ARCH there is no `users`/profile involvement — `RecipientID` is already an identity
uuid, so it is `auth_users.email` via `FindByID`, no mapping.

**4. The error contract E15 did not specify, which D8 needs:** recipient not found (deleted
user) wraps `domain.ErrNonRetryable` and dead-letters the row; a transient failure returns a
plain error and retries. Backwards one way it retries a ghost until the budget is spent;
backwards the other it destroys a security mail on a blip.

**Part 4 was not implementable as stated, and #55 fixes that.** `UserRepo`'s lookups returned
`errors.New("not found")` — a fresh value per call, classifiable only by matching text — so
the adapter could not have told the two cases apart. `refresh_token_store.go` was already
using `db.ErrNotFound`/`db.Translate`; `user_repo.go` was the outlier. #55 converts it and
pins the behaviour with `errors.Is` assertions.

**What remains is D8's**: the port declaration, the sender, and the `cmd/api` adapter. No
further answer is needed from the human.

**Found while answering, NOT changed:** `modules/identity/application/login.go:13-19` collapses
a database outage into `ErrInvalidCredentials` and audits it as `ReasonUnknownUser` — an outage
is recorded as though the account did not exist. Pre-existing, outside E15, and it touches
authentication behaviour, so it wants its own task rather than a drive-by.

### E15 (original entry, kept) — nobody owns address resolution ⚠ blocks D8
Proposed **D8a**: `AddressResolver` in `infrastructure/sender/`, adapter in `cmd/api` over
identity's `UserRepository.FindByID`. Needs no `users` involvement. **Blocker inside:**
`identity.New` exposes nothing resolvable. **Rejected:** snapshotting the address at enqueue
— mails a **stale address after an email change**, i.e. the breach alert goes to the
attacker's new address.

## E9b — **ANSWERED 2026-08-29 (human, delegated): option (a), `claimed_at`. Implemented in #63.**
Not (b). Four reasons, and the fourth decided it:
1. **(b) writes to the very column the claim predicate reads.** A mistake there changes *which
   rows are deliverable*, not merely which are reported. A new column cannot touch delivery.
2. **(b) gives one column two meanings selected by another** — "not before this" while pending,
   "claimed at this" while sending. This tree has been bitten by that shape: **D4's
   `default:true` tag, which made `false` unrepresentable.**
3. **The migration this entry regretted is cheap here.** ADR-010 makes additive migrations the
   norm and notification's is the newest, so nothing reorders.
4. **`claimed_at` is exactly what E12's latency histogram needs** — `sent_at - claimed_at`,
   durable and queryable rather than an in-process timer. **(b) could never provide it**, because
   it overwrites the value on every retry. **One migration serves both escalations**, which is
   why the two were answered together.

Nullable, no default, no backfill: NULL means "never claimed", true of every existing row, and
a `DEFAULT now()` would have claimed the whole outbox at migration time.

### E9b (original entry, kept) — D6's sweep has no column to run on; the cheap window CLOSED
`next_attempt_at` holds its **pre-claim** value, so a freshly claimed row is
indistinguishable from a stalled one and resetting it is a **double delivery**. D1 merged
before this was decided, so it now costs a **new migration** — my error. Options: (a) add
`claimed_at`; (b) specify `ClaimBatch` to set `next_attempt_at = now` on claim, with the
semantics written into its doc comment. **E9a raised the stakes:** a false positive now also
burns an attempt and can dead-letter a healthy in-flight row.

## E9c — **ANSWERED 2026-08-29 (human, delegated). Implemented in #63.**
`ClaimStalled(ctx, n, stalledBefore, now)` on `domain.NotificationRepository`, with the
`FOR UPDATE SKIP LOCKED` shape `ClaimBatch` already establishes. This entry's correction is
upheld: the sweep is select → `RecoverStalled` → `Save` per row.

**Two things it deliberately does not do.** It **does not change status** — a set-based recovery
UPDATE would be a second, untested copy of the state machine, so the transition stays in
`RecoverStalled` and this decides only which rows a sweeper owns. And it **does not trust the
caller to hold a lock**: since E9a, `RecoverStalled` increments `Attempts`, so two sweepers on
one row burn two attempts and can dead-letter a row that was merely slow.

**It re-stamps `claimed_at`**, which is what makes a crashed sweeper safe — a row whose recovery
never lands is deferred by another full stall window rather than being immediately eligible
again. That costs the original claim timestamp, which nothing reads once the sweep has decided.

**Deviation from this entry's proposed signature:** it carries `now` as well as `stalledBefore`.
The re-stamp needs it, and it keeps the caller's injected clock governing the whole cycle exactly
as `ClaimBatch`'s does.

**The domain entity does NOT carry `ClaimedAt`** — `Save` writes a fixed five-column list that
excludes it, so nothing in the domain needs to see it, and the column survives a save untouched.

### E9c (original entry, kept) — the domain has no port to FIND stalled rows
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

## E8 — **RESOLVED 2026-08-29 by #46 (`bc0cc2f`)**
The entry below was right, and can be sharpened: **the two halves repair DIFFERENT tests,
and neither is optional.**
- **Isolation alone does not fix `_IsReversible`.** In an empty schema `goose.Up` still
  applies all 8 migrations and `goose.Down` still reverts only the newest. It needs the
  version pin: `UpTo(7)` → `DownTo(6)` → `UpTo(7)`.
- **Pinning alone does not fix `_RefusesCollidingData`**, and is worse: `DownTo(6)` on the
  shared database leaves `public` at 6 for whatever runs next.
- **Isolation alone DOES fix `_RefusesCollidingData`** — a private schema makes `UpTo(6)`
  mean what it says.

**New fact, found while verifying:** the old tests did not merely fail. Running them
against a fully migrated database **rewinds `public` from `20260808170025` to `7` and
leaves it there**; every package running afterwards inherited that. The fix carries
`assertPublicUntouched` so a recurrence fails loudly at the source.

Isolation was **not designed** — `schema_test.go:43-58` already migrated into a private
schema, which is exactly why that test alone in the package was never part of E8. Two
details it lacks were added: `SetMaxOpenConns(1)` (a bare `SET search_path` lands on one
pooled connection, and `schema_test.go` survives on pool luck) and a `current_schema()`
assertion.

Verified against real Postgres 16 in CI's configuration — both failures reproduced with
CI's exact messages first — and **mutation-tested**: removing 00007's constraint restore
fails `_IsReversible`; downgrading its `RAISE EXCEPTION` to `RAISE NOTICE` fails
`_RefusesCollidingData`. No assertion was weakened; no migration file was touched.

### E8 (original entry, kept) — `internal/db` — the one-line fix is NOT enough
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

# CI — GREEN on `main` as of 2026-08-29 (verified in run logs)

`main` = `5030e7f`. Latest full green: run 33243128092.

| Job | State |
| --- | --- |
| **Build, vet, test** | 🟢 green — **every step**, including the two below that had never run |
| **golangci-lint** | 🟢 green — T4 (#42). depguard now executes; the authn/httpx fence's editor-time half is live |
| **Swagger spec** | 🟢 green — T5 self-healed exactly as predicted |
| **Migrations up/down/up** | 🟢 green |
| **Vulnerabilities and secrets** | 🟢 green — T3, then **T6 (#44)** |
| **Binary size / No large files** | 🟢 green |

**Two steps executed for the first time in this repository's history**, both previously
`skipped` because the job died at `go test -race` before reaching them:
- **`architecture rules` — PASSED.** B4's and T2's fences had never once been enforced by
  CI. They hold. Until 2026-08-29 the archtest suite was written, merged, believed, and
  never run by the pipeline.
- **`coverage floors and ratchet`** — now passing; see E18.

## T6 — the toolchain pin has no owner, and this WILL recur ⚠ NEW
#44 bumped `go 1.25.12` → **`1.25.14`** after **six** stdlib advisories accumulated against
a fixed floor: GO-2026-6218 (`net/url`), GO-2026-6090 (`crypto/tls`), GO-2026-6089 and
GO-2026-5026 (`net/http`), GO-2026-6088 (`encoding/xml`), GO-2026-5972 (`encoding/asn1`) —
every one traced to our own call sites (`cmd/api/healthcheck.go:70`,
`internal/httpx/server.go:96`).

**This is T3's mechanism working as designed, not a regression.** CI installs
`govulncheck@latest` so the advisory database is always current, while the `go` directive is
a floor that fails closed. **The failure mode is time-based, not change-based:** it reddens
pull requests whose authors changed nothing — here after an eighteen-day gap between CI runs
on `main`.

**1.25.14 was taken over the 1.25.13 govulncheck named.** T3 pinned on "the newest patch in
the 1.25 line" and the two criteria had diverged; the merely-sufficient version schedules its
own recurrence. The `Dockerfile` moved in the same commit, per T3 — no CI job builds the
image (BL37), so that commit is the only thing coupling them.

**Ask:** who owns keeping this current, and on what trigger? Nothing on this board does.

## GITIGNORE — #49 ⚠ NEW, small
`.gitignore:22` read `*.zipcoverage.out` — **two entries fused into one**, so **`*.zip` was
ignored by nothing at all**. `coverage.out` was never at risk (`coverage.*` line 31, and
`*.out` line 16). Root cause is the **missing trailing newline** on the last line, so
anything appending lands *on* it; fixed too, or it recurs.

**Third time this file has bitten** — see `8b1b185`, "land tools/coverage, **which
.gitignore had been swallowing**".

Left undone: nine comment headers sit in a block at the top, detached from the entries they
label, body in one flat alphabetical run. The file looks `sort`ed, which is also the likeliest
origin of the fusion. Cosmetic — no effect on what is ignored.


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
- **BL34 — RESOLVED 2026-08-29, in TWO parts, because it had two failure modes.**
  (a) **Concurrent access** → #45 (`cf7a4cb`), `-p 1` on `ci.yml:92`. Sufficient rather than
  partial: none of the four integration files calls `t.Parallel()`, so all remaining
  concurrency was between binaries. Cost: `build` 2m16s → 3m33s.
  (b) **Leftover rows** → #48. `cmd/api/login_e2e_test.go` called `testsupport.MigratedDB`
  and never `TruncateAll`, so it inherited whatever ran before it and died on
  `idx_auth_users_email_lower`. **`-p 1` does not help this** — it fixed concurrency, not
  residue. It was green only because `cmd/api` sorts first under `./...` and so usually met
  an empty database: an ordering accident, not a property.
  Long-term: **per-package databases** remove both and buy back the 77s.
- **BL34 (original entry, kept)** Shared-`TEST_DATABASE_URL` leaks state between test binaries; `-p 1` avoids it.
- **BL35** Arch failures are reported twice per violation (`[p.test]` variant).
- **BL36 — ~~RETRACTED~~ 2026-08-29. THE TEST WAS NOT FLAKY AND ITS ASSERTION WAS SOUND.**
  It was one instance of BL34's class. The fix this entry proposed — force overlap, or drop
  the assertion — would have **weakened a correct test and left the real bug in place**.
  Evidence: CI run 33236797861, the SAME SHA run twice, failed a **different set of tests
  each time** — only `internal/db` (E8) was constant, the rest rotated across notification
  AND identity. Signatures: `"[]" should have 2 item(s), but has 0` (rows removed mid-test)
  and `deadlock detected (SQLSTATE 40P01)` traced to `internal/testsupport/db.go:168`
  (`TruncateAll`). Fixed by #45, not by touching the test.
  **Standing correction:** a test that fails intermittently is a *hypothesis about the
  test*, not a conclusion. Before recording one as flaky, run the same SHA twice and check
  whether the **same** test fails.
- **BL36 (original entry, kept)** `TestNotificationRepo_ClaimBatchIsExclusiveUnderConcurrency` is a **timing flake** —
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
