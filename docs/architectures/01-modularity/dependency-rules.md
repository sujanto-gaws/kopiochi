# Dependency Rules

**Status:** Implemented — Phase 3.2 (`1b46d87`)
**Date:** 2026-08-02
**Last verified:** 2026-08-03, after Phase 3.2 — see
[ADR-005](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)

Both enforcement mechanisms below now exist and run in CI:

- **`.golangci.yml`** carries the `depguard` rules for R1 (domain and
  application purity) and R3 (`internal/**` must not import `modules/**`).
- **`tools/archtest`** carries five tests over the real import graph: R2, R3,
  R1 for both layers, and R4.
- **`.github/workflows/ci.yml`** — the repository's first CI — runs both.

They overlap on purpose. depguard matches on file globs, so it only polices
boundaries known when the config was written; archtest walks the graph and
therefore covers modules that do not exist yet. Each caught something the
other missed during Phase 3: depguard found `internal/db/schema_test.go`
importing two modules' persistence models (a real R3 violation, since moved to
`tools/schemacheck/`), and archtest expresses the generic "module A must not
import module B" that no glob can.

Two properties were verified rather than assumed. The tests **do** fail on
injected violations — a cross-module import plus a `domain -> bun` import
produce three failures, and reverting restores green. And **`go test` caching
silently masks violations**: the cache keys on `tools/archtest`'s own files
while the tests read the whole repository, so a violation introduced anywhere
else returns `ok (cached)`. Measured on one tree: `ok (cached)` without
`-count=1`, three failures with it. CI, `make arch` and the package doc all
pass `-count=1`.

Rules are worthless without enforcement. This document states the rules **and**
the check that fails the build when they are broken.

---

## Problem

> **Withdrawn.** An earlier revision of this document opened with an "inverted
> dependency" finding: eleven named files under `extensions/identity/` importing
> `internal/utils`, including two in the domain layer. Neither `extensions/` nor
> `internal/utils` appears in any commit of this repository
> (`git log --all --diff-filter=A` returns nothing for either), and four of the
> eleven filenames — `mfa_repository.go`, `user_token_repository.go`,
> `role_repository.go`, `refresh_repository.go` — have never existed under any
> path. The finding could not be substantiated and has been withdrawn, along with
> the `internal/utils` split table that followed from it. The rules below are
> unaffected: they were written as the target, not as a description of the
> violation.

### No mechanical enforcement anywhere — ✅ resolved in Phase 3.2

> Nothing in the repository checks layering. There is no `.golangci.yml`, no
> `depguard` configuration, no architecture test, and no CI to run one. Every
> rule below is currently upheld — or not — by review discipline alone, which is
> the one control this document exists to replace.

That was true until `1b46d87`. All four now exist; see the header above. The
paragraph is kept because it states the problem the rest of this document
solves, and because the first violation the new rules found had been sitting in
the tree unnoticed — which is precisely what review discipline alone gets you.

---

## The rules

### R1 — Layer direction inside a module

```
transport ──▶ application ──▶ domain ◀── infrastructure
```

| Package | May import | Must NOT import |
|---|---|---|
| `domain` | stdlib, `internal/platform` | bun, chi, viper, zerolog, any sibling layer |
| `application` | `domain`, stdlib, `internal/platform` | bun, chi, `infrastructure`, `transport` |
| `infrastructure` | `domain`, bun, pgx, external clients | `application`, `transport` |
| `transport` | `application`, chi, stdlib, `internal/authn`, `internal/httpx` | `infrastructure`, `domain` models directly, the rest of `internal/**` |

`transport`'s two shared-kernel imports are the exception that proves the ring,
and they are enforced by the `transport-kernel-access` depguard rule rather than
left to review. `internal/authn` carries the authentication contract — who the
caller is — so that a module's handlers depend on a `Principal` rather than on
whichever context key the identity module happens to use today; before it
existed that coupling was a real dependency edge that neither depguard nor
`tools/archtest` could see, because a context value is not an import.
`internal/httpx` owns the problem+json writer and the canonical 401, so that
every module rejects a request identically instead of inventing its own error
body. Everything else in `internal/**` stays out: a transport package that can
import `internal/db` can query the database from a handler, which is the
layering R1 exists to prevent. The rule exempts `_test.go` files, which
legitimately use `internal/testsupport`.

### R2 — Module isolation

- `modules/a` must not import `modules/b/domain` or `modules/b/infrastructure`.
- Cross-module needs are expressed as an interface **declared by the consumer**
  and satisfied at the composition root.

### R3 — Platform direction

- `modules/**` may import `internal/platform` (the shared kernel).
- `internal/platform` must not import `modules/**`.
- `internal/httpx`, `internal/db`, `internal/config` must not import `modules/**`.
- Only `cmd/**` imports both.

