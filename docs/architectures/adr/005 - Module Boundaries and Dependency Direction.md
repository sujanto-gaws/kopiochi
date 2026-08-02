# ADR-005: Module Boundaries and Dependency Direction

## Status
**Proposed** – *Date: 2026-08-02*

## Context

[ADR-001](001%20-%20Adopt%20Domain%20Driven%20Design.md) commits the project to DDD
with strict layer separation and dependency inversion, and states that code
review must verify the domain layer has no dependency on infrastructure. In
practice, neither the layering nor the location of modules has held.

> **Context revised, 2026-08-02.** As first drafted, this section described three
> coexisting directory "generations" (`internal/modules/aquaculture/`,
> `extensions/identity/`, and flat aquaculture files under `internal/`), an
> inverted dependency in which eleven named files under `extensions/identity/`
> imported `internal/utils`, and `internal/utils` itself as a grab-bag package.
> None of those paths appears in any commit of this repository:
> `git log --all --diff-filter=A` returns nothing for `extensions/`,
> `internal/utils`, `internal/modules/`, or any file matching `*aquaculture*`.
> Those claims could not be substantiated and have been withdrawn. This ADR is
> still `Proposed`, so the Context is revised in place rather than appended to;
> the Decision is unchanged, because it was written as a target rather than as a
> reaction to those specific findings.

**Two directory layouts coexist for business code:**

| Location | State |
|---|---|
| `modules/identity/` | Live, wired through `cmd/api/container.go`, matching the shape this ADR decides on |
| `internal/domain/`, `internal/application/`, `internal/infrastructure/` | Live, layer-first — the profile-user stack that `cmd/api/container.go` wires as the `user` module |

One capability was moved into `modules/` in Phase 1; the other was not. Layer-first
grouping scatters a single capability across four directories, which is the
arrangement this ADR exists to end.

**A dead parallel implementation sits beside them.**
`internal/extension/identity/` is 1,076 LOC across four files, written against the
`internal/extension/` framework, with no transport layer and no importer at all.
It is not an older revision of the live auth stack; it duplicates it.

**Nothing enforces any of this.** There is no import linter, no architecture
test, no `depguard` configuration, and no CI to run one. Whether the domain layer
stays pure is currently a matter of reviewer memory — which is precisely the
control ADR-001 said code review must provide, and which nothing verifies.

## Decision

1. **One location for business code: `modules/<capability>/`** at the repository
   root. `internal/` holds only platform code; `modules/` holds the product.
2. **One layout inside every module:** `domain/`, `application/`,
   `infrastructure/`, `transport/`, `migrations/`, plus `module.go`.
3. **Layer dependency rules** (enforced, not advisory):

   | Package | May import | Must NOT import |
   |---|---|---|
   | `domain` | stdlib, `internal/platform` | bun, chi, viper, zerolog, sibling layers |
   | `application` | `domain`, stdlib, `internal/platform` | bun, chi, `infrastructure`, `transport` |
   | `infrastructure` | `domain`, bun, pgx | `application`, `transport` |
   | `transport` | `application`, chi | `infrastructure`, domain models on the wire |

4. **Modules do not import each other.** Cross-module needs are expressed as an
   interface declared by the consumer and satisfied at the composition root.
5. **Platform never imports modules.** `internal/platform`, `internal/httpx`,
   `internal/db`, and `internal/config` must not reference `modules/**`. Only
   `cmd/**` sees both.
6. **No `utils`-style bucket package is created.** Shared code goes into
   packages named for a concept: `internal/httpx` (HTTP helpers, already exists),
   `internal/platform/paging`, `internal/platform/crypto`, `internal/platform/id`.
   `common`, `shared`, `helpers`, and `util(s)` are rejected package names.
7. **Enforcement is mechanical**: `depguard` rules in `.golangci.yml` for the
   layer and platform rules, and a Go test walking the import graph for the
   module-isolation rule.

## Consequences

### Positive
- **One obvious place** for new code; the "which layout?" question disappears.
- **ADR-001's layering becomes real**, because CI fails when it is violated.
- **Modules become extractable** — no `internal/` imports blocking a future split.
- **The dead parallel identity implementation gets deleted**, removing 1,076 LOC
  and the ambiguity about which auth stack is authoritative.
- **Regressions are caught at PR time**, not at the next architecture review.

### Negative
- **A large, mostly-mechanical move.** Many files change path; open branches will
  conflict. Mitigated by doing it in one PR, immediately after the line-ending
  normalisation.
- **`internal/platform` risks becoming a bucket.** Mitigated by requiring each
  sub-package to be named for a concept, never `common`, `shared`, or `helpers`.
- **`depguard` upkeep** as new dependencies are added.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Keep a separate tree for "pluggable" capabilities alongside core modules** | The one attempt at this in the tree, `internal/extension/identity/`, produced 1,076 LOC that nothing imports. A second location for business code is a second place for it to be forgotten. |
| **Flat `internal/` with layer-first packages** | The current arrangement of the profile-user stack. Layer-first grouping scatters one capability across four directories and scales poorly; conflicts with the bounded-context emphasis of ADR-001. |
| **One Go module per business module** | Real isolation, but version-bump overhead for every cross-cutting change. Revisit only if teams need independent release cadences. |
| **Convention documented, enforced by review** | Already the status quo, and there is no evidence it is being applied: no linter, no architecture test, no CI. A rule nothing checks is indistinguishable from no rule. |

## Implementation Plan

1. ✅ Move the auth capability into `modules/identity/**` — done in Phase 1
   (`5f6edfe`, `6d0c1b7`), from `internal/{domain,application,infrastructure}/auth/**`.
2. Add the `depguard` configuration and the module-isolation test — **before**
   the remaining move, so the new structure is protected from the first commit.
3. Move the profile-user stack (`internal/domain/user`,
   `internal/application/user`, its persistence repository and HTTP handler) into
   `modules/user/`.
4. Delete `internal/domain/` and `internal/application/` **only once step 3 has
   landed and nothing imports them** — they hold live code that
   `cmd/api/container.go` depends on today.
5. Delete `internal/extension/`, including the unreferenced
   `internal/extension/identity/`.

## Compliance / Enforcement

- `golangci-lint` with `depguard` rules for layer purity and platform independence.
- `tools/archtest` test asserting no module imports another module.
- Review checklist: no bun model outside `infrastructure/persistence`; no domain
  entity serialised directly in `transport`.

## Related ADRs
- [ADR-001: Adopt Domain-Driven Design](001%20-%20Adopt%20Domain%20Driven%20Design.md) — this ADR makes its layering rules enforceable
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
- [ADR-010: Module-Owned Database Migrations](010%20-%20Module-Owned%20Database%20Migrations.md)

## Related Documents
- [Module layout](../01-modularity/module-layout.md)
- [Dependency rules](../01-modularity/dependency-rules.md)

---

**This ADR serves as a binding architectural decision for the project.**
