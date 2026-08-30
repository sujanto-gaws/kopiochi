# Agent Implementation Plan — authn SPI + Notification Module

Target repo: `sujanto-gaws/kopiochi`
Companion docs: `authn-spi-impact-analysis.md`, `notification-module-blueprint.md`
Format: discrete tasks, each executable by one coding agent in one session/branch.
Tasks within a phase are ordered; a task must not start until its `Depends on` tasks
are merged.

---

## 0. Global guardrails — prepend to EVERY agent's context

These override anything an agent infers from general Go conventions.

1. **Read before writing.** Open and read every file you will modify, plus
   `CLAUDE.md`, the relevant module's `module.go`, and
   `docs/architectures/01-modularity/dependency-rules.md`, before the first edit.
2. **Never run `make generate`.** It is broken and adds an unused import that breaks
   `go build ./...`. Create files by hand.
3. **Dependency rules are build-enforced.** A module must not import another module.
   `internal/**` must not import `modules/**`. Only `cmd/**` and `tools/**` see both.
   If your change seems to need a forbidden import, the design is wrong — stop and
   re-read the task's design notes.
4. **Verification commands that lie:** `go test ./tools/archtest/...` returns cached
   results — always use `make arch` (it passes `-count=1`). A plain `go test ./...`
   skips DB-backed tests silently when no Postgres is available; run
   `make compose-up` first or accept that CI is the arbiter for integration tests.
5. **Layer rules inside a module:** `domain` → stdlib + `internal/platform` only;
   `application` → own domain; `infrastructure` → domain + `internal/**`;
   `transport` → application, domain, `internal/authn`, `internal/httpx`.
   **`internal/authn` is fenced to exactly four areas** — `modules/*`,
   `internal/httpx`, `internal/testsupport`, `internal/authn/authntest`
   (`tools/archtest/arch_test.go`, `authnAreas`). **`cmd/**` is not one of them**,
   so the composition root must not name `authn.Middleware` in a declaration, even
   though wiring is exactly where you would expect to. Use the structural type
   `func(http.Handler) http.Handler`, which is what `cmd/api/container.go:150`
   already does — the value still is an `authn.Middleware`, it is just never named
   as one outside the fence. Widening `authnAreas` is a decision to take
   deliberately, not a diff to wave through. See
   `docs/architectures/08-authn/README.md` for the full rationale.
6. **Secrets:** any password/credential config field uses
   `internal/platform/secret.String`, is populated from env only, never from YAML,
   and is never logged. Config `Validate()` fails closed — missing required values
   at boot are errors, not defaults.
7. **Coverage:** new `domain` packages need 90%, `application` 80%. Add entries to
   `tools/coverage/policy.json` with a reason string in the same PR that adds the
   package. `make coverage-check` must pass.
8. **Definition of done for every task:** `gofmt -l .` empty, `go build ./...`,
   `go vet ./...`, `make lint`, `make arch`, `make coverage-check`, plus the task's
   own acceptance criteria. Commit messages follow the repo's existing style.
9. **Scope discipline:** do not refactor, rename, or "improve" anything outside the
   task's file list. If you find a pre-existing bug, note it in the PR description;
   do not fix it in this branch.
10. **Stop conditions:** if an acceptance criterion cannot be met without violating
    a guardrail, or a file the task assumes exists is missing/materially different,
    stop and report the discrepancy instead of improvising.

---

## Phase A — authn SPI: contract + identity implementation (one PR, sequenced commits)

### Task A1 — Golden capture of current 401 shapes
**Depends on:** nothing. **Commit 1 of the PR — must be its own commit.**
**Files:** `modules/identity/transport/unauthorized_golden_test.go` (new),
`modules/identity/transport/testdata/golden/401_*.json` (new).
**Do:** Write a test that sends five requests to one protected route (missing header;
malformed bearer; expired token; wrong-`alg` token; refresh-class token — mint via
`internal/testsupport.MintToken`) and writes status, `Content-Type`,
`WWW-Authenticate`, and body to one golden JSON file per case. Support an `-update`
flag following the pattern used elsewhere in the repo if one exists; otherwise
standard `flag.Bool("update", ...)`.
**Accept:** test passes; goldens committed; goldens reflect *current* behavior — do
not "fix" any inconsistency you observe between cases; inconsistency is expected and
is the point of the capture.

