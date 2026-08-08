# Task Status Board

Maintained by team-lead. This file is the lead's ONLY writable file and the
single source of truth for effort state. Update on every state change.

States: pending | dispatched | in-review | blocked | merged | escalated

## Tasks

| ID  | Owner                | State | Branch / PR | Review | Updated |
| --- | -------------------- | ----- | ----------- | ------ | ------- |
| A1  | test-guardian        | **merged** | PR #16 (`9e50896`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A2  | domain-engineer      | **merged** | PR #16 (`6288dcd`) | APPROVE-WITH-NOTES | 2026-08-08 |
| T1  | test-guardian        | **merged** | PR #18 (`8b1b185`) | APPROVE-WITH-NOTES | 2026-08-08 |
| A3  | transport-engineer   | **merged** | PR #17 (`4a969f2`) | APPROVE-WITH-NOTES | 2026-08-08 |
| **A4** | transport-engineer | **approved — awaiting merge** | `feat/A4-identity-authn` → **PR #20** (`5a0b56e`) | APPROVE-WITH-NOTES | 2026-08-08 |
| B1  | test-guardian        | next — dispatch after A4 merges | - | - | 2026-08-08 |
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
| D10 | platform-engineer    | pending | - | - | 2026-08-08 |

**Critical path:** merge A4 (#20) → B1 → B2/B3 → B4 → C1.

**I cannot merge.** `gh pr merge` and direct pushes to `main` are blocked by the
sandbox classifier. Branch pushes and PR creation work. A human merges.

## Merge log

| PR | Merged as | Contents |
| --- | --- | --- |
| #15 | `d63200b` | `6eed3aa` package rename — the Phase A base |
| #16 | `f594ebc` | A1 (`9e50896`), A2 (`6288dcd`), `c5c25bd` (human's docs/roster commit) |
| #17 | `d74ec67` | A3 — `httpx.Unauthorized` |
| #18 | `0c0a1cf` | T1 — `tools/coverage` |
| #19 | `f6e93ff` | board |
| **#20** | **open** | **A4 — identity implements the contract** |

## Phase A — status after A4

**Coherent; not complete, and the plan already knows it** (arch-reviewer's assessment).

**Enforced end to end today:** identity's middleware is the single writer of
`authn.Principal`; every rejection path funnels through `httpx.Unauthorized`, which
**structurally cannot take a reason**; all five goldens are one byte-identical body
plus a by-value contract test; the old context key exists nowhere in the tree; the
transport→kernel boundary is machine-enforced for production code (reviewer proved it
bites in both directions); README and `dependency-rules.md` match the enforced rule in
the same commit. **The atomicity requirement — the one thing that could have gone
wrong — was met.**

**Holes Phase B must close, in order of importance:**
1. **`modules/user` is still outside the contract (B2).** Its transport imports nothing
   from `internal/**` and reads no principal. Today the contract has one producer and
   effectively one consumer. **This is the substantive gap.**
2. **There is a conformance *instance*, not a conformance *guarantee* (B3).**
   `TestCanonical401Contract` pins *identity's* 401. Nothing forces a second
   `authn.Middleware` implementation to emit the same shape, and nothing asserts the
   success path at the transport layer at all (same hole as P1, seen from the other side).
3. **`internal/authn` has no import fence (B4).** Any package may import it today.
4. **Test-side depguard (N1, owned by B1).**

## Escalations — RESOLVED

**E1** — `tools/coverage` never committed; human authorised T1, merged in #18. Root
cause was the bare `coverage/` pattern (arch-reviewer right; my "correction" wrong — I
read the working file, not the committed blob). **Standing rule adopted:** assertions
about repo configuration cite `git show <ref>:<path>`, never the working file.
**E2** — SPI rationale holds as stated; A4 unblocked. Carry into B2:
`modules/user/module.go:42` is `AuthMiddleware`, not `Config.Auth`; the rename is free
because `Middleware` is an alias.
**E3** — corrected changelog scope approved (see C1 note).
**E4** — uniformity claim narrowed to the middleware-protected surface;
`/auth/mfa/verify` is an accepted permanent divergence (BL13). Honoured by A4 —
reviewer confirmed its two 401s lie outside every diff hunk.
**E5** — `request_id` accepted in the canonical body.
**E6** — `docs/plans/` tracked and on `main`.

## Escalations — OPEN

### E7 — OPEN — `auditlog` has no tests and trips a pre-existing 60% floor
`modules/identity/infrastructure/auditlog`: 8 functions, no test file, matches the
**pre-existing** `modules/*/infrastructure/...` floor. Invisible today because CI never
reaches the coverage step (E8); becomes an immediate CI failure the instant E8 is fixed.
Reviewer confirmed it is the **sole** such failure. **Needs an owner. The fix is a test
or a reasoned `exempt` entry — not a lower floor.**

### E8 — OPEN — CI has been red for 12+ consecutive runs, all pre-existing
1. **`internal/db` — `TestCaseInsensitiveIdentifiers_RefusesCollidingData` fails**
   (unique-constraint violation). Sits **upstream of the coverage step**, so guardrails
   7/8 stay unverifiable in CI even with T1 merged. Highest-value fix in the tree.
2. **The CI `golangci-lint` job cannot start** (built with go1.24, targets 1.25.0) — it
   has been producing **no findings at all**. Local `make lint` is the repo's only lint
   coverage. A silent hole.
3. **`govulncheck` fails** on stdlib advisories. Likely a toolchain bump.
**Needs owners, or §0.4's "CI is the arbiter" is fiction.**

## B1 dispatch adjustments — banked (next task)

- **N1 — close the depguard test hole.** A4's `transport-kernel-access` rule exempts
  `**/*_test.go` entirely, so transport tests may import **anything** under `internal/**`
  (reviewer proved this: a probe test file importing `internal/db` and `internal/logger`
  drew zero findings). Justified for A4 and documented, but the worker's stated dichotomy
  was false. **Add a sibling rule** — needs no revert later, keeps the production rule at
  exactly two packages:
  ```yaml
  transport-test-kernel-access:
    list-mode: lax
    files: ["**/modules/*/transport/**/*_test.go"]
    allow: [internal/authn, internal/httpx, internal/testsupport]
    deny: [pkg: github.com/sujanto-gaws/kopiochi/internal]
  ```
  B1 is the natural owner because B1 makes `testsupport.FakeAuth` mandatory for every
  transport test. **`list-mode: lax` is load-bearing** — under the default `original`, a
  non-empty allow list forbids everything not on it, including chi.
- B1's own scope is unchanged: `FakeAuth(subject)` and `FakeAuthPrincipal(p)` in
  `internal/testsupport`, no JWT/keypair imports in that file.

## B2 dispatch adjustments — banked
- `modules/user/module.go:42` is `AuthMiddleware`, not `Config.Auth` (E2).
- `modules/user/transport` currently imports **nothing** under `kopiochi/internal` and
  reads no principal — B2 is where the contract gains its second real consumer.
- Keep the nil fail-closed check. Keep exactly one end-to-end test mounting the real
  identity middleware — **do not delete integration coverage.**
- **N5 (cosmetic):** normalise the middleware field to `authn.Middleware` here; identical
  type (alias), zero behavioural difference. `AuthHandler.authMW` (`auth.go:39`) is the
  analogous case in identity.

## B3 dispatch adjustments — banked
- `Principal` is **not comparable** (contains `[]string`); `got == want` will not compile.
  **Do not add `Equal` or change fields on `internal/authn`.** `reflect.DeepEqual` is
  faithful because A2's clone helpers preserve nil.
- **Do not export A3's `unauthorizedDetail`.** B3 asserts reason-invariance by comparing
  responses *to each other*; if it wants the literal, hard-coding it is *better* — a
  second independent copy of the contract, and A3's constant test fails first if anyone
  edits it, so the two cannot silently diverge.
- **B3 also closes P1** — its "valid → handler reached, principal present, Subject
  matches" assertion is the missing success-path guard. If B3 slips, P1 becomes a real
  exposure and should be patched directly.
- **Dependency warning:** `github.com/google/go-cmp v0.7.0` is in `go.sum` but **not** a
  direct `go.mod` requirement. `cmp.Diff` promotes it — must be a named line item in B3's
  file list or I escalate it.

## B4 — respecify before dispatch
T1 consumed only the first clause of B4's coverage sentence. B4 still owns the more
important half: **the archtest rule** (only `modules/*`, `internal/httpx`,
`internal/testsupport`, `internal/authn/authntest` may import `internal/authn`) and
**the deliberate-violation proof**. Plus: **`internal/authn/authntest` matches NO policy
pattern** (the `internal/authn` floor is non-recursive — 2 segments vs authntest's 3 —
and `exempt` lists `internal/testsupport` but not `authntest`), so it would land
unfloored and unbaselined; B4 decides exempt vs floor, arguably a floor since B3's
acceptance is "the suite itself has tests" and an untested conformance suite passes
everything. And **re-verify** `internal/authn: 100` after B2/B3 — T1's number is a
snapshot at `f594ebc`.

## C1 dispatch adjustments — banked
- **E3 approved scope, verbatim:** lead with the transport-level break — content-type
  `text/plain` → `application/problem+json`; body → RFC 9457 object **with no `error`
  member at all** (zero field-name overlap, so `body.error` clients break outright
  rather than degrade); `WWW-Authenticate: Bearer realm="api"` now sent where **none was
  ever sent**. Keep "key off `status`, never `detail`", augmented not replaced. **Do not
  repeat §8.1/§8.3's "collapses several unstable shapes into one"** — unsupported.
- **E4:** scope the uniformity claim to routes behind `AuthRequired`; **state the
  `/auth/mfa/verify` carve-out positively.**
- **E5:** document `request_id` as an RFC 9457 extension member.
- **N3 — the companion doc is wrong in two places and seeded a real deviation:**
  `authn-spi-impact-analysis.md:86` contains **non-compiling sketch code**
  (`Scopes: claims.Scopes` against a `Claims` type with no such field), and line 93 says
  `FromContext` where the plan says `MustFromContext`. C1 should correct both or mark
  the file a superseded design sketch. **Still needs the human's word** — I cannot edit
  companion docs.
- Every claim checked against **merged code**.

## Backlog — pre-existing, do NOT spawn unplanned work

- **P1 / BL18** `cmd/api/protected_routes_test.go:157` — `TestProtectedRoute_ValidTokenPassesTheGuard`
  asserts only `rec.Code != 401`, and `httpx.Recovery` converts a panic to 500, so a
  `MustFromContext` regression **passes the only e2e control on the success path**.
  Pre-existing. Scheduled to close in B3.
- **BL1** `internal/db/schema_test.go:31,43` — two errcheck findings (blame `6eed3aa`);
  the only lint findings repo-wide, confirmed at A1–A4 and T1.
- **BL3** `modules/identity/transport/jwks.go:7` imports `infrastructure/token` in
  production, contradicting R1. **A4's new depguard rule does not catch it** — it denies
  `internal/**`, not sibling layers. Still unenforced.
- **BL4** RSA keypair parsed twice at boot; deliberate (`container.go:83-90`). Do not "fix".
- **BL5** `identity.New` returns only `(*module.Module, error)`; `cmd/api` builds a
  **second** `token.NewJWTService` + `AuthRequired`.
- **BL8** `.gitignore:22` `*.zipcoverage.out` — a fused pattern; `.zip` files are ignored
  by nothing.
- **BL13** **ACCEPTED PERMANENT DIVERGENCE (E4) — do not "fix".** `/auth/mfa/verify`
  emits OAuth2-shaped 401s by design.
- **BL16** **`golangci-lint` cache trap — produces false greens.** It serves cached
  results keyed to deleted directories and silently drops findings it cannot relativize.
  **Every reviewer must `golangci-lint cache clean` before `make lint`.**
- **BL17** (T1) `-require-profile=false` exits 0 on an empty profile — must never appear
  in a Makefile or workflow. `tolerance = 0.05` softens **floors** as well as the ratchet.
  **Floors are only enforced for packages present in the profile** — exactly what hid E7.
  `tools/...` is exempt, so the coverage gate does not gate itself.
- **BL19 (N2)** `modules/identity/transport` baseline left at 15.8 while actual is 16.7.
  Correct per the tool's contract, but a later PR can silently give back 0.9 points. Run
  `make coverage-update` at the end of Phase B rather than per task.
- **BL20 (N4)** `docs/architectures/adr/005 - Module Boundaries…md:60` still shows
  `transport → application, chi`. ADRs are point-in-time records; if the board wants them
  live, that table now understates the layer rule.
- **BL21 (N6)** `TestCanonical401Contract`'s reason-vocabulary sweep scans the whole body
  including `instance`, and the list contains `"token"` — a future protected route whose
  path contains "token" (e.g. `/auth/token/revoke`) would fail spuriously. Exclude
  `instance` from the scan when convenient.
- **BL22 (P3)** `JWTService.Validate` (`jwt.go:154`) does not require a `sub` claim.
  Harmless now that A4 rejects empty-subject tokens at the middleware and no issue path
  can produce one, but the defence is one layer further out than it could be.
- **BL23** `docs/architectures/04-security/token-architecture.md:109` documents a
  `UserNameContextKey` that exists nowhere in the tree. Stale doc.

## Deviations accepted

- **A1 (4)**, **A2 (1)**, **A2 copy-on-write semantics**, **A3 (0 from task text; two
  from §8.2's example, pre-authorised by E5)** — see PR history; all adjudicated.
- **T1 (1)** — **refused** my instruction to baseline five unmeasurable packages.
  Reviewer: "I would have blocked the PR had it complied." **Correct refusal — the
  instruction was mine and it was wrong.**
- **A4 (2), both adjudicated CORRECT by arch-reviewer:**
  1. **`Scopes` left nil.** `domain.Claims` (`service.go:33-42`) has no `Scopes` field —
     only `Scope string`, hardcoded to `"access"` by `IssueAccessToken` (`jwt.go:89`), a
     duplicate of the class. `domain.Class`'s doc names `"scope"` as the "convention-only
     field" not to be trusted. `internal/authn:47-49` says **nil and empty mean the same
     thing**, so nil is a conforming value — not a contract deviation. Roles/Permissions
     were rejected as a source: mapping them into a field named `Scopes` would be a
     semantic lie and violate the two-consumer admission rule.
  2. **New empty-`sub` rejection path.** *Entailed* by A4(3): `MustFromContext` panics on
     empty Subject, so Logout's old emptiness check had to move ahead of the context
     write or a bad credential becomes a 500. Cannot reject anything that previously
     succeeded (`uuid.String()` is never empty, even for `uuid.Nil`). A strict tightening
     — MFASetup and MFAVerifySetup previously passed empty-subject tokens straight
     through to `SetupMFA("")`.
- **A4 scope**, pre-authorised by me: `modules/identity/domain/service.go` (the key was
  *defined* there) and `unauthorized_golden_test.go` (by-value assertions). Swagger
  annotations updated to `internal_httpx.Problem` — swag's mangled package name; the
  obvious `httpx.Problem` breaks `make swagger-docs` because swag resolves against the
  *file's* imports and `auth.go` does not import `httpx`.
- **Phase A PR merged after A2** rather than after A4 — human's decision.
