# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner                | State      | Branch / merged as         | Review              | Updated    |
| --- | -------------------- | ---------- | -------------------------- | ------------------- | ---------- |
| A1  | test-guardian        | **merged to main** | PR #16 (`9e50896`) | APPROVE-WITH-NOTES  | 2026-08-08 |
| A2  | domain-engineer      | **merged to main** | PR #16 (`6288dcd`) | APPROVE-WITH-NOTES  | 2026-08-08 |
| A3  | transport-engineer   | pending    | `feat/A3-canonical-401` (to cut from `main`) | - | 2026-08-08 |
| A4  | transport-engineer   | blocked    | - (E2 open; needs A3)      | -                   | 2026-08-08 |
| B1  | test-guardian        | pending    | -                          | -                   | -          |
| B2  | transport-engineer   | pending    | - (see E2)                 | -                   | 2026-08-08 |
| B3  | test-guardian        | pending    | - (see B3 note)            | -                   | 2026-08-08 |
| B4  | test-guardian        | blocked    | - (blocked on E1)          | -                   | 2026-08-08 |
| C1  | docs-scribe          | pending    | - (E3+E4+E5 settled)       | -                   | 2026-08-08 |
| D1  | persistence-engineer | pending    | -                          | -                   | -          |
| D2  | domain-engineer      | pending    | -                          | -                   | -          |
| D3  | domain-engineer      | pending    | -                          | -                   | -          |
| D4  | persistence-engineer | pending    | -                          | -                   | -          |
| D5  | platform-engineer    | pending    | -                          | -                   | -          |
| D6  | platform-engineer    | pending    | -                          | -                   | -          |
| D7  | transport-engineer   | pending    | -                          | -                   | -          |
| D8  | platform-engineer    | pending    | -                          | -                   | -          |
| D9a | domain-engineer      | pending    | -                          | -                   | -          |
| D9b | platform-engineer    | pending    | -                          | -                   | -          |
| D10 | platform-engineer    | blocked    | - (blocked on E1)          | -                   | 2026-08-08 |

**Critical path:** A3 → A4 (needs E2) → Phase B.

## Merge log

| What | Merged as | Contents |
| --- | --- | --- |
| PR #15 `refactor/module-package-names` | `d63200b` (2026-08-08T04:47Z) | `6eed3aa` package rename — the Phase A base |
| **PR #16 `feat/A1-golden-401-capture`** | **`f594ebc`** | `9e50896` (A1), `6288dcd` (A2), `c5c25bd` (human's docs/roster commit), `ef11a6c` (board) |

### Phase A PR merged early — recorded deviation (human's decision, not mine)
The plan specifies Phase A as **one PR with sequenced commits A1→A2→A3→A4**. PR #16
was merged after **A2**, so A3 and A4 are no longer commits on that branch.
**Consequences, all benign but load-bearing for what comes next:**
1. **A3 and A4 each need a fresh branch cut from `main`** — `feat/A1-golden-401-capture`
   is merged and must not be reused or force-updated. A1's and A2's commits are
   preserved verbatim in `main`'s history, as required.
2. **Nothing shipped behaviourally.** A1 is test-only; A2 adds a leaf package that
   *nothing imports yet* (verified at review: `grep -rn "internal/authn"` outside the
   package returned no hits). `main` is therefore not in a half-migrated state — the
   old context key is still the only mechanism in use, and the canonical 401 does not
   exist yet. This is why merging early was harmless.
3. **The guardrail-8 baseline is unchanged** — `make lint` remains red on BL1 and
   `make coverage-check` remains dead (E1), exactly as before the merge.
4. **A4's atomicity warning still applies in full.** Old-key deletion and every
   reader update must land in one commit — now in A4's own PR rather than commit 4 of
   a stack. The `MustFromContext` panic is still the loud failure mode for a missed
   reader.

**`c5c25bd`** — committed by Sujanto, 2026-08-08 17:09:45 +0700, "docs:
agent-implementation-plan.md". 22 files, +1944/−508: all four `docs/plans/*.md`, the
new `.claude/agents/*` roster, the eight deleted legacy agent definitions,
`.claude/settings.json`, `README.md`. Not dispatched by me and not gated — the
human's own commit, recorded so no reviewer mistakes it for worker output.

## Sequencing decisions (operational, not plan changes)

- **Phase A base was `6eed3aa`**, now an ancestor of `main` via PR #15. An earlier
  note here claimed the base could not be `main`; that was read off a **stale local
  `main`** two commits behind origin. No harm resulted — the branch was correctly
  based on current main content and merged without replay.