### Task A2 — `internal/authn` package
**Depends on:** A1 merged locally (same branch).
**Files:** `internal/authn/authn.go`, `internal/authn/authn_test.go` (new).
**Do:** Implement exactly:
```go
type Principal struct {
    Subject string
    Scopes  []string
    Extra   map[string]string
}
type Middleware = func(http.Handler) http.Handler
func WithPrincipal(ctx context.Context, p Principal) context.Context
func FromContext(ctx context.Context) (Principal, bool)
func MustFromContext(ctx context.Context) Principal // panics if absent or Subject == ""
```
Unexported struct-type context key. Imports limited to `context`, `net/http`. Package
doc comment must state the admission rule: "A field enters Principal only when two or
more consumer modules need it; everything else rides in Extra or a wrapper
middleware."
**Accept:** 100% branch coverage is achievable and expected here (~90% floor
enforced); no imports beyond stdlib; `MustFromContext` panics on both absence and
empty Subject, with distinct panic messages.

### Task A3 — Canonical 401 helper
**Depends on:** A2.
**Files:** `internal/httpx/unauthorized.go`, `internal/httpx/unauthorized_test.go`
(new).
**Do:** `func Unauthorized(w http.ResponseWriter, r *http.Request)` emitting: status
401, `Content-Type: application/problem+json`, `WWW-Authenticate: Bearer
realm="api"`, RFC 9457 body with `type` `about:blank`, `title` `Unauthorized`,
`status` 401, `detail` `authentication required` (constant — never varies by
reason), `instance` = `r.URL.Path`. Reuse the existing problem+json writer in
`internal/httpx` — read it first; do not hand-roll JSON if a helper exists.
**Accept:** table test asserting every header and field; a test asserting `detail`
is a package-level constant used verbatim (guards against future per-reason drift).

### Task A4 — Identity implements the contract; canonicalize 401
**Depends on:** A2, A3.
**Files:** `modules/identity/transport/middleware.go` (edit), identity's protected
handlers that read caller identity (edit — locate by searching for the current
context-key accessor), `modules/identity/transport/testdata/golden/401_*.json`
(rewrite), `.golangci.yml` (edit), `README.md` layer table (edit),
`docs/architectures/01-modularity/dependency-rules.md` (edit).
**Do:** (1) On verify success, build `authn.Principal{Subject, Scopes}` from claims
and call `authn.WithPrincipal`; delete the old context key and its accessor entirely
— do not leave a deprecated alias. (2) All rejection paths call
`httpx.Unauthorized`. (3) Identity's own protected handlers switch to
`authn.MustFromContext`. (4) Regenerate goldens; all five cases must now be
byte-identical except `instance`. (5) depguard: allow `internal/authn` and
`internal/httpx` — and nothing else from `internal/**` — in transport layers; update
the README layer table and dependency-rules doc to match in the same commit.
**Accept:** `make arch` passes; goldens uniform; `grep -r` for the old context key
returns nothing; full `make test` green (CI runs integration).
**Atomicity warning:** old-key deletion and all reader updates must be in this one
commit — a missed reader must fail loudly via the `MustFromContext` panic in tests,
not slip through. Search the whole repo for readers, not just `modules/identity`.

---

## Phase B — consumers + conformance guarantee (one PR)

### Task B1 — `testsupport.FakeAuth`
**Depends on:** Phase A merged.
**Files:** `internal/testsupport/fakeauth.go` (+ test) (new).
**Do:** `func FakeAuth(subject string) authn.Middleware` injecting
`authn.Principal{Subject: subject}`; also `FakeAuthPrincipal(p authn.Principal)
authn.Middleware` for tests needing scopes/extra.
**Accept:** used by B2's tests; no JWT/keypair imports in this file.

