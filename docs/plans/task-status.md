# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

Phase A is ONE PR with sequenced commits on `feat/A1-golden-401-capture`.
"merged" for A1–A4 therefore means "accepted into the phase branch"; the PR
itself merges to `main` after A4 clears the gate.

## Tasks

| ID  | Owner                | State      | Branch                     | Review              | Updated    |
| --- | -------------------- | ---------- | -------------------------- | ------------------- | ---------- |
| A1  | test-guardian        | merged     | feat/A1-golden-401-capture | APPROVE-WITH-NOTES  | 2026-08-08 |
| A2  | domain-engineer      | merged     | feat/A1-golden-401-capture | APPROVE-WITH-NOTES  | 2026-08-08 |
| A3  | transport-engineer   | blocked    | -                          | - (blocked on E5)   | 2026-08-08 |
| A4  | transport-engineer   | blocked    | -                          | - (blocked E2/E4/E5)| 2026-08-08 |
| B1  | test-guardian        | pending    | -                          | -                   | -          |
| B2  | transport-engineer   | pending    | -                          | - (see E2)          | 2026-08-08 |
| B3  | test-guardian        | pending    | -                          | - (see Q1 note)     | 2026-08-08 |
| B4  | test-guardian        | blocked    | -                          | - (blocked on E1)   | 2026-08-08 |
| C1  | docs-scribe          | pending    | -                          | - (see E3, E4)      | 2026-08-08 |
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
| D10 | platform-engineer    | blocked    | -                          | - (blocked on E1)   | 2026-08-08 |

**Branch history (linear, no amend/rebase/squash — verified via reflog):**
`6eed3aa` (base) → `9e50896` (A1) → `6288dcd` (A2).

**A1 detail:** commit `9e50896`, 6 files (golden test + 5 goldens). No blocking
findings. Reviewer re-verified the capture in a scratch clone: all five inputs are
genuinely distinct and fail at the intended stage (refresh-class reaches the class
gate with signature/iss/aud/exp all passing; wrong-alg rejected specifically on
`alg`), with a positive control proving the rig can accept a valid token.
Deterministic over `-count=5`, clean tree after. All four declared deviations
accepted; `alg:none` helper confirmed unexported, test-only, absent from
`internal/testsupport`.

**A2 detail:** commit `6288dcd`, exactly 2 files (+484). No blocking findings.
100.0% statement coverage, all branches. Reviewer verified independently rather than
trusting claims: `go list -deps` shows **zero** non-stdlib transitive dependencies;
admission rule present byte-verbatim; `Middleware` is textually a `=` alias.
**Reviewer ran its own mutation testing in an isolated scratch module** (repo
untouched): defined-type `Middleware`, clones removed, nil-guards dropped, panic
messages collapsed, `Subject == ""` check deleted — **all five mutations failed the
suite** except the built-in-context-key mutation (see N2). No coverage padding.
Nothing imports the package yet, so the commit cannot change behavior.

## Sequencing decisions (operational, not plan changes)

- **Phase A base = `refactor/module-package-names` @ `6eed3aa`**, not `main`.
  Evidence: `git log main..refactor/module-package-names` = 1 commit
  ("refactor: name module packages for their layer, not their module");
  `git log refactor/module-package-names..main` = empty (no divergence,
  fast-forwardable). The plan's paths assume the post-refactor layout —
  `modules/identity/transport/middleware.go` is `package transport` at this HEAD.
- **Dirty tree**: `Dockerfile`, `README.md`, `go.mod` modified; `.claude/agents/*`
  deleted/added; `docs/plans/` untracked. Workers commit ONLY their task's file list
  by explicit path. Verified clean for A1 and A2. (The `go.mod` delta is only
  `go 1.25.0` → `go 1.25.12`, a toolchain auto-bump — unrelated, correctly uncommitted.)
- **D1/D2 held.** The graph shows no edges into them, but the plan sanctions them
  "in parallel with **Phase B**", and Phase B has not started. Not dispatching them
  during Phase A without the human's word — offered as an option in the status report.

## Escalations