- **A3/A4 branch from `origin/main` (`f594ebc` or later).** Fetch before cutting.
- **Workers commit ONLY their task's file list, by explicit path.** There is
  **concurrent human activity in the working tree** — as of this writing `.gitignore`
  carries an uncommitted `coverage/` → `/coverage/` edit (the E1 fix, in progress),
  and eight legacy `.claude/agents/*.md` files are present but untracked. No worker
  may stage, revert, or "clean up" any of it. See BL15.
- **D1/D2 held.** The graph shows no edges into them, but the plan sanctions them
  "in parallel with **Phase B**", and Phase B has not started.

## Escalations — RESOLVED

### E6 — RESOLVED 2026-08-08 — `docs/plans/` is tracked and on `main`
Closed by the human's own commit `c5c25bd`, merged to `main` in PR #16. All four
plan documents — including this board — are in history. The separately-requested
docs-only PR reduced to publishing the newer board revision, since the plan documents
themselves were already identical on `main`. **No add/add conflict risk remains**
(the earlier concern assumed two independent branches adding `docs/plans`; in fact
both sides descend from `c5c25bd`).

### E5 — RESOLVED 2026-08-08 — `request_id` in the canonical 401 body is ACCEPTABLE
**Was:** `internal/httpx/problem.go` `WriteProblem` fills a `RequestID` extension
member (`omitempty`) that §8.2's literal example omits, while §8.2 says
`httpx.Unauthorized` emits "exactly" that shape — and A3's task text orders reuse of
that writer. A plan-internal contradiction.
**DECISION: §8.2's example is illustrative, not literal.** A3 reuses `WriteProblem`;
no hand-rolled JSON, no fork of the problem writer.
**Consequences:**
1. **A3 is unblocked.**
2. Canonical body = `{type, title, status, detail, instance}` plus `request_id`
   **when present** — `omitempty` means absent under a bare `chi.NewRouter()`,
   present under the real `httpx.NewRouter`.
3. **A4 golden hazard (A4 adjustment #3):** `request_id` is volatile and
   `compareGolden` (`unauthorized_golden_test.go:185`) compares marshalled bytes with
   no normalization hook. A4 must keep the golden harness free of the request-ID
   middleware or add a scrubber. Now a *known* hazard, not a discovered one.
4. C1 documents `request_id` as an RFC 9457 extension member; do not copy §8.2's
   literal example as if it were the contract.

### E3 — RESOLVED 2026-08-08 — corrected changelog scope APPROVED
**Ground truth pinned by A1's goldens, all five cases:** status 401 uniform;
`Content-Type: text/plain; charset=utf-8` uniform (from `http.Error`);
`WWW-Authenticate` **absent on every path**; body has exactly two values —
`{"error":"missing token"}` vs `{"error":"invalid token"}` (the latter four already
byte-identical).
**APPROVED SCOPE — C1 carries this verbatim:**
1. **Lead with the transport-level break, not the `detail` string:** `Content-Type`
   `text/plain; charset=utf-8` → `application/problem+json`; body `{"error":"…"}` →
   RFC 9457 object **with no `error` member at all** (zero field-name overlap — any
   client matching on `body.error` breaks outright rather than degrading);
   `WWW-Authenticate: Bearer realm="api"` now sent, where previously **none was sent
   on any rejection path, ever** — it did not "vary by path", it was uniformly absent.
2. **Keep the plan's line** ("clients must key off `status`, never `detail`"),
   **scoped** per E4 and *augmented* by (1), not replaced.
3. **Do not repeat §8.1/§8.3's "collapses several unstable shapes into one."**
   Unsupported: two strings differing by one word.
4. **Every claim checked against merged code**, not the plan or this board.
**Residual question, open, not blocking:** §8.1's "Expected finding: the current
rejections are already inconsistent *with each other*" is known false and will
mislead the next reader. Outside C1's file list and I cannot edit companion docs.

### E4 — RESOLVED 2026-08-08 — NARROW THE CHANGELOG CLAIM (do not widen A4)
**DECISION: the claim covers the middleware-protected surface only.
`/auth/mfa/verify` is NOT touched.**
1. **A4's scope stands exactly as the plan wrote it** — file list unchanged, no new task.
2. **C1's uniformity claim is scoped** to 401s from the bearer-token middleware
   (routes behind `AuthRequired`), not the whole API.
3. **State the carve-out positively** — `/auth/mfa/verify` deliberately retains an
   OAuth2-style `application/json` error body because it is an OAuth2-conformant
   endpoint ahead of authentication. Silent omission is the failure mode this creates.
4. **Accepted permanent divergence — BL13.** Do not "fix".
5. **B3 unaffected** — its suite asserts uniformity for *middleware implementations*.
6. **A1's goldens sit entirely within the narrowed scope** (all hit
   `POST /api/v1/auth/logout`).

