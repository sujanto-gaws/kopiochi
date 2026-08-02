# Dependency Rules

**Status:** Proposed — see [ADR-005](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
**Date:** 2026-08-02

Rules are worthless without enforcement. This document states the rules **and**
the check that fails the build when they are broken.

---

## Problem

### Inverted dependency: `extensions/` → `internal/`

Eleven files under `extensions/identity/` import `internal/utils`:

```
extensions/identity/application/admin.go
extensions/identity/application/auth.go
extensions/identity/application/helpers.go
extensions/identity/domain/repository.go          ← domain layer!
extensions/identity/domain/service.go             ← domain layer!
extensions/identity/infrastructure/http/helpers.go
extensions/identity/infrastructure/http/user_handler.go
extensions/identity/infrastructure/persistence/helpers.go
extensions/identity/infrastructure/persistence/mfa_repository.go
extensions/identity/infrastructure/persistence/user_repository.go
extensions/identity/infrastructure/persistence/user_token_repository.go
```

Two consequences:

1. An `extensions/` tree that imports the host's private helpers can never be
   extracted into its own repository or module — the `internal/` visibility rule
   guarantees it.
2. `domain/repository.go` and `domain/service.go` — the layer that is supposed to
   be pure — depend on a utility package containing HTTP helpers, pagination, and
   hashing.

### No mechanical enforcement anywhere

Nothing in the repository checks layering. There is no linter config, no
`depguard`, no architecture test. The three-layout drift documented in
[module layout](module-layout.md) happened precisely because nothing objected.

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
| `transport` | `application`, chi, stdlib | `infrastructure`, `domain` models directly |

### R2 — Module isolation

- `modules/a` must not import `modules/b/domain` or `modules/b/infrastructure`.
- Cross-module needs are expressed as an interface **declared by the consumer**
  and satisfied at the composition root.

### R3 — Platform direction

- `modules/**` may import `internal/platform` (the shared kernel).
- `internal/platform` must not import `modules/**`.
- `internal/httpx`, `internal/db`, `internal/config` must not import `modules/**`.
- Only `cmd/**` imports both.

### R4 — No `internal/utils`

`utils` is not a boundary; it is a bucket. It currently mixes HTTP response
helpers, pagination, string manipulation, hashing, and ID generation — which is
how `domain` ended up importing HTTP helpers.

Split it:

| Current | Destination | Rationale |
|---|---|---|
| `utils/http.go` | `internal/httpx` | HTTP concern — `domain` must never reach it |
| `utils/pagination.go` | `internal/platform/paging` | Genuinely shared value type |
| `utils/hasher.go` | `internal/platform/crypto` | Shared primitive (see caveat below) |
| `utils/generator.go` | `internal/platform/id` | ID generation |
| `utils/string_utils.go` | inline at call sites, or `internal/platform/strings` | Mostly one-caller helpers |

**Caveat on `utils/hasher.go`:** `StringHashHex` is plain SHA-256 and is used to
hash refresh tokens (`identity/application/auth.go:67`,
`identity/application/helpers.go:14`) and a user-agent string
(`identity/infrastructure/http/helpers.go:75`). Unsalted SHA-256 is acceptable
for a high-entropy random refresh token, and is *not* acceptable for anything
low-entropy. Document that constraint at the function, and keep it well away from
password handling — passwords correctly use bcrypt in
`identity/infrastructure/hasher/bcrypt.go`. See
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

      no-utils:
        files:
          - "$all"
        deny:
          - pkg: "github.com/sujanto-gaws/kopiochi/internal/utils"
            desc: "internal/utils is removed; use internal/platform/* or internal/httpx"
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

1. Create `internal/platform/{paging,crypto,id}` and `internal/httpx`; move the
   contents of `internal/utils` into them.
2. Re-point the 11 identity imports; `domain/repository.go` and
   `domain/service.go` should end up importing only `platform/paging`.
3. Delete `internal/utils`.
4. Add the `depguard` rules and the module-isolation test.
5. Run CI — it must pass before the layout migration in
   [module layout](module-layout.md) proceeds, so the new structure is protected
   from day one.

Order matters: adding enforcement **before** the bulk move means the move cannot
reintroduce the inversion.

---

## Related documents

- [ADR-005: Module Boundaries and Dependency Direction](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [Module layout](module-layout.md)
- [Testing strategy](../06-quality/testing-strategy.md)