### E1 — OPEN — `tools/coverage` has never been committed; guardrails 7 and 8 are unexecutable repo-wide
**Raised by:** test-guardian (A1) as a formal stop condition. **Root cause found by
arch-reviewer.** **Blocks:** guardrail 8 for every task; **B4 and D10 outright**.
**Evidence:** `Makefile:87,91` invoke `go run ./tools/coverage -profile coverage.out`;
`git ls-files tools` lists only `archtest` and `schemacheck`;
`git log --all --diff-filter=A -- tools/coverage` is empty. Confirmed again at A2:
`git ls-files | grep -iE "coverage|policy"` → **empty**, so there is no committed
coverage baseline or policy file anywhere in the repo.
**Root cause:** `git check-ignore -v tools/coverage/main.go` → `.gitignore:33:coverage/`.
The bare `coverage/` pattern matches a directory at **any depth**, so
`git add tools/coverage` silently did nothing. A complete implementation
(`main.go`, `main_test.go`, `policy.json`) is stranded uncommitted at
`.claude/worktrees/phase-3-consolidate/tools/coverage/`.
**Proposed fix (reviewer):** anchor the pattern to `/coverage/`, then
`git add -f tools/coverage`. **Not executing it** — new task touching `.gitignore`
plus importing an unreviewed tool = scope change.
**Decision needed:** (a) authorise a task to rescue the stranded tool, (b) amend
guardrails 7/8 to drop `make coverage-check` and respecify B4/D10, or (c) other.
B4 and D10 are specified entirely in terms of `tools/coverage/policy.json`.
**Interim in force:** workers report coverage via `go test -cover`, forbidden from
creating the tool, editing `policy.json`, or touching `.gitignore`. A reporting
stopgap only — NOT an accepted deviation, and it lowers no coverage bar. A2 hit
100% under it.

### E2 — OPEN — `modules/user` does NOT read identity's context key; a load-bearing premise of the analysis is false
**Raised by:** test-guardian (A1) probe finding B3. **Affects:** A4 scope, B2, the
stated rationale for the SPI.
**Evidence (repo-wide, `.claude/worktrees` excluded):**
- Defined: `modules/identity/domain/service.go:10` (`claimsContextKey`), `:12-13` (`var ClaimsKey`).
- Written: `modules/identity/transport/middleware.go:30`.
- Read: `modules/identity/transport/auth.go:109` (Logout), `:134` (MFASetup), `:162` (MFAVerifySetup). **All three read sites.**
- **Readers outside `modules/identity`: NONE. Readers in tests: NONE.**
- `modules/user/transport/user.go` imports only `modules/user/domain` and derives ids from the URL path; `modules/user/module.go:42` takes `AuthMiddleware func(http.Handler) http.Handler` and never inspects a principal.
**Contradicts:** analysis §1, §4, §10 risk 1 (currently unrealizable).
**Consequence:** A4's atomic migration is 4 sites in 2 files, all inside
`modules/identity` — far smaller than planned. The SPI still pays forward (the edge
appears the moment B2/notification adds ownership checks), but the plan's framing of
the migration is wrong.
**Decision needed:** confirm the SPI rationale still holds as stated before A4/B2.
**Also:** `modules/user/module.go:42` is named `AuthMiddleware`; B2's text says
`Config.Auth`. To be reconciled in B2's dispatch. Reviewer notes the rename is free:
`httpx.NewRouter` is already `mw ...func(http.Handler) http.Handler` (`router.go:33`),
and because `Middleware` is an alias, `...authn.Middleware` is the *identical* type —
zero call-site churn, no conversions.

### E3 — OPEN — current 401s are near-uniform, not inconsistent; changelog scope changes
**Raised by:** test-guardian (A1) probe finding B2; confirmed by arch-reviewer.
**Affects:** C1 changelog, §8.1/§8.3 framing, A4's acceptance wording.
**Captured reality (all five cases):** status 401 uniform; `Content-Type:
text/plain; charset=utf-8` uniform (from `http.Error`); **`WWW-Authenticate` absent
on every path** — it does not "vary by path" as §8.1 predicted, it is never sent at
all; body has exactly **two** values, `{"error":"missing token"}` (missing header)
vs `{"error":"invalid token"}` (the other four). Cases 2–5 are already byte-identical.
**Consequence:** §8.1/§8.3's "collapses several unstable shapes into one" is not
supported — it collapses two strings into one, and a mutual-equality check proves
much less than the plan implies. The real client-visible break is **larger** than a
`detail` string: `Content-Type` `text/plain` → `application/problem+json`; body
`{"error":"…"}` → RFC 9457 object **with no `error` member at all** (zero field-name
overlap — any client matching on `body.error` breaks outright);
`WWW-Authenticate: Bearer realm="api"` appears where none was ever sent.
**Decision needed:** approve the corrected changelog scope for C1.