`internal/httpx` already exists (`routes.go`, `health.go`, added in `4fdc609` and
`40887de`) and holds no module imports. `internal/platform` does not exist yet; it
is created when the first genuinely shared value type needs a home.

### R4 — No `utils` package, ever

`utils` is not a boundary; it is a bucket. A package named for its lack of a
concept accumulates HTTP helpers, pagination, string manipulation, hashing, and ID
generation side by side, and then the domain layer imports it for one of them and
inherits all the rest.

This is a forward-looking rule, not a description of an existing package. Shared
code goes into a package named for what it *is*:

| Kind of helper | Destination |
|---|---|
| HTTP request/response helpers | `internal/httpx` |
| Pagination value types | `internal/platform/paging` |
| Hashing and other crypto primitives | `internal/platform/crypto` |
| ID generation | `internal/platform/id` |

`common`, `shared`, `helpers`, `misc`, and `util(s)` are rejected package names.
Note that `modules/identity/domain/refresh_token.go` already keeps its
`HashToken` (plain SHA-256, used only on high-entropy refresh tokens) inside the
domain package that owns the concept, rather than in a shared bucket — that is the
pattern to follow. See
[token architecture](../04-security/token-architecture.md).

---

## Enforcement

### 1. `go-arch-lint` or `depguard` in CI

`depguard` config (`.golangci.yml`) expressing R1 and R3:

```yaml
linters-settings:
  depguard:
    rules:
      domain-purity:
        files:
          - "**/modules/*/domain/**"
        deny:
          - pkg: "github.com/uptrace/bun"
            desc: "domain must not depend on the ORM"
          - pkg: "github.com/go-chi/chi/v5"
            desc: "domain must not depend on the HTTP router"
          - pkg: "github.com/spf13/viper"
            desc: "domain must not depend on configuration loading"
          - pkg: "github.com/rs/zerolog"
            desc: "domain must not depend on the logger"

      application-purity:
        files:
          - "**/modules/*/application/**"
        deny:
          - pkg: "github.com/uptrace/bun"
            desc: "application talks to domain interfaces, not the ORM"
          - pkg: "github.com/go-chi/chi/v5"
            desc: "application must not depend on the HTTP router"

      platform-independence:
        files:
          - "**/internal/platform/**"
          - "**/internal/httpx/**"
          - "**/internal/db/**"
          - "**/internal/config/**"
        deny:
          - pkg: "github.com/sujanto-gaws/kopiochi/modules"
            desc: "platform must not depend on business modules"
```

### 2. Module-isolation test

`depguard`'s glob rules do not express "module A must not import module B"
generically. Cover R2 with a test that walks the import graph:

```go
// tools/archtest/modules_test.go
func TestModulesDoNotImportEachOther(t *testing.T) {
    pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedImports | packages.NeedName},
        "github.com/sujanto-gaws/kopiochi/modules/...")
    if err != nil {
        t.Fatal(err)
    }

    const prefix = "github.com/sujanto-gaws/kopiochi/modules/"
    for _, p := range pkgs {
        owner := moduleOf(p.PkgPath, prefix)   // e.g. "identity"
        for imp := range p.Imports {
            if !strings.HasPrefix(imp, prefix) {
                continue
            }
            if other := moduleOf(imp, prefix); other != owner {
                t.Errorf("%s imports %s: modules must not depend on each other", p.PkgPath, imp)
            }
        }
    }
}
```

This runs as a normal `go test ./tools/archtest/...` and therefore gates CI with
no extra tooling.

### 3. Review checklist

For anything the linters cannot see:

- [ ] No bun model type appears outside `infrastructure/persistence`.
- [ ] No domain entity is JSON-serialised directly in `transport`.
- [ ] New shared code went to a named `internal/platform/*` package, not a `utils` bucket.

---

## Migration path

1. Add the `.golangci.yml` `depguard` rules and the module-isolation test against
   the tree as it stands. `modules/identity/**` is the only module today, so the
   rules should pass immediately — if they do not, that is a real finding.
2. Add a CI job that runs them. Until one exists (Phase 4.4) the rules are
   documentation, not enforcement.
3. Only then perform the remaining move in
   [module layout](module-layout.md) — the profile-user stack out of
   `internal/{domain,application,infrastructure}` into `modules/user/`.

Order matters: adding enforcement **before** the move means the move cannot
introduce an inversion that nothing objects to.

---

## Related documents

- [ADR-005: Module Boundaries and Dependency Direction](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [Module layout](module-layout.md)
- [Testing strategy](../06-quality/testing-strategy.md)
