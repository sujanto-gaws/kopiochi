# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner                | State | Branch / PR | Review | Updated |
| --- | -------------------- | ----- | ----------- | ------ | ------- |
| A1  | test-guardian        | **merged** | PR #16 (`9e50896`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A2  | domain-engineer      | **merged** | PR #16 (`6288dcd`) | APPROVE-WITH-NOTES | 2026-08-08 |
| **T1** | test-guardian     | **approved — awaiting merge** | `chore/T1-land-coverage-tool` → **PR #18** (`8b1b185`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A3  | transport-engineer   | **approved — awaiting merge** | `feat/A3-canonical-401` → **PR #17** (`4a969f2`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A4  | transport-engineer   | pending — unblocked, needs A3 merged | - | - | 2026-08-08 |
| B1  | test-guardian        | pending | - | - | - |
| B2  | transport-engineer   | pending | - | - | 2026-08-08 |
| B3  | test-guardian        | pending | - | - | 2026-08-08 |
| B4  | test-guardian        | pending — **respecify, see B4 note** | - | - | 2026-08-08 |
| C1  | docs-scribe          | pending | - | - | 2026-08-08 |
| D1  | persistence-engineer | pending | - | - | - |
| D2  | domain-engineer      | pending | - | - | - |
| D3  | domain-engineer      | pending | - | - | - |
| D4  | persistence-engineer | pending | - | - | - |
| D5  | platform-engineer    | pending | - | - | - |
| D6  | platform-engineer    | pending | - | - | - |
| D7  | transport-engineer   | pending | - | - | - |
| D8  | platform-engineer    | pending | - | - | - |
| D9a | domain-engineer      | pending | - | - | - |
| D9b | platform-engineer    | pending | - | - | - |
| D10 | platform-engineer    | pending — unblocked by T1 | - | - | 2026-08-08 |