### Task B2 — Refit `modules/user`
**Depends on:** B1.
**Files:** `modules/user/module.go` (Config type), `modules/user/transport/*`
(handlers + tests), `cmd/api/container.go` (only if type change requires it).
**Do:** `Config.Auth` becomes `authn.Middleware` (keep the nil fail-closed check).
Handlers obtain the caller via `authn.MustFromContext`; ownership checks compare
against `Principal.Subject`. Transport tests drop `MintToken`/keypair setup in favor
of `testsupport.FakeAuth`; keep exactly one test that mounts the real identity
middleware end-to-end if one exists today — do not delete integration coverage.
**Accept:** no import of the old key accessor (it no longer exists — compile
enforces); test files measurably simpler (no keypair generation in transport tests);
`make coverage-check` shows no regression.

### Task B3 — Conformance suite `internal/authn/authntest`
**Depends on:** Phase A.
**Files:** `internal/authn/authntest/suite.go` (new),
`modules/identity/transport/conformance_test.go` (new).
**Do:**
```go
func RunMiddlewareSuite(t *testing.T, mw authn.Middleware,
    mintValid func(t *testing.T, subject string) *http.Request,
    mintInvalid map[string]func(t *testing.T) *http.Request)
```
Asserts: valid → handler reached, principal present, Subject matches; every invalid
case → status 401, `application/problem+json`, `WWW-Authenticate` present, `detail`
identical across all invalid cases; principal absent downstream after rejection; a
panicking handler propagates (middleware must not swallow it). Wire identity in via
`conformance_test.go` supplying real minters (valid, expired, malformed, wrong-alg).
**Accept:** identity passes the suite in CI; the suite itself has tests (run it
against a deliberately broken middleware fixture and assert it fails).

### Task B4 — archtest rule + coverage policy
**Depends on:** B2, B3.
**Files:** `tools/archtest/*` (new rule), `tools/coverage/policy.json`.
**Do:** archtest: only `modules/*`, `internal/httpx`, `internal/testsupport`, and
`internal/authn/authntest` may import `internal/authn`. Coverage: entries for
`internal/authn` (90%) and any package whose floor now applies, each with a reason.
**Accept:** `make arch` (with `-count=1`, which it already passes) and
`make coverage-check` green; introduce a deliberate violation locally, confirm arch
fails, revert.

---

## Phase C — docs (one PR)

