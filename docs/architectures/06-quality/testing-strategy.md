# Testing Strategy

**Status:** Partially implemented
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phase 1

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

**Now:** seven test files exist (`d92480c`, `720e580`, plus the Phase 0 config
tests in `b74b358`):

```
cmd/api/container_test.go            internal/config/config_test.go
cmd/api/routes_test.go               internal/db/schema_test.go
cmd/api/login_e2e_test.go            internal/httpx/health_test.go
internal/plugins/middleware/ratelimit_test.go
```

That is the priority-1 set below plus health coverage and a login E2E flow. It is
a floor, not a suite — everything from "Unit tests" onward in this document is
still unwritten, and **there is still no CI pipeline of any kind**, so `make ci`
is run by hand or not at all.

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
| 3 | `internal/httpx/ratelimit_test.go` | `internal/plugins/middleware/ratelimit_test.go` | **Skipped** — see below |
| 4 | `internal/config/config_test.go` | same | Passing |
| 5 | `internal/db/schema_test.go` | same | Passing |

Test 3 is `t.Skip`ped with an explicit reason: the rate limiter still holds its
mutex across `next.ServeHTTP`, so the test still fails by design. Run it with
`RUN_KNOWN_FAILING=1` to observe the failure. It goes green with Phase 2.1 — the
skip is the marker, not a pass.

Test 4 as sketched calls `cfg.DB.Password.Reveal()`; `secret.String` does not
exist yet (Phase 2.9), so the shipped test asserts a plain `string`.

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

| Area | What to assert |
|---|---|
| `domain` | Invariants, state transitions, validation. No DB, no HTTP — if a mock is needed, the boundary is wrong. |
| Token service | Algorithm pinning rejects HS256 when RS256 expected; expired/wrong-`iss`/wrong-`aud`/wrong-class tokens rejected. |
| Config | Precedence, validation errors, unknown-key rejection. |
| CORS | Disallowed origin gets no header **and** no 403; `Vary: Origin` always present. |
| Rate limiter | Refill maths with an injected clock; eviction; `max_keys` cap. |
| Recovery | Panic → `problem+json` 500, no partial body. |

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

### Integration tests

Use `testcontainers-go` for a real Postgres — not sqlmock. The queries under test
are bun-generated; mocking the driver tests the mock.

```go
func TestUserRepository_ByEmail(t *testing.T) {
    db := testdb.New(t)          // container + migrations + truncate between tests
    repo := persistence.NewUserRepository(db)

    require.NoError(t, repo.Create(ctx, &domain.AppUser{Email: "a@example.com"}))

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

## Sequencing

1. ✅ `.gitattributes` + normalise line endings — `b294de2`; makes `gofmt` meaningful.
2. ✅ The five priority-1 tests — `d92480c`; they document the critical bugs as failing tests.
3. ⏳ CI pipeline with build, vet, lint, test, `-race` — **not started**; no CI config exists in the repository.
4. ⏳ `internal/testsupport` + integration tests as modules are migrated — partially anticipated by `internal/testutil/postgres.go`.
5. ⏳ Coverage floors and the ratchet.
6. ⏳ E2E flows once the identity module is wired and serving — one exists (`cmd/api/login_e2e_test.go`, `720e580`); the refresh and protected-call flows do not.

Writing the priority-1 tests *before* the fixes gave a red-green signal for the
whole remediation effort, and step 3 is what turns that signal into a gate. Until
it lands, every check in this document is opt-in.

---

## Related documents

- [Repository hygiene](repository-hygiene.md)
- [Observability](observability.md)
- [Dependency rules](../01-modularity/dependency-rules.md)
- [Remediation plan](../07-roadmap/remediation-plan.md)
