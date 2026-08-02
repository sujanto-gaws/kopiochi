# Testing Strategy

**Status:** Partially implemented
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 2

> **Read [Race detection is outstanding](#race-detection-is-outstanding) before
> relying on any concurrency claim in these documents.** `go test -race` cannot
> run in this development environment, and a real data race shipped and was
> caught by inspection rather than by tooling.

---

## Problem (partially resolved): the test suite was empty

At the time of the review:

```
$ find . -name "*_test.go" | wc -l
0
```

Zero test files in an ~8,000 LOC codebase. Meanwhile the Makefile advertises:

```make
test:          $(GO) test -v ./...
test-coverage: $(GO) test -coverprofile=coverage.out ./...
ci: check test
```

All three **passed**, because `go test ./...` over a tree with no tests succeeds.
`make ci` was a green light that verified nothing.

**Now:** thirteen test files exist — seven from Phases 0–1 (`b74b358`,
`d92480c`, `720e580`) and six added by Phase 2:

```
cmd/api/container_test.go                     internal/config/config_test.go
cmd/api/routes_test.go                        internal/db/schema_test.go
cmd/api/login_e2e_test.go                     internal/httpx/health_test.go
internal/plugins/middleware/ratelimit_test.go

# added in Phase 2
internal/httpx/security_headers_test.go       (2.8, 0968aae)
internal/middleware/clientip_test.go          (2.2, 333968c)
internal/platform/secret/secret_test.go       (2.9, acc057d)
internal/plugins/middleware/cors_test.go      (2.3, 87381d2)
internal/plugins/middleware/ratelimit_tokenbucket_test.go  (2.1, dcc6e5d)
modules/identity/infrastructure/token/jwt_test.go          (2.4/2.5, e0da81e, 946c1c8)
```

That is the priority-1 set, plus health coverage, a login E2E flow, and unit
coverage for every piece of Phase 2 — tokens, CORS, client-IP resolution,
security headers, the rate limiter, and secret redaction. Several rows of the
"Unit tests" table below are now genuinely populated rather than aspirational.

It is still a floor, not a suite. The whole "Integration tests", "Architecture
tests", and most of "E2E" remain unwritten; **there is still no CI pipeline of
any kind**, so `make ci` is run by hand or not at all; and `-race` — the one
flag this document calls non-optional — does not run here at all.

Every defect catalogued in these documents — the mis-wired composition root, the
shadowed `/api/v1` router, the mutex held across `ServeHTTP`, the ignored
`APP_DB_PASSWORD`, the schema that matched no model — would have been caught by a
single test each.

### The formatting signal (fixed)

`gofmt -l .` used to list **every** `.go` file. Not because of formatting, but
because every file used CRLF line endings and there was no `.gitattributes`.
`gofmt` normalises to LF, so it reported all files as unformatted. Consequences
at the time:

- `make fmt` rewrote the entire repository — an unreviewable diff.
- `make check` (`fmt vet tidy`) always reported the tree dirty.
- The one automated quality signal that existed was pure noise.

*Fixed in `b294de2` (`.gitattributes` = `* text=auto eol=lf`, plus
`git add --renormalize`) and `3dbd1b4` (`gofmt -s` across the tree).
`gofmt -l .` now returns nothing, so formatting is enforceable — see
[repository hygiene](repository-hygiene.md).*

### `go vet` findings (fixed)

```
examples/extension-demo/main.go:131: fmt.Println arg list ends with redundant newline
examples/extension-demo/main.go:183: fmt.Println arg list ends with redundant newline
```

Two warnings, both in code slated for deletion. *Fixed in `6459348`;
`go vet ./...` is now clean.* They remain worth recording as evidence that vet
output was gating nothing — and, with no CI, still is not.

---

## Target: a pyramid, with the highest-value tests first

```
        ╱╲          E2E — a handful: login → refresh → protected call
       ╱  ╲
      ╱────╲        Integration — repositories against real Postgres, handlers via httptest
     ╱      ╲
    ╱────────╲      Unit — domain rules, token logic, config, middleware
```

### Priority 1 — tests that would have caught the critical defects — written

All five shipped in `d92480c`. Each failed against the code as it stood when this
document was written. Where they landed differs from the sketch below:

| # | Sketched location | Actual location | State |
|---|---|---|---|
| 1 | `cmd/api/container_test.go` | same | Passing |
| 2 | `internal/httpx/routes_test.go` | `cmd/api/routes_test.go` — needs `BuildApp`, which is in `package main` | Passing |
| 3 | `internal/httpx/ratelimit_test.go` | `internal/plugins/middleware/ratelimit_test.go` | **Passing** since Phase 2.1 |
| 4 | `internal/config/config_test.go` | same | Passing |
| 5 | `internal/db/schema_test.go` | same | Passing |

Test 3 was `t.Skip`ped with an explicit reason: the rate limiter held its mutex
across `next.ServeHTTP`, so the test failed by design and had to be run with
`RUN_KNOWN_FAILING=1` to observe the failure. *`dcc6e5d` (Phase 2.1) made it
green. The skip and the environment-variable escape hatch are gone —
`TestRateLimitAllowsConcurrentRequests` now runs unconditionally as part of the
default suite. That was the point of writing it first: the red-green transition
is the evidence that 2.1 fixed the thing it claimed to fix.*

Test 4 as sketched calls `cfg.DB.Password.Reveal()`; `secret.String` did not
exist yet, so the shipped test asserted a plain `string`. *`acc057d` added
`secret.String` and the assertions are now `.Reveal()`, as sketched.*

**All five of the priority-1 tests now pass.** The red-green signal the whole
remediation effort was hung on has been collected.

```go
// 1. Empty container — cmd/api/container_test.go
func TestBuildApp_RegistersModules(t *testing.T) {
    app, err := BuildApp(testConfig(t), testDB(t), zerolog.Nop())
    require.NoError(t, err)
    require.NotEmpty(t, app.Modules)
}

// 2. Route table / missing prefix — internal/httpx/routes_test.go
func TestRouteTable(t *testing.T) {
    routes := walk(t, buildTestRouter(t))
    require.Contains(t, routes, "POST /api/v1/auth/login")
    require.Contains(t, routes, "GET /healthz")
}

// 3. Rate limiter serialisation — internal/httpx/ratelimit_test.go
func TestRateLimitAllowsConcurrentRequests(t *testing.T) {
    release := make(chan struct{})
    entered := make(chan struct{}, 2)
    h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        entered <- struct{}{}
        <-release
    }))
    go h.ServeHTTP(httptest.NewRecorder(), req(t))
    go h.ServeHTTP(httptest.NewRecorder(), req(t))

    // Both handlers must be inside simultaneously. Current code still
    // serialises here — this test is skipped until Phase 2.1.
    for i := 0; i < 2; i++ {
        select {
        case <-entered:
        case <-time.After(2 * time.Second):
            t.Fatal("handler blocked: rate limiter holds its lock across ServeHTTP")
        }
    }
    close(release)
}

// 4. Env override — internal/config/config_test.go
func TestEnvOverridesFile(t *testing.T) {
    t.Setenv("APP_DB_PASSWORD", "from-env")
    cfg, err := Load("testdata/full.yaml")
    require.NoError(t, err)
    require.Equal(t, "from-env", cfg.DB.Password.Reveal())
}

// 5. Schema drift — internal/db/schema_test.go
func TestModelsMatchMigratedSchema(t *testing.T) {
    // Apply migrations to a scratch database, then compare each bun model's
    // expected columns against information_schema.
}
```

### Unit tests

| Area | What to assert | State after Phase 2 |
|---|---|---|
| `domain` | Invariants, state transitions, validation. No DB, no HTTP — if a mock is needed, the boundary is wrong. | ⏳ none written |
| Token service | Algorithm pinning rejects HS256 when RS256 expected; expired/wrong-`iss`/wrong-`aud`/wrong-class tokens rejected. | ✅ 7 tests, `modules/identity/infrastructure/token/jwt_test.go` |
| Config | Precedence, validation errors, unknown-key rejection. | ◐ precedence and validation covered (17 tests); unknown-key rejection not written because strict decoding did not ship |
| CORS | Disallowed origin gets no header **and** no 403; `Vary: Origin` always present. | ✅ 7 tests, `internal/plugins/middleware/cors_test.go` |
| Rate limiter | Refill maths with an injected clock; eviction; `max_keys` cap. | ✅ 6 tests across the two `ratelimit*_test.go` files, clock injected via `setClock` — no sleeps |
| Recovery | Panic → `problem+json` 500, no partial body. | ⏳ `httpx.Recovery` does not exist |
| Client IP | Forwarded headers honoured only from trusted proxies; empty list trusts nothing. | ✅ 6 tests, `internal/middleware/clientip_test.go` *(added to this table in Phase 2)* |
| Security headers | Present on every response including 404s; HSTS gated; CSP relaxed only for `/swagger/*`. | ✅ 4 tests, `internal/httpx/security_headers_test.go` *(added in Phase 2)* |
| Secret redaction | `%v`/`%s`/`%#v`/JSON all yield `[REDACTED]`, including inside a struct. | ✅ 4 tests, `internal/platform/secret/secret_test.go` *(added in Phase 2)* |

Token tests deserve emphasis — they are cheap, need no infrastructure, and cover
the highest-severity findings in
[token architecture](../04-security/token-architecture.md):

```go
func TestValidate_RejectsWrongAlgorithm(t *testing.T)
func TestValidate_RejectsWrongIssuer(t *testing.T)
func TestValidate_RejectsWrongAudience(t *testing.T)
func TestValidate_RejectsMFATokenAsAccessToken(t *testing.T)   // the escalation path
func TestValidate_RejectsExpired(t *testing.T)
```

*All five shipped in `e0da81e` + `946c1c8`, under exactly these names, plus
`TestValidate_RejectsNoneAlgorithm` and `TestValidate_AcceptsGenuineAccessToken`.
They were as cheap as predicted — a generated keypair and no infrastructure.*

### Integration tests

Use `testcontainers-go` for a real Postgres — not sqlmock. The queries under test
are bun-generated; mocking the driver tests the mock.

```go
func TestUserRepository_ByEmail(t *testing.T) {
    db := testdb.New(t)          // container + migrations + truncate between tests
    repo := persistence.NewUserRepository(db)

    require.NoError(t, repo.Save(ctx, &domain.User{Email: "a@example.com"}))

    got, err := repo.ByEmail(ctx, "A@EXAMPLE.COM")   // case-insensitivity
    require.NoError(t, err)
    require.Equal(t, "a@example.com", got.Email)
}
```

Handler tests use `httptest` against the real router so middleware is exercised:

```go
func TestLogin_InvalidPassword_Returns401(t *testing.T)
func TestProtectedRoute_WithoutToken_Returns401(t *testing.T)   // guards fail-open
func TestProtectedRoute_WithMFAToken_Returns401(t *testing.T)   // guards escalation
```

### Architecture tests

The module-isolation test from
[dependency rules](../01-modularity/dependency-rules.md) runs as an ordinary Go
test, so layering violations fail CI like any other bug.

### E2E

A small number of full-stack flows against a running binary:

1. Register → login → access protected route → refresh → logout.
2. Login with MFA enabled → MFA token → verify → access token.
3. Refresh-token reuse → family revoked.

---

## Coverage policy

- **No blanket percentage target.** A global number encourages tests on trivial
  getters.
- **Per-package floors** where they matter:

| Package | Floor |
|---|---|
| `modules/*/domain` | 90% |
| `modules/*/application` | 80% |
| `internal/httpx` (middleware) | 85% |
| `internal/config` | 85% |
| `modules/*/infrastructure` | 60% (integration-covered) |
| `cmd/` | none — covered by E2E |

- **Ratchet, never regress:** CI compares coverage against the base branch and
  fails on a decrease. This avoids demanding a big-bang test-writing effort while
  guaranteeing new code is covered.

---

## Test infrastructure

```
internal/testsupport/
├── db.go        testcontainers Postgres + migrations + per-test truncation
├── config.go    valid config fixtures
├── auth.go      token minting for authenticated requests
└── http.go      request/response helpers
```

Rules: tests are hermetic (no shared external state), parallel by default
(`t.Parallel()`), deterministic (inject the clock — never `time.Sleep` to wait for
a window), and use table-driven cases for permutations.

---

## CI pipeline

```yaml
jobs:
  quality:
    steps:
      - go build ./...
      - go vet ./...
      - golangci-lint run ./...        # includes the depguard layering rules
      - gofmt -l . | tee /dev/stderr | (! read)   # meaningful once .gitattributes lands
      - go test -race -coverprofile=coverage.out ./...
      - go run ./tools/coverage -check-floors
      - govulncheck ./...
      - gitleaks detect --no-git
  migrations:
    services: [postgres]
    steps:
      - make migrate-up
      - make migrate-down
      - make migrate-up
      - go test ./internal/db/... -run TestModelsMatchMigratedSchema
```

`-race` is not optional: the rate-limiter defect and the registry re-init leak are
both concurrency issues.

---

## Race detection is outstanding

**`go test -race` cannot run in this development environment.** Every package
fails immediately:

```
$ go test -race ./internal/platform/secret/
exit status 0xc0000139
FAIL	github.com/sujanto-gaws/kopiochi/internal/platform/secret	0.668s
```

`0xc0000139` is `STATUS_ENTRYPOINT_NOT_FOUND` — the race runtime is linking
against a C toolchain that does not export the symbols it needs. The toolchain
here is **mingw-w64 GCC 8.1.0, built in 2018**, against Go 1.25; Go's Windows
race runtime requires a substantially newer GCC. This is an environment
limitation, not a code defect, and it is not fixable from inside the repository.

**Race coverage is therefore outstanding.** No concurrency claim anywhere in
these documents has been verified by the race detector. That gap is not
theoretical:

> Phase 2.1 (`dcc6e5d`) rewrote the rate limiter specifically for concurrency
> correctness — and shipped with a data race of its own. `p.now`,
> `p.initialized`, and `p.burst` were read without the mutex that guards their
> writes, so a request served concurrently with `Close` raced on `initialized`
> and `burst`. It was **found by inspection and fixed in `d130519`**
> (`snapshot()` reads `initialized` and `burst` together under the lock;
> `allow()` takes the clock inside its own critical section; `evictExpired`
> already held it).
>
> A race introduced by the commit whose entire purpose was concurrency
> correctness, in the highest-traffic middleware in the process, caught only
> because someone re-read the diff. That is precisely the class of bug `-race`
> exists to catch, and precisely why "-race is not optional" above is not
> rhetoric.

**Phase 4.4's CI closes this.** The pipeline runs on Linux, where the race
detector needs no external toolchain, so `go test -race ./...` there covers
every package regardless of what any individual developer's machine can do.
Until that lands, treat concurrency correctness in this codebase as
review-enforced, and re-read concurrent code with that in mind.

Local workarounds, in descending order of practicality, none of them adopted:
run the suite under WSL or a Linux container; install a modern mingw-w64
(GCC 13+) and re-test; or wait for CI. The first is the cheapest, and is worth
doing ad hoc before any further concurrency change.

---

## Sequencing

1. ✅ `.gitattributes` + normalise line endings — `b294de2`; makes `gofmt` meaningful.
2. ✅ The five priority-1 tests — `d92480c`; they documented the critical bugs as
   failing tests, and **all five now pass** after Phase 2.
2b. ✅ Unit coverage for every Phase 2 change — six new test files, `dcc6e5d`
   through `acc057d`.
3. ⏳ CI pipeline with build, vet, lint, test, `-race` — **not started**; no CI
   config exists in the repository. This is now the single highest-value item
   left in this document: it is the only route to `-race` coverage (see
   [above](#race-detection-is-outstanding)), and it is what turns every other
   check here from opt-in into a gate.
4. ⏳ `internal/testsupport` + integration tests as modules are migrated — partially anticipated by `internal/testutil/postgres.go`.
5. ⏳ Coverage floors and the ratchet.
6. ⏳ E2E flows once the identity module is wired and serving — one exists (`cmd/api/login_e2e_test.go`, `720e580`); the refresh and protected-call flows do not.

Writing the priority-1 tests *before* the fixes gave a red-green signal for the
whole remediation effort, and it worked — Phase 2 turned the last of them green.
Step 3 is what turns that signal into a gate. Until it lands, every check in this
document is opt-in, and `-race` is not available at all.

---

## Related documents

- [Repository hygiene](repository-hygiene.md)
- [Observability](observability.md)
- [Dependency rules](../01-modularity/dependency-rules.md)
- [Remediation plan](../07-roadmap/remediation-plan.md)