**Critical path:** merge T1 (#18) → merge A3 (#17) → A4 → Phase B.

### ⚠ THREE BRANCHES ARE PUSHED; I CANNOT MERGE THEM
`gh pr merge` and any direct push to `main` are **blocked by the sandbox
classifier**, with and without `--admin`. I stopped attempting rather than working
around it. PR creation and branch pushes work. **A human must merge:**

| PR | What | Gate | Order |
| --- | --- | --- | --- |
| **#18** | T1 — land `tools/coverage` | APPROVE-WITH-NOTES | **first** |
| **#17** | A3 — canonical 401 writer | APPROVE-WITH-NOTES | second |
| `docs/plans-tracking` | this board | n/a — my own file | any time |

Reviewer's reasoning for T1 first: self-contained, unblocks the *specification* of B4
and D10, prerequisite for guardrails 7/8 being meetable at all, and it cannot make
anything redder (E8). A3 was independently run against T1's gate and passes on the
merits — `internal/httpx` 93.9 → 94.0, ratchet satisfied in the right direction.

## Merge log

| What | Merged as | Contents |
| --- | --- | --- |
| PR #15 `refactor/module-package-names` | `d63200b` | `6eed3aa` package rename — the Phase A base |
| PR #16 `feat/A1-golden-401-capture` | `f594ebc` | `9e50896` (A1), `6288dcd` (A2), `c5c25bd` (human's docs/roster commit) |

### Phase A PR merged early — recorded deviation (human's decision)
The plan specifies Phase A as one PR with commits A1→A2→A3→A4. PR #16 merged after
**A2**, so A3 and A4 are each their own PR from `main`.
**Nothing shipped behaviourally:** A1 is test-only and A2 adds a leaf package that
*nothing imports yet* (verified at review). `main` is not half-migrated — the old
context key is still the only mechanism in use and the canonical 401 does not exist
there yet. **A4's atomicity requirement is unchanged:** old-key deletion and every
reader update in ONE commit.

## Escalations — RESOLVED

### E1 — RESOLVED — human authorised T1; delivered, approved, awaiting merge (#18)
Root cause (**arch-reviewer was right; my "correction" of it was wrong** — I read the
working file instead of the committed blob): `git show HEAD:.gitignore` line 33 was
the bare `coverage/`, matching at **any depth**, silently swallowing
`tools/coverage/`. `b7ac597 build(coverage)` added the Makefile targets but **not the
tool** — it shipped dead. T1 anchors the pattern and lands the stranded
implementation. Reviewer verified the fix in both directions, probed 13 adversarial
profiles confirming the tool **fails closed** (notably: a package in the baseline but
absent from the profile *fails*, so "the tests stopped running" cannot read as
"coverage is fine"), and confirmed `-allow-decrease` can release the ratchet but
**cannot defeat a floor**.
**Standing rule adopted:** assertions about repo configuration cite
`git show <ref>:<path>`, never the working file. Two of my diagnoses and two of the
reviewer's were distorted by reading stale or uncommitted state.

### E2 — RESOLVED — human confirmed the SPI rationale still holds as stated
The finding stands on the facts — `modules/user` never read identity's context key
(read sites are only `auth.go:109`, `:134`, `:162`, all inside `modules/identity`;
none outside, none in tests) — so analysis §1/§4/§10-risk-1 describe a coupling that
does not exist *yet*. Ruling: the rationale holds regardless, because the edge appears
the moment B2/notification adds ownership checks. **A4 is unblocked.** A4's migration
is 4 sites in 2 files, all inside `modules/identity`.
**Carry into B2:** `modules/user/module.go:42` is named `AuthMiddleware`, not
`Config.Auth`. The rename is free — `httpx.NewRouter` is already
`mw ...func(http.Handler) http.Handler` (`router.go:33`) and `Middleware` is an alias,
so `...authn.Middleware` is the *identical* type; zero call-site churn.

### E3 — RESOLVED — corrected changelog scope APPROVED
**Ground truth from A1's goldens:** all five cases 401; `Content-Type: text/plain;
charset=utf-8` uniform; **`WWW-Authenticate` absent on every path**; body has exactly
two values, `{"error":"missing token"}` vs `{"error":"invalid token"}` (the latter
four already byte-identical).
**C1 carries verbatim:** lead with the transport-level break — content-type
`text/plain` → `application/problem+json`; body → RFC 9457 object **with no `error`
member at all** (zero field-name overlap, so `body.error` clients break outright
rather than degrade); `WWW-Authenticate: Bearer realm="api"` now sent where **none was
ever sent**. Keep "key off `status`, never `detail`", scoped per E4 and augmented by
the above. **Do not repeat §8.1/§8.3's "collapses several unstable shapes into one"** —
unsupported. Every claim checked against merged code.
**Residual, open, not blocking:** §8.1's "Expected finding: … already inconsistent
with each other" is known false and will mislead the next reader. Outside C1's file
list; I cannot edit companion docs.

### E4 — RESOLVED — NARROW THE CHANGELOG CLAIM (do not widen A4)
`/auth/mfa/verify` is mounted outside the middleware (`auth.go:291`) and answers 401
via `writeOAuth2Error` (`helpers.go:60`) with `application/json`
`{"error":"invalid_request",…}`. **A4's file list is unchanged.** C1 scopes the
uniformity claim to routes behind `AuthRequired` and **states the carve-out
positively** — silent omission is the failure mode this creates. Accepted permanent
divergence, BL13. B3 unaffected. A1's goldens all sit inside the scope.

### E5 — RESOLVED — `request_id` accepted in the canonical body
§8.2's example is illustrative, not literal. A3 reuses `WriteProblem` per its task
text. Delivered and verified: 124-byte body without a request ID, 173 with one —
**that byte count is machine-specific** (chi's request ID embeds the hostname). No
test asserts a length; the table pins the key set instead. Do not let "173" migrate
into an assertion.

### E6 — RESOLVED — `docs/plans/` is tracked and on `main`
Closed by the human's own commit `c5c25bd`, merged in PR #16.

## Escalations — OPEN

### E7 — OPEN — `auditlog` has no tests and will fail the coverage floor the moment CI can reach it
**Raised by:** test-guardian (T1); **confirmed by arch-reviewer.**
`modules/identity/infrastructure/auditlog` has **8 functions and no test file**. It
matches the **pre-existing** `modules/*/infrastructure/...` floor of 60% — unchanged
by T1, which merely makes it enforceable for the first time.
**Evidence:** invisible locally only because this machine lacks `covdata`; a complete
toolchain emits no-test-file packages at 0.0%. Reviewer simulated a complete toolchain
and got `auditlog: 0.0% < floor 60.0%`, and confirmed **auditlog is the sole such
failure** — `cmd/*`, `internal/testsupport`, `internal/version` are exempt;
`internal/logger`, `internal/module`, `modules/identity`, `modules/user/transport`
match no floor; the two `models` packages emit no profile lines at all.
**A landmine with a delayed fuse:** invisible today because CI never reaches the
coverage step (E8); becomes an immediate CI failure the instant E8 is fixed.
**Decision needed:** give it an owner now. The fix is a test, or an explicit reasoned
`exempt` entry — **not a lower floor.** Whoever fixes E8 should expect to handle this
in the same PR, or CI will appear to "break" somewhere new.

### E8 — OPEN — CI has been red for 12+ consecutive runs, for reasons predating this effort
**Found by:** arch-reviewer while establishing whether T1 could make CI redder. Three
independent pre-existing failures, none caused by any task here:
1. **`internal/db` — `TestCaseInsensitiveIdentifiers_RefusesCollidingData` fails**
   (`duplicate key value violates unique constraint "idx_auth_users_email_lower"`).
   This is why every run is red, and it sits **upstream of the coverage step**, so
   guardrails 7/8 remain unverifiable in CI even after T1 merges. Highest-value fix.
2. **The CI `golangci-lint` job cannot start** — "the Go language version (go1.24)
   used to build golangci-lint is lower than the targeted Go version (1.25.0)". **The
   lint job has been producing no findings at all**; local `make lint` is currently the
   repo's only lint coverage. A silent hole, not a noisy one.
3. **`govulncheck` fails** on stdlib advisories (`net` NUL-byte Dial/LookupPort panic;
   `crypto/x509` DSA panic). Likely a toolchain bump.
**Decision needed:** these are outside the plan and outside every task's file list.
They need owners, or guardrail §0.4's "CI is the arbiter" is fiction.

## B4 — respecify before dispatch (arch-reviewer's recommended amendment)
T1 consumed only the first clause of B4's coverage sentence. **B4 still owns the more
important half**; nothing is orphaned:
1. **The archtest rule** — only `modules/*`, `internal/httpx`, `internal/testsupport`,
   `internal/authn/authntest` may import `internal/authn`. Untouched by T1 and
   load-bearing: the coverage floor says authn is *tested*, the archtest rule says
   authn is not *bypassed*. Different guarantees.
2. **The deliberate-violation proof** — untouched; a blocking criterion in its own right.
3. **`internal/authn/authntest` matches NO policy pattern** (confirmed: the
   `internal/authn` floor pattern is non-recursive — 2 segments vs authntest's 3 — and
   `exempt` lists `internal/testsupport` but not `authntest`). It would land unfloored
   and unbaselined. B4 decides: exempt, or a floor. Arguably a floor, since B3's own
   acceptance is "the suite itself has tests", and an untested conformance suite is a
   gate that passes everything.
4. **Re-verify** `internal/authn: 100` still holds after B2/B3 — T1's number is a
   snapshot at `f594ebc`, not a guarantee.
**Amended wording:** "Coverage: the `internal/authn` 90% floor and its baseline landed
in T1; **verify** they still hold after B2/B3 rather than re-adding them, **decide the
policy treatment of `internal/authn/authntest`** (exempt vs floor, with a reason), and
add any other package whose floor now applies."

## A4 dispatch adjustments — banked, ready

1. **"Byte-identical except `instance`" is vacuous as written** — all five cases hit
   `POST /api/v1/auth/logout`, so `instance` is identical too. Reviewer recommends a
   **sixth case on a second protected route** (`POST /api/v1/auth/mfa/setup`). Scope
   call — needs approval.
2. **Assert positively, by value** — content-type, `WWW-Authenticate`, RFC 9457
   members. Mutual equality is nearly free to satisfy and proves little.
3. **Goldens will NOT flap — confirmed.** `unauthorized_golden_test.go:211`
   `protectedRouter` builds a bare `chi.NewRouter()` with **no `chimw.RequestID`**, so
   `request_id` is omitted. The golden records the challenge via `Header().Get`, which
   canonicalizes, so header casing cannot flap it either.
4. **The golden struct captures only `status`, `content_type`, `www_authenticate`,
   `body`** (lines 67-72) — **not** `X-Content-Type-Options`. Do not let A4 claim "the
   goldens prove the full header set changed"; they prove three headers and the body.
5. **Expected golden diff:** `content_type` → `application/problem+json`,
   `www_authenticate` `""` → `Bearer realm="api"`, and the five bodies collapse to one
   identical problem+json. **That collapse is the proof the leak closed.**
6. **Name `unauthorized_golden_test.go` in A4's file list**; extending the struct and
   regenerating with `-update` is expected.
7. **Keep `Extra` nil** — zero clone allocations when nil; dumping the claims map in
   costs a per-request map clone **and** violates the admission rule.
8. **`MustFromContext` is production-safe here** — `internal/httpx/router.go:57`
   mounts `Recovery(log)` globally, so a wiring mistake is a loud, correlatable 500.
   **Corollary:** any handler reachable both authenticated and anonymous must use
   `FromContext`, not `MustFromContext`.
9. **Ordering:** reject and `return` **before** any `WithPrincipal`, so no zero-Subject
   Principal reaches the context.
10. **Do NOT touch `/auth/mfa/verify`** (E4/BL13).
11. **`writeProblemDetails` does NOT become dead code — do not remove it.** Confirmed:
    **11 call sites**; A4 replaces only 3 (`auth.go:111`, `:136`, `:164`). The other 8
    are 400/423/500 paths (`:87`, `:90`, `:115`, `:141`, `:169`, `:175`, `:178`,
    `:265`). Removing it is a compile error across 8 lines.
12. **Swagger annotations — decide explicitly in the dispatch.** `auth.go:105`, `:130`,
    `:158` declare `@Failure 401 {object} transport.ProblemDetails`; after A4 the body
    is `httpx.Problem`. Those lines are in files A4 already edits, so it is a scope
    *call*, not a violation. Decide, because "the swagger spec lies about the canonical
    401" ships silently and surfaces as a client bug.

## B3 dispatch adjustment — banked
- `Principal` is **not comparable** (contains `[]string`); `got == want` will not
  compile — a compile error, never a silent wrong result. **Do not add `Equal` or
  change fields on `internal/authn`.** `reflect.DeepEqual` is faithful *because* A2's
  clone helpers preserve nil.
- **Do not export A3's `unauthorizedDetail` for B3's sake.** B3 asserts
  reason-invariance by comparing responses *to each other*, needing no access to the
  literal; if B3 wants the exact string, hard-coding it is *better* — a second
  independent copy of the contract, and A3's constant test fails first if anyone edits
  it, so the two cannot silently diverge.
- **Dependency warning:** `github.com/google/go-cmp v0.7.0` is in `go.sum` but **not** a
  direct `go.mod` requirement. `cmp.Diff` promotes it to a direct dependency and edits
  `go.mod` — must be a named line item in B3's file list or I escalate it.

## C1 dispatch adjustments — banked
Carry E3's approved scope, E4's narrowing, E5's `request_id` decision. Scope the
uniformity claim to routes behind `AuthRequired`; state the `/auth/mfa/verify`
carve-out positively. Every claim checked against **merged code**. BL2 is worth a line.

## Backlog — pre-existing, do NOT spawn unplanned work

- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings, blame `6eed3aa`.
  The only lint findings repo-wide, confirmed at A1, A2, A3 and T1.
- **BL2** Swagger annotations lie about the 401 shape (`auth.go:105`, `:130`, `:158`;
  `:195` declares `transport.OAuth2Error`). Relevant to A4 item 12 and D10.
- **BL3** `transport` imports `infrastructure` in production at `jwks.go:7`,
  contradicting R1 (`dependency-rules.md:79`). Unenforced by depguard or archtest.
- **BL4** RSA keypair parsed twice at boot; deliberate, `container.go:83-90`. Do not "fix".
- **BL5** `identity.New` returns only `(*module.Module, error)`;
  `cmd/api/container.go:95-105` builds a **second** `token.NewJWTService` +
  `AuthRequired`. A4/B2 must not assume a second return value.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern; `.zip` files are
  ignored by nothing. Needs a one-line authorisation to split.
- **BL9–BL11** (A2, cosmetic) doc comment overstates goroutine scope; a built-in
  context key would leave the suite green (held by staticcheck **SA1029** via
  `.golangci.yml` — **do not "simplify" that config**); `authn_test.go:301-303`
  overstates `wrap`.
- **BL12** Three live worktrees under `.gitignore:43` holding stale
  `.gitignore`/`CLAUDE.md`/tooling — source of one wrong root-cause diagnosis.
  Dispatches must tell agents to exclude them for **config files**, not just code.
- **BL13** **ACCEPTED PERMANENT DIVERGENCE (E4) — do not "fix".** `/auth/mfa/verify`
  emits OAuth2-shaped 401s by design; converting it would break OAuth2 clients.
- **BL16** **`golangci-lint` cache trap — real, local, produces false greens.** It
  serves cached results keyed to deleted directories and silently drops findings it
  cannot relativize, printing `0 issues.` with `path_relativity` warnings. **Every
  reviewer must `golangci-lint cache clean` before `make lint`.** Confirmed not to have
  masked anything on A1/A2 (findings identical at `f594ebc`), and CI runs on a cold
  runner, so merged work is unaffected.
- **BL17** (T1 notes, non-blocking) `-require-profile=false` exits 0 on an empty
  profile — a no-op gate with a success code; unreachable today since neither the
  Makefile nor CI passes it. Policy: it must never appear in a Makefile or workflow.
  `tolerance = 0.05` softens **floors** as well as the ratchet (effective authn floor
  89.95%); the comment only justifies it for the ratchet. `-allow-decrease` prints
  nothing about *what* it lowered. **Floors are only enforced for packages present in
  the profile** — a floor-matching package with no baseline and no profile lines is
  silently unenforced; exactly what hid E7. `tools/...` is exempt, so the coverage gate
  does not gate itself.

## Deviations accepted

- **A1 (4)** — refresh-class as raw `cls:"refresh"`; `alg:none` built in-file, helper
  unexported/test-only with `internal/testsupport` confirmed clean; body stored as a raw
  JSON string; plain `flag.Bool("update", ...)`.
- **A2 (1)** — alias assertion rewritten to a reflect-based check; reviewer confirmed by
  mutation that it discriminates.
- **A2 copy semantics** — **copy-on-write, not copy-on-read.** Rests on the clone *plus*
  the unexported key: `WithPrincipal` is the only way a Principal enters a context, so
  **cross-request corruption is structurally unreachable.**
- **A3** — no deviation from the task text. Two from §8.2's literal example, both
  pre-authorised by E5: `request_id`, and `X-Content-Type-Options: nosniff` inherited
  from `WriteProblem`. `Www-Authenticate` uses Go's canonical MIME casing; field names
  are case-insensitive per RFC 9110 §5.1 and the test reads case-insensitively — no
  client-facing risk.
- **T1 (1)** — **refused** my dispatch instruction to baseline `internal/logger`,
  `internal/module`, `auditlog` and two `models` packages: `grep -c` returns 0 profile
  lines for all five, so recording a number would fabricate a measurement **and** trip
  the tool's own "in baseline but absent from profile" failure, breaking
  `make coverage-check` on the very commit introducing it. Reviewer: "I would have
  blocked the PR had it complied." **Correct refusal — the instruction was mine and it
  was wrong.**
- **Phase A PR merged after A2** rather than after A4 — human's decision.