## Escalations — OPEN

### E1 — OPEN — `tools/coverage` has never been committed; guardrails 7 and 8 are unexecutable repo-wide
**Blocks:** guardrail 8 for every task; **B4 and D10 outright**.
**Confirmed:** `git ls-files tools/` → only `archtest`, `schemacheck`.
`git log --all --diff-filter=A -- tools/coverage` → empty.
`git ls-files | grep -iE "coverage|policy"` → empty. `Makefile:87` and `:91` both
`go run ./tools/coverage`. Dead targets.

**⚠ CORRECTION OF MY OWN CORRECTION (2026-08-08). arch-reviewer's root cause was
RIGHT; my "correction" of it was WRONG, and wrong in exactly the way I accused the
reviewer of.** I read the working-tree `.gitignore` instead of the committed blob.
Verified properly:
- `git show HEAD:.gitignore` → line 33 is **`coverage/`** (bare).
- `git show 6eed3aa:.gitignore` → line 33 is **`coverage/`** (bare).
- The anchored `/coverage/` I reported is an **uncommitted local edit** in the working
  tree, made concurrently by the human — which is also why my
  `git check-ignore tools/coverage/main.go` returned "not ignored".
**So the committed truth is:** the bare `coverage/` pattern matches at any depth and
**does** silently swallow `git add tools/coverage`. The fix **does** require the
`.gitignore` anchoring, as the reviewer said. That edit is currently in the working
tree, uncommitted (BL15).

**What survives from my analysis (independently verified, still true):**
- `b7ac597 build(coverage)` added the Makefile targets and five test/source files but
  **not the tool** — no `tools/coverage` in its diffstat. The target shipped dead.
- A complete implementation is stranded at
  `.claude/worktrees/phase-3-consolidate/tools/coverage/` (`main.go` 13.7KB,
  `main_test.go` 10.5KB, `policy.json` 2KB).
- **Stdlib only** — `bufio, encoding/json, errors, flag, fmt, os, path,
  path/filepath, sort, strconv, strings`. **Adds no dependency.**
  `modulePrefix` (`main.go:42`) still correct; flags match the Makefile comments,
  `-with-database` included; `policy.json` floors are *directory* patterns, unaffected
  by the package-name rename.
- **Stale policy content to settle on the way in:** `exempt` names
  `modules/ofbiz/...` which no longer exists; `baseline` omits `internal/authn` (A2),
  `modules/identity/transport` (0% → 15.8% via A1), `internal/logger`,
  `internal/module`, `modules/identity/infrastructure/auditlog`, both
  `.../persistence/models`.
**Decision needed:** authorise a task to land the anchoring **and** the tool — route
to **test-guardian** (roster: "coverage policy"), gated by arch-reviewer, with your
call on baseline-as-is vs regenerate with `-update`. Or amend guardrails 7/8 and
respecify B4/D10.
**Interim in force:** workers report coverage via `go test -cover`; forbidden from
creating the tool, editing `policy.json`, or touching `.gitignore`.
**Process note:** two of my own diagnoses and two of the reviewer's have now been
distorted by reading stale or uncommitted state. Standing rule from here: assertions
about repo configuration cite `git show <ref>:<path>`, never the working file.

### E2 — OPEN — `modules/user` does NOT read identity's context key; a load-bearing premise of the analysis is false
**Affects:** A4 (blocking), B2, the stated rationale for the SPI.
**Evidence (repo-wide, worktrees excluded):** key defined at
`modules/identity/domain/service.go:10,12-13`; written at
`modules/identity/transport/middleware.go:30`; read at
`modules/identity/transport/auth.go:109` (Logout), `:134` (MFASetup), `:162`
(MFAVerifySetup) — **all three read sites. None outside `modules/identity`. None in
tests.** `modules/user/transport/user.go` imports only `modules/user/domain` and
derives ids from the URL path; `modules/user/module.go:42` takes
`AuthMiddleware func(http.Handler) http.Handler` and never inspects a principal.
**Contradicts:** analysis §1, §4, §10 risk 1 (currently unrealizable).
**Consequence:** A4's atomic migration is 4 sites in 2 files, all inside
`modules/identity` — far smaller than planned. The SPI still pays forward (the edge
appears the moment B2/notification adds ownership checks), but the plan's framing is
wrong.
**Decision needed:** confirm the SPI rationale still holds as stated before A4/B2.
**The only escalation on the critical path.**
**Also:** `modules/user/module.go:42` is `AuthMiddleware`; B2's text says
`Config.Auth`. The rename is free — `httpx.NewRouter` is already
`mw ...func(http.Handler) http.Handler` (`router.go:33`), and because `Middleware` is
an alias, `...authn.Middleware` is the *identical* type; zero call-site churn.