### Task C1 — Contract documentation + replacement recipe
**Depends on:** Phase B merged.
**Files:** `docs/architectures/<next-number>-authn/README.md` (new — match existing
numbering scheme, read the directory first), `BOILERPLATE.md` (edit "to add a
module"), changelog location per repo convention.
**Do:** Document: the contract and its admission rule; canonical 401 spec verbatim
(§8.2 of the impact analysis); the clean-break decision AND the deprecation-window
alternative for adopters with live clients; the replacement recipe (swap constructor
in `BuildApp`, pass `authntest`); note that consumer modules take `authn.Middleware`.
Changelog: "401 responses are uniform problem+json; clients must key off `status`,
never `detail`."
**Accept:** every claim in the doc is checked against the merged code, not the plan;
links resolve.

---

## Phase D — notification module (per blueprint; PR boundaries = D1, D2–D3, D4–D6, D7, D8–D9, D10)

### Task D1 — Migration + bun models + schemacheck
**Files:** `migrations/<ts>_create_notifications.sql` (new — SQL is in blueprint §9,
use it verbatim, timestamp via `make migrate-create`),
`modules/notification/infrastructure/persistence/model.go` (new).
**Do:** Migration exactly per blueprint (both tables, partial claim index, recipient
index, partial unique idempotency index, status CHECK). Bun models matching
column-for-column.
**Accept:** `make migrate-up` then full `migrate-down` then `up` clean (CI
migrations job pattern); `tools/schemacheck` passes; no other package imports the
models yet.

### Task D2 — Domain layer
**Depends on:** nothing (pure Go; parallel-safe with D1).
**Files:** `modules/notification/domain/{notification,preference,errors,repository}.go`
+ tests (new).
**Do:** Entities, `Channel`/`Category`/`Status` value types, transition method
returning `ErrInvalidTransition` for anything outside
pending→sending→{sent,failed}, failed→{pending,dead}, and the direct sending→dead
path for non-retryable errors; `Backoff(attempt int, base, cap time.Duration)
time.Duration` as a pure function (deterministic given an injected rand source);
preference default = enabled when no record; hard rule: `security`+`email` cannot be
disabled (return a domain error). Repository interfaces incl.
`ClaimBatch(ctx, n int, now time.Time) ([]*Notification, error)`.
**Accept:** ≥90% coverage; zero imports beyond stdlib + `internal/platform`;
transition table test covers every (from,to) pair, allowed and forbidden.

### Task D3 — Application layer
**Depends on:** D2.
**Files:** `modules/notification/application/{service,dispatch,ports,dto}.go` + tests
(new).
**Do:** Ports per blueprint §5 (`ChannelSender`, `TemplateRenderer`, `Clock`).
`Service`: `Enqueue` (preference filter; idempotency-key duplicate → success no-op),
`ListForUser`, `MarkRead`, `MarkAllRead`, `GetPreferences`, `UpdatePreferences`.
`DispatchBatch`: claim → render → send → settle; unknown template key or missing
sender for channel ⇒ straight to dead with descriptive LastError; retryable failure
⇒ failed + `NextAttemptAt = clock.Now() + Backoff(...)`; attempts exhausted ⇒ dead.
All tests against hand-written fakes — no DB, no network.
**Accept:** ≥80% coverage; tests for: happy path per channel, retry scheduling
values (fixed clock), dead-letter on exhaustion, dead-letter on non-retryable,
idempotent enqueue, preference-filtered enqueue (incl. the protected
security+email combo being unfilterable).

### Task D4 — Persistence
**Depends on:** D1, D2.
**Files:** `modules/notification/infrastructure/persistence/{notification_repo,preference_repo}.go`
+ tests (new).
**Do:** Implement repositories over bun. `ClaimBatch` = single statement:
`UPDATE ... SET status='sending' WHERE id IN (SELECT id ... WHERE status='pending'
AND next_attempt_at <= now ORDER BY next_attempt_at LIMIT n FOR UPDATE SKIP LOCKED)
RETURNING *`. Tests via `testsupport.ScratchPostgres`.
**Accept:** the mandatory concurrency test — two goroutines calling `ClaimBatch`
simultaneously over a seeded set; assert zero overlap in claimed IDs (runs under
`-race`; will only execute where Postgres is available — CI is the arbiter, per
guardrail 4). Also: idempotency-key conflict handled as no-op at the SQL level.

### Task D5 — Log sender + templates
**Depends on:** D3.
**Files:** `modules/notification/infrastructure/sender/log.go`,
`modules/notification/infrastructure/template/renderer.go`,
`modules/notification/infrastructure/template/templates/*.tmpl` + tests (new).
**Do:** Renderer over `embed.FS`, resolving **`<key>.<channel>.<part>.tmpl`** naming
(document the convention in a package comment); missing template ⇒ typed error. Log
sender: writes rendered message summary to zerolog at info, always succeeds — the
dev/test channel implementation.

> **Corrected 2026-08-30, delivered as #80.** This line said `<key>.<channel>.tmpl`,
> which stopped being executable when E14 gave `RenderedMessage` an `HTMLBody`: a family
> then has three parts (`subject`, `text`, `html`) and one name can carry one. Blueprint
> §3 held the complementary half — `*.subject/*.text/*.html`, with no channel — so
> neither document's convention worked alone. The shipped name is the union, parsed
> right-to-left because the key is the segment allowed to contain dots.
>
> **"the dispatcher maps to dead" describes a mapping that already exists and does not
> depend on the type.** `application/dispatch.go`'s `attemptDelivery` wraps *every*
> renderer error in `domain.ErrNonRetryable` by construction. The sentinels are still
> worth having — they separate a deployment bug from a producer bug — and #80 wraps
> `ErrNonRetryable` into them so the classification travels with the failure instead of
> depending on the one caller that currently applies it.
**Accept:** renderer tests for found/missing/malformed-payload; ship at least the
`security.password_changed` template family as the working example.

### Task D6 — Dispatcher + module assembly + wiring
**Depends on:** D3, D4, D5. **Merge gate for “module exists”.**
**Files:** `modules/notification/infrastructure/dispatcher/dispatcher.go`,
`modules/notification/{module,config}.go`, `internal/config/*` (add Notification
section), `config/default.yaml`, `.env.example`, `cmd/api/container.go` + tests.
**Do:** Config per blueprint §8, `Validate()` fails closed (email enabled ⇒
host/from/password required; backoff_base ≤ backoff_cap; positive batch/workers).
Dispatcher: ticker at `poll_interval`, `workers` concurrent `DispatchBatch` calls,
plus the stuck-`sending` sweep — **already built, do not write a new one:** call
`domain.NotificationRepository.ClaimStalled(ctx, n, stalledBefore, now)` (landed in #63),
then run each returned row through `RecoverStalled` and `Save` it, per row. `ClaimStalled`
takes ownership under `FOR UPDATE SKIP LOCKED` and **deliberately does not change
status** — a set-based recovery `UPDATE` would be a second, untested copy of the state
machine (E9c) — and re-stamps `claimed_at`, so a sweeper that dies defers the row by
another window rather than leaving it instantly eligible again; context-aware `Stop()` that finishes in-flight sends.
**`New(deps module.Deps, cfg Config) (*module.Module, error)`** — two values, matching
`modules/identity/module.go:88` and `modules/user/module.go:75`; `enabled: false` ⇒
routeless module, no dispatcher, `Close` still safe to call.

> **Corrected 2026-08-30. This line called a three-value shape "the settled constructor
> decision", and no such constructor exists anywhere in the tree.** Four documents
> described this and only one matched the code:
>
> | Source | Shape |
> |---|---|
> | `modules/identity/module.go:88`, `modules/user/module.go:75` | **`(*module.Module, error)`** |
> | this line, before the correction | `(*module.Module, Service, error)` |
> | blueprint §3 | `(*module.Module, *application.Service, error)` |
> | `.claude/agents/docs-scribe.md:33` | `(*module.Module, RootInterface, error)` |
>
> `RootInterface` appears **nowhere in the Go tree** — one hit, in an agent definition.
> That is **E25**, still open, and it is the human's to close. Two agents refused to build
> on it independently: C1 would not document it, and D5 flagged it unprompted.
>
> **Why two values, beyond "the code says so".** `module.Module` already carries
> `Close func() error`, which is where a dispatcher's shutdown belongs — the need that
> made a third value look necessary. And nothing consumes a returned `Service` today;
> `internal/authn`'s package doc states the rule this repo runs on: *"What is absent is
> absent on purpose, and stays absent until a second consumer asks for it."*
>
> **The blueprint's need is real but is D9's, not D6's** — see the note under §10 there.
> When identity needs to enqueue, R2 makes that an interface declared by the **consumer**
> and satisfied at the composition root, which may not change this signature at all.
> Adding a return value later is one line at one call site. Wire into `BuildApp`;
register `Close` per existing lifecycle pattern.
**Accept:** boot with module enabled+disabled both clean; dispatcher shutdown test
(start, enqueue, stop, assert no goroutine leak via the repo's pattern or
`goleak` if already a dependency — do not add new deps otherwise); config validation
table test.

### Task D7 — Transport
**Depends on:** D6 and Phase B.
**Files:** `modules/notification/transport/{handler,routes,dto}.go` + tests,
`cmd/api/routes_test.go` (edit `TestRouteTable`).
**Do:** Routes per blueprint §7; `Config.Auth authn.Middleware` required (fail
closed); recipient scoping via `authn.MustFromContext(...).Subject` — a user can
list/read only their own rows (cross-user id ⇒ 404, not 403). Preference PUT
rejecting protected combos ⇒ problem+json 422. Cursor pagination consistent with any
existing repo pattern — read `modules/user/transport` first and match its style.
Tests use `testsupport.FakeAuth`.
**Accept:** `TestRouteTable` updated and green; 401-without-token covered by
mounting real middleware in one test; cross-user 404 test present.

### Task D8 — SMTP sender
**Depends on:** D6.
**Files:** `modules/notification/infrastructure/sender/email_smtp.go` + test (new).
**Do:** Prefer stdlib `net/smtp` unless the repo already carries a mail dependency —
check `go.mod`; do not add a new dependency without noting it in the PR description.
Password via `secret.String.Reveal()` at dial time only. Timeouts hard-set from
config. Distinguish retryable (connection, 4xx SMTP) from non-retryable (auth
failure, bad recipient) via the error contract D3 established.
**Accept:** unit tests against a local test SMTP listener (in-process); error
classification table test.

### Task D9 — Identity `SecurityNotifier` + adapter
**Depends on:** D6; identity work per its own roadmap.
**Files:** `modules/identity/application/notifier.go` (interface + `NoopNotifier`),
call sites in identity use cases, `cmd/api/notifier_adapter.go` (new),
`cmd/api/container.go`.
**Do:** Interface owned by identity (`PasswordChanged`, plus whatever events
identity's current use cases can emit today — do not invent events with no
producer). Adapter in `cmd/api` maps each to `EnqueueRequest` with idempotency key
`<event>:<userID>:<eventID>:<channel>`. **The channel segment is required**, not
optional tidiness: `idx_notifications_idem` is one partial-unique index over the whole
outbox and is not channel-scoped, so a key shared across channels makes the second
channel's `Enqueue` collide with the first channel's row and disappear through
`ON CONFLICT DO NOTHING` — a silently undelivered notification, not a deduplicated
retry. This task originally specified the three-segment form; D9 (#95) caught it. Wire real adapter when notification enabled,
`NoopNotifier` otherwise.
**Accept:** identity unit tests assert notifier invocation via a fake; one
composition-level test drives an identity use case and asserts a pending row lands
(DB-backed, CI-arbitrated); `make arch` proves no new module-to-module import.

### Task D10 — Metrics, audit, swagger, coverage
**Depends on:** D6–D9.
**Files:** dispatcher (metrics hooks), audit emission for dead security-category
rows, swagger annotations on transport handlers, `tools/coverage/policy.json`.
**Do:** Counters `notification_{sent,failed,dead}_total{channel}` + dispatch latency
histogram via `internal/metrics` existing patterns; audit event on
security-category dead-letter; swagger annotations matching existing handler style
(`make swagger-docs` must succeed); coverage entries for notification's domain (90%)
and application (80%) with reasons.
**Accept:** `make swagger-docs`, `make coverage-check`, full CI green.

---

## Execution protocol per task

1. Branch `feat/<task-id>-<slug>` from the phase's base.
2. Read guardrails (§0) + the task + referenced blueprint/analysis sections + every
   listed file before editing.
3. Implement; run the task's accept commands + guardrail 8's standard set.
4. PR description: task ID, deviations from this plan (if any, with why), pre-existing
   issues noticed but not fixed.
5. Any stop condition hit → report, don't improvise.

## Dependency graph (merge order)

```
A1 → A2 → A3 → A4   (one PR, commit-ordered)
A4 → B1 → B2 ─┐
A4 → B3 ──────┼→ B4   (one PR)
B4 → C1
D1 ─┐
D2 → D3 ─┤
D1+D2 → D4 ─┼→ D6 → D7 (needs B), D8 → D9 → D10
D3 → D5 ─┘
```
D1 and D2 can start in parallel with Phase B; D6 onward should wait for Phase A so
notification is born on the contract (D7 hard-requires Phase B's FakeAuth).