### E4 — OPEN — a fifth, reachable 401 shape sits outside A4's file list (scope change)
**Raised by:** test-guardian (A1) probe finding B4. **Affects:** A4 scope, C1's claim.
**Evidence:** `POST /auth/mfa/verify` is mounted deliberately OUTSIDE the middleware
(`modules/identity/transport/auth.go:291`) and answers 401 itself at `:198-203` and
`:212-214` via `writeOAuth2Error` (`helpers.go:60`): `Content-Type: application/json`,
body `{"error":"invalid_request","error_description":"Missing mfa token"}`. A real,
reachable, mounted route. A4's file list covers only `middleware.go` and the
protected handlers, so as written A4 leaves it untouched — and the changelog claim
"401 responses are uniform problem+json" would then be **false**.
**Decision needed:** widen A4 to cover `/auth/mfa/verify`, or narrow the changelog
claim to the middleware-protected surface. The OAuth2 error shape may be deliberate
for OAuth2 conformance on that endpoint — exactly why I am not deciding it.

### E5 — OPEN — canonical 401 body will carry a `request_id` member not in §8.2
**Raised by:** test-guardian (A1) probe finding B5. **Affects:** A3 (blocked), A4
goldens, C1.
**Evidence:** `internal/httpx/problem.go:36`
`func WriteProblem(w, r, status int, typ, title, detail string)` already fills
`Instance` from `r.URL.Path` (`:42`) and `RequestID` from `chimw.GetReqID` (`:43`);
`Problem` (`:21-28`) carries `RequestID` as a non-RFC extension member (`omitempty`).
**The contradiction is internal to the plan:** A3's own task text orders "reuse the
existing problem+json writer… do not hand-roll JSON if a helper exists", and doing
exactly that produces a body §8.2 says is emitted "exactly" without `request_id`.
Per my mandate, plan/companion-doc contradictions are escalated, not resolved by me
— which is why A3 stays blocked even though arch-reviewer confirms A3 has **no**
technical dependency on A2 and could otherwise start immediately.
**Decision needed:** confirm `request_id` is acceptable in the canonical body (i.e.
§8.2's example is illustrative, not literal). **This is the single cheapest unblock
on the board — it releases A3 by itself.**
**Dispatch facts banked:** `WriteProblem` does not set `WWW-Authenticate`, so
`Unauthorized` must set it before calling. Under a bare `chi.NewRouter()` the field
is empty/omitted; under the real `httpx.NewRouter` it is present — goldens differ by
harness, which A4 must control for.

### E6 — OPEN (informational) — the plan itself is untracked
`docs/plans/` is untracked (`??`). The document defining every acceptance criterion
in this effort — and this board — is not in git. **Decision needed:** commit
`docs/plans/` before the Phase A PR merges. Not in any task's file list; I have not
committed it.

## A3 dispatch adjustments — banked, to apply when E5 resolves

- `internal/httpx/problem.go:36` `WriteProblem(w, r, status, typ, title, detail)` is
  the writer to reuse; it fills `Instance` from `r.URL.Path` and `RequestID` from
  `chimw.GetReqID`, and sets `X-Content-Type-Options: nosniff` (`:47`).
  `ProblemContentType` constant at `problem.go:13`.
- `Unauthorized` must set `WWW-Authenticate: Bearer realm="api"` **itself, before**
  calling `WriteProblem` — the writer does not set it.
- No import edge to `internal/authn` is needed or wanted: `Unauthorized(w, r)` takes
  only `w` and `r`. Reviewer confirms A3 is fully independent of A2's delivery.
- `detail` must be a package-level constant used verbatim (task acceptance), guarding
  against future per-reason drift.

## A4 dispatch adjustments — banked from A1 + both gates

1. **The "byte-identical except `instance`" criterion is vacuous as written.** All
   five cases hit the same route (`POST /api/v1/auth/logout`,
   `unauthorized_golden_test.go:56`), so `instance` is identical too. Reviewer
   recommends a **sixth case on a second protected route**
   (`POST /api/v1/auth/mfa/setup`) so `instance` is shown to vary while everything
   else holds. Needs the human's nod (scope).
2. **Assert positively, not by mutual equality.** Only one divergence exists to
   collapse (E3). Acceptance must assert by value: problem+json content-type,
   `WWW-Authenticate: Bearer realm="api"` present, RFC 9457 members.
3. **`compareGolden` (`unauthorized_golden_test.go:185`) compares marshalled bytes
   with no normalization hook.** Any volatile member (timestamp, trace id, or E5's
   `request_id`) makes the goldens flap. Keep the body free of volatile members or
   add a scrubber.
4. **Name `unauthorized_golden_test.go` explicitly in A4's file list** and state that
   extending the `unauthorizedGolden` struct and regenerating with `-update` is
   expected — otherwise the agent may treat it as untouchable.
5. **The golden rig is a bare `chi.NewRouter()` + `h.Routes`, not the production
   chain.** If the canonical writer reads anything only `BuildApp` populates, the
   golden pins a degraded response. Verify the writer is self-contained or mount
   through `internal/httpx.Mount`.
6. **Goldens record the response, not the reason.** Four files read
   `{"error":"invalid token"}`; nothing committed distinguishes them. Distinctness
   was verified out-of-band only. Verifier-level assertions + a positive control are
   not called for by the plan — needs approval before anyone adds them.
7. **Keep `Extra` nil.** Clone cost is zero allocations when `Extra` is nil. Dumping
   the whole claims map into `Extra` "so consumers have it" adds a per-request map
   clone **and** violates the admission rule. The plan's `Principal{Subject, Scopes}`
   is also the cheapest spelling.
8. **Panic contract is production-safe here — keep it that way.**
   `internal/httpx/router.go:57` mounts `Recovery(log)` globally, third in the stack,
   emitting problem+json 500 with a structured log. So a wiring mistake is a loud,
   correlatable 500, not a silent empty account id. A4 must not add a route path that
   bypasses `NewRouter`'s stack. **Corollary:** any handler reachable **both**
   authenticated and anonymous must use `FromContext`, not `MustFromContext`.
9. **Ordering:** rejection paths must call `httpx.Unauthorized` and `return`
   **before** any `WithPrincipal`, so no zero-Subject Principal ever reaches the
   context. A violation trips A2's second panic message loudly — the payoff for
   keeping the two messages distinct.

## B3 dispatch adjustment — banked

- `Principal` is **not comparable** (confirmed at compile time: struct containing
  `[]string`). `got == want` will not compile in table tests. This is a compile
  error, never a silent wrong result. **Do not add `Equal` or change fields on
  `internal/authn`** — that is an unsettled new export on the SPI and would be a
  review finding. If friction is real, an `authntest.Equal`/`RequireEqual` helper in
  B3 is the right home. `reflect.DeepEqual` is faithful here *because* A2's clone
  helpers preserve nil — a round-tripped Principal compares deep-equal, and A2's
  tests pin that so it cannot drift.
- **Dependency warning for B3:** `github.com/google/go-cmp v0.7.0` is in `go.sum` but
  **not** a direct requirement in `go.mod`. Using `cmp.Diff` promotes it to a direct
  dependency and edits `go.mod`. If wanted, it must be a named line item in B3's file
  list — otherwise it is out-of-scope drift and I would escalate it as a new
  dependency.

## Backlog — pre-existing issues logged by reviewers/workers (do NOT spawn unplanned work)

- **BL1** `make lint` red on pre-existing content: `internal/db/schema_test.go:31`
  (`defer sqlDB.Close()`) and `:43` (`defer sqlDB.Exec(...)`), errcheck. `git blame`
  puts both on **`6eed3aa` — the Phase A base commit itself**, which added the file.
  Every PR in this stack inherits a red lint gate, making guardrail 8 unmeetable by
  construction. Survives because `make ci` (`Makefile:292`) runs `check`, which
  excludes `lint`. Confirmed at both A1 and A2 as the *only* two findings.
- **BL2** Swagger annotations lie about the 401 shape:
  `modules/identity/transport/auth.go:105`, `:130`, `:158` declare
  `@Failure 401 {object} transport.ProblemDetails`, but the middleware returns
  `text/plain` with `{"error":…}`. A4 makes them true by accident.
- **BL3** `transport` imports `infrastructure` in production at
  `modules/identity/transport/jwks.go:7`, contradicting R1
  (`dependency-rules.md:79`). Unenforced: `.golangci.yml` depguard has only
  `domain-purity`, `application-purity`, `platform-independence`; `tools/archtest`
  checks `application → infrastructure` (`arch_test.go:234-239`) but has no transport
  rule. Two items: the violation, and the missing enforcement.
- **BL4** RSA keypair parsed twice at boot: `modules/identity/module.go:98` and
  `cmd/api/container.go:95`. Documented as a deliberate trade-off at
  `container.go:83-90`. A4/B2 must NOT "fix" this without a decision.
- **BL5** `identity.New` returns only `(*module.Module, error)` and does not expose
  its middleware; `cmd/api/container.go:95-105` constructs a **second**
  `token.NewJWTService` + `AuthRequired`. Analysis §6 is true of `user.New`'s call
  site but not of the middleware's provenance. A4/B2 must not assume a second return
  value exists.
- **BL6** `modules/identity/transport` had no test files at all before A1; package
  coverage 0% → 15.8%. Ratchet unverifiable (E1).
- **BL7** `go: no such tool "covdata"` locally — tolerated by the leading `-` on
  `Makefile:76-80`. CI is the arbiter.
- **BL8** `.gitignore:22` reads `*.zipcoverage.out` — a missing newline has fused two
  patterns, so neither `*.zip` nor `coverage.out` is ignored by that line
  (`coverage.*` on line 31 happens to cover the latter). Cosmetic; logged so it is
  not rediscovered as new. Same file as E1's root cause — fix together if E1 is
  authorised.
- **BL9** (A2, cosmetic, no commit warranted) `internal/authn/authn.go:92-98` doc
  comment calls handler mutation of shared `Scopes`/`Extra` a "same-request,
  same-goroutine bug". Same-request is right; same-goroutine is not — a handler that
  fans out (`errgroup`, background span) has multiple goroutines on the same map, a
  genuine race `-race` catches only if a test exercises the fan-out. The operative
  instruction ("treat a Principal from the context as read-only") is correct and
  sufficient; blast radius stays within one request. Strike ", same-goroutine" only
  if the file is legitimately reopened later.
- **BL10** (A2) One mutation the suite does not catch: changing the context key to a
  built-in type (`type ctxKey = string`) leaves it green, because the collision test
  uses a *defined* string type. The defense that actually holds this line is
  staticcheck **SA1029**, enabled via `.golangci.yml` `staticcheck.checks:
  ["all", "-QF1008"]`. Deliberate and documented at `authn_test.go:13-15`. **Do not
  "simplify" the staticcheck config without knowing what it holds up.**
- **BL11** (A2, cosmetic) `authn_test.go:301-303` comment claims `wrap` is the
  assignability assertion; an unnamed func type is assignable to a defined type with
  the same underlying type, so `wrap` compiles either way. The aliasing guarantee
  rests entirely on `TestMiddleware_IsATypeAliasNotADefinedType`, whose own comment
  is correct.

## Deviations accepted

- **A1 (4)** — all adjudicated and accepted by arch-reviewer:
  1. Refresh-class minted as raw `cls:"refresh"` — `domain.Class` has only
     access/mfa/id (`domain/service.go:24-32`); refresh tokens are opaque and
     server-stored. Verified to reach and fail the class gate with `ErrWrongTokenClass`.
  2. `alg:none` token built in-file with `golang-jwt` (MintToken hardcodes RS256).
     Helper unexported, test-only, `package transport`; `internal/testsupport`
     confirmed free of `UnsafeAllowNoneSignatureType`; prior art at
     `modules/identity/infrastructure/token/jwt_test.go:115`. No security weakening.
  3. Body stored as a raw JSON string, not re-encoded — preserves `http.Error`'s
     trailing `\n` and survives non-JSON bodies.
  4. Plain `flag.Bool("update", ...)` — no golden convention exists in the committed
     tree; this is the repo's first golden test.
- **A2 (1)** — accepted by arch-reviewer. The alias assertion was rewritten from
  `var mw Middleware = plain` round-tripping (staticcheck ST1023 flagged the explicit
  types as redundant — and the round-trip would have compiled for a defined type
  anyway, so it proved nothing) to a reflect-based check
  (`reflect.TypeOf((*Middleware)(nil)).Elem().Name()` — `""` for an alias,
  `"Middleware"` for a defined type) plus a `wrap` call-site helper. Reviewer
  confirmed by mutation that the replacement genuinely discriminates. Contract itself
  unchanged: no `Validate`, no `Authorizer`, no `map[string]any`, three fields, alias
  not defined type.
- **A2 copy semantics** — accepted and endorsed: **copy-on-write, not copy-on-read.**
  `WithPrincipal` clones `Scopes`/`Extra`; readers get shared references. Reviewer's
  endorsement rests on the clone *plus* the unexported key: `WithPrincipal` is the
  only way a Principal enters a context, so the stored slice/map is reachable only
  through that request's context chain. **Cross-request corruption is structurally
  unreachable, not merely unlikely.** Nil round-trips losslessly (nil stays nil,
  empty stays empty-non-nil), pinned by tests — which is also what makes
  `reflect.DeepEqual` safe for B2/B3/D7.