## A3 dispatch adjustments — banked (E5 resolved; ready to dispatch)

- Branch `feat/A3-canonical-401` from `origin/main` (`f594ebc` or later).
- Reuse `internal/httpx/problem.go:36` `WriteProblem(w, r, status, typ, title,
  detail)` — it fills `Instance` from `r.URL.Path` (`:42`), `RequestID` from
  `chimw.GetReqID` (`:43`), and sets `X-Content-Type-Options: nosniff` (`:47`).
  `ProblemContentType` constant at `problem.go:13`. Do not hand-roll JSON.
- `Unauthorized` must set `WWW-Authenticate: Bearer realm="api"` **itself, before**
  calling `WriteProblem` — the writer does not set it.
- `request_id` in the body is approved (E5).
- No import edge to `internal/authn`: `Unauthorized(w, r)` takes only `w` and `r`.
- `detail` must be a package-level constant used verbatim (task acceptance).

## A4 dispatch adjustments — banked from A1 + both gates

1. **"Byte-identical except `instance`" is vacuous as written** — all five cases hit
   `POST /api/v1/auth/logout`, so `instance` is identical too. Reviewer recommends a
   **sixth case on a second protected route** (`POST /api/v1/auth/mfa/setup`). Needs
   approval (scope).
2. **Assert positively, not by mutual equality** — content-type, `WWW-Authenticate`,
   RFC 9457 members, by value. Confirmed by E3.
3. **`compareGolden` (`unauthorized_golden_test.go:185`) has no normalization hook** —
   `request_id` is a known volatile member (E5). Keep the harness free of the
   request-ID middleware or add a scrubber.
4. **Name `unauthorized_golden_test.go` explicitly in A4's file list**; extending
   `unauthorizedGolden` and regenerating with `-update` is expected.
5. **The golden rig is a bare `chi.NewRouter()` + `h.Routes`**, not the production
   chain — verify the writer is self-contained or mount through `httpx.Mount`.
6. **Goldens record the response, not the reason** — distinctness verified
   out-of-band only. Verifier-level assertions aren't in the plan; need approval.
7. **Keep `Extra` nil** — zero clone allocations when nil; dumping the claims map in
   costs a per-request map clone **and** violates the admission rule.
8. **Panic contract is production-safe here** — `internal/httpx/router.go:57` mounts
   `Recovery(log)` globally, third in the stack, emitting problem+json 500. A4 must
   not add a route bypassing `NewRouter`'s stack. **Corollary:** any handler reachable
   both authenticated and anonymous must use `FromContext`, not `MustFromContext`.
9. **Ordering:** rejection paths call `httpx.Unauthorized` and `return` **before** any
   `WithPrincipal`, so no zero-Subject Principal reaches the context.
10. **E4 boundary — do NOT touch `/auth/mfa/verify`** (`writeOAuth2Error`,
    `helpers.go:60`; 401 paths `auth.go:198-203`, `:212-214`). Explicit non-goal (BL13).
11. **A4 is now its own PR**, not commit 4 of a stack. The atomicity warning is
    unchanged: old-key deletion + every reader update in ONE commit.

## B3 dispatch adjustment — banked

- `Principal` is **not comparable** (struct containing `[]string`). `got == want` will
  not compile — a compile error, never a silent wrong result. **Do not add `Equal` or
  change fields on `internal/authn`.** An `authntest.Equal`/`RequireEqual` helper in
  B3 is the right home if friction is real. `reflect.DeepEqual` is faithful *because*
  A2's clone helpers preserve nil.
- The suite's uniformity assertions cover **middleware implementations** only.
- **Dependency warning:** `github.com/google/go-cmp v0.7.0` is in `go.sum` but **not**
  a direct requirement in `go.mod`. `cmp.Diff` promotes it to a direct dependency and
  edits `go.mod` — must be a named line item in B3's file list, or I escalate it.

## C1 dispatch adjustments — banked

- Carry E3's approved scope, E4's narrowing, and E5's `request_id` decision verbatim.
- Scope the uniformity claim to routes behind `AuthRequired`; state the
  `/auth/mfa/verify` carve-out positively (BL13).
- C1's acceptance stands: every claim checked against **merged code**; links resolve.
- BL2 is worth a line.

## Backlog — pre-existing issues (do NOT spawn unplanned work)

- **BL1** `make lint` red: `internal/db/schema_test.go:31`, `:43` (errcheck).
  `git blame` → **`6eed3aa`**. Every PR inherits a red lint gate, making guardrail 8
  unmeetable by construction. Survives because `make ci` (`Makefile:292`) runs
  `check`, which excludes `lint`. Confirmed at A1 and A2 as the only two findings.
- **BL2** Swagger annotations lie about the 401 shape: `auth.go:105`, `:130`, `:158`.
- **BL3** `transport` imports `infrastructure` in production at `jwks.go:7`,
  contradicting R1 (`dependency-rules.md:79`). Unenforced by depguard or archtest.
- **BL4** RSA keypair parsed twice at boot (`modules/identity/module.go:98`,
  `cmd/api/container.go:95`); deliberate, `container.go:83-90`. Do not "fix".
- **BL5** `identity.New` returns only `(*module.Module, error)`;
  `cmd/api/container.go:95-105` builds a **second** `token.NewJWTService` +
  `AuthRequired`. A4/B2 must not assume a second return value.
- **BL6** `modules/identity/transport` 0% → 15.8% via A1; not in `policy.json`'s
  baseline; ratchet unverifiable (E1).
- **BL7** `go: no such tool "covdata"` locally — tolerated by the leading `-` on
  `Makefile:86,90`. CI is the arbiter.
- **BL8** `.gitignore:22` reads `*.zipcoverage.out` — a fused pattern; `*.zip` is
  ignored by nothing. Fix alongside E1.
- **BL9** (A2, cosmetic) `internal/authn/authn.go:92-98` says "same-request,
  same-goroutine bug"; same-goroutine is not necessarily true for a fan-out handler.
  The operative instruction is correct and sufficient.
- **BL10** (A2) A built-in context key type would leave the suite green; the defense
  is staticcheck **SA1029** via `.golangci.yml` `staticcheck.checks: ["all",
  "-QF1008"]`. **Do not "simplify" that config.**
- **BL11** (A2, cosmetic) `authn_test.go:301-303` overstates `wrap`; the guarantee
  rests on `TestMiddleware_IsATypeAliasNotADefinedType`.
- **BL12** Three live worktrees under `.gitignore:43` (`.claude/worktrees`) holding
  stale `.gitignore`/`CLAUDE.md`/tooling — source of one wrong root-cause diagnosis.
  Dispatches must tell agents to exclude them for **config files**, not just code.
- **BL13** **ACCEPTED PERMANENT DIVERGENCE (E4) — do not "fix".**
  `POST /auth/mfa/verify` emits 401 as OAuth2-style `application/json`. Mounted ahead
  of the auth middleware by design; converting it would break OAuth2 clients.
- **BL14** Local `main` was two commits behind `origin/main`, producing a wrong
  base-branch note. Always `git fetch` before cutting a branch.
- **BL15** **Concurrent human edits in the working tree — workers must not touch.**
  Uncommitted `.gitignore` change (`coverage/` → `/coverage/`, the E1 fix in
  progress) and eight untracked legacy `.claude/agents/*.md` files. No worker may
  stage, revert, or clean these.

## Deviations accepted

- **A1 (4)** — accepted by arch-reviewer: refresh-class as raw `cls:"refresh"` (no
  refresh constant; verified to fail the class gate with `ErrWrongTokenClass`);
  `alg:none` built in-file (MintToken hardcodes RS256), helper unexported/test-only,
  `internal/testsupport` free of `UnsafeAllowNoneSignatureType`, prior art at
  `token/jwt_test.go:115`; body stored as a raw JSON string (preserves `http.Error`'s
  trailing `\n`); plain `flag.Bool("update", ...)` as the repo's first golden test.
- **A2 (1)** — alias assertion rewritten to a reflect-based check plus a `wrap`
  call-site helper (the original round-trip would have compiled for a defined type,
  proving nothing). Reviewer confirmed by mutation that the replacement discriminates.
- **A2 copy semantics** — **copy-on-write, not copy-on-read**, endorsed. Rests on the
  clone *plus* the unexported key: `WithPrincipal` is the only way a Principal enters
  a context, so the stored slice/map is reachable only through that request's chain.
  **Cross-request corruption is structurally unreachable.** Nil round-trips
  losslessly — which is what makes `reflect.DeepEqual` safe for B2/B3/D7.
- **E3** corrected changelog scope; **E4** narrowed uniformity claim; **E5**
  `request_id` accepted — all approved 2026-08-08.
- **Phase A PR merged after A2** rather than after A4 — human's decision; recorded in
  the Merge log with its consequences.
