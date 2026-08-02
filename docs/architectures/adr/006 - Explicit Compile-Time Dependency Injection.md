# ADR-006: Explicit Compile-Time Dependency Injection

## Status
**Accepted — implemented** – *Decided: 2026-08-02 · Implemented: 2026-08-02 (Phase 1)*

All six implementation steps have shipped; see the Implementation Plan below for
per-step commits. The Context section is preserved as written — but note that the
"empty container" excerpt it quotes does not match any commit in this
repository's history, and is being re-verified. The Decision stands on its own
merits regardless of that excerpt.

## Context

`cmd/api/container/container.go` is the project's composition root. Its body is
three section comments and an empty slice:

```go
func New(cfg *config.Config, db bun.IDB) (*Container, error) {
	// ── Shared infrastructure ─────────────────────────────────────
	// ── Auth ──────────────────────────────────────────────────────
	// ── User ──────────────────────────────────────────────────────
	return &Container{
		registrars: []handlers.RouteRegistrar{
			// Auth
		},
	}, nil
}
```

Both parameters are unused. The function returns success. The server starts, logs
`"application starting"`, connects and pings the database, reports healthy on
`/api/health`, and **serves no business routes at all**. Nothing — not the
compiler, not startup, not the health check, not a test — reports a problem.

The container's correctness is a property of a code comment ("To add a new
handler: wire it above and append it here") rather than of the type system. There
is no signal distinguishing an intentionally empty container from an unfinished
one.

Related runtime-lookup patterns exist in the frameworks being removed by
[ADR-004](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md):
`Manager.GetService(name string) (interface{}, bool)` resolves collaborators by
string with an untyped result, and `RegisterService` eagerly invokes the factory
it stores, so the "lazy factory" API is not lazy.

## Decision

Dependencies are wired **explicitly, in Go, at compile time**, in a single
composition root.

1. **Constructor injection everywhere.** Every component receives its
   collaborators as constructor parameters. No service locator, no string-keyed
   lookup, no `map[string]interface{}`.
2. **One composition root:** `cmd/api/container.go`, the only file that imports
   every module.
3. **`BuildApp` refuses to produce an empty application.** An explicit
   `len(modules) == 0` check returns an error, so the empty-container state
   cannot start.
4. **Constructors return `(T, error)`.** Missing keys, unreadable files, and
   invalid config fail at boot with a wrapped error naming the module.
5. **No DI framework.** No `wire`, `dig`, or `fx`.
6. **Consumers declare interfaces.** Repository and service interfaces live in
   the consuming package (`domain`), satisfied structurally by infrastructure
   implementations.
7. **Wiring is observable.** Registered module names are logged at startup.

## Consequences

### Positive
- **The failure that made the application inert becomes impossible** — an empty
  application cannot boot.
- **Wiring mistakes are compile errors.** A missing dependency does not build.
- **No reflection, no codegen.** Stack traces point at real code; the wiring is
  greppable and debuggable.
- **Testable composition.** `BuildApp` can be asserted on directly, and modules
  can be constructed with fakes in tests.
- **Boot-time validation.** Bad config surfaces at startup, not on the first
  request that happens to touch it.

### Negative
- **Manual wiring grows** with module count. Acceptable at two modules; revisit
  past roughly a dozen.
- **Constructor churn.** Adding a dependency to a deep component touches the call
  chain — which is a visibility benefit as much as a cost.
- **No lazy initialisation.** Everything is constructed at boot. For this
  application (one DB pool, one token service) that is desirable: failures happen
  before traffic arrives.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **`google/wire`** | Compile-time and type-safe, but adds a codegen step and a DSL for a graph small enough to read on one screen. Reconsider past ~12 modules. |
| **`uber-go/fx` or `dig`** | Reflection-based; wiring errors move from compile time to runtime — the exact class of failure this ADR exists to eliminate. |
| **Service locator (`GetService(name)`)** | Already present in `internal/extension`; untyped, runtime-failing, and it hides the dependency graph. |
| **Keep the current registrar list, add a lint rule** | Does not address the root problem: "wired" is not expressible in the type system, and a lint rule cannot tell intentional emptiness from an unfinished one. |
| **Package-level globals + `init()`** | Untestable, unordered initialisation, hidden coupling. |

## Implementation Plan — complete

1. ✅ Define `module.Deps` and `module.Module` (per ADR-004) — `05b1051`. The
   shipped `Deps` carries `DB` and `Logger`; no `Clock`.
2. ✅ Replace `cmd/api/container/container.go` with `cmd/api/container.go`
   exposing `BuildApp(cfg, db, log) (*App, error)`, including the zero-module
   guard — `ef76759`.
3. ✅ Construct each module explicitly, wrapping errors with the module name —
   `ef76759` (`"build identity module: %w"`, `"build user module: %w"`).
4. ✅ Update `main.go` to call `BuildApp` and pass `app.Modules` to route mounting
   — `ef76759`, `4fdc609`.
5. ✅ Add tests: `TestBuildApp_RegistersModules` and
   `TestBuildApp_FailsOnInvalidConfig` — `d92480c`, in `cmd/api/container_test.go`.
6. ✅ Delete `handlers.RouteRegistrar` and `handlers.RouterGroup` once unused —
   `4fdc609`; neither identifier remains anywhere in the tree.

## Compliance / Enforcement

- `BuildApp` must return an error when no module is registered; a test asserts it.
  *Caveat: the guard is currently unreachable, because two modules are appended
  unconditionally before it. It encodes the rule for future edits; it does not
  today execute.*
- A route-table test asserts expected routes exist — the practical guard against
  silent emptiness. *Shipped as `TestRouteTable` in `cmd/api/routes_test.go`.*
- Review rejects new service-locator lookups and `map[string]interface{}`
  constructor parameters.
- Only `cmd/**` may import more than one module.

## Related ADRs
- [ADR-004: Consolidate on a Single Extension Framework](004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)
- [ADR-005: Module Boundaries and Dependency Direction](005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [ADR-007: API Versioning at the Router Boundary](007%20-%20API%20Versioning%20at%20the%20Router%20Boundary.md)

## Related Documents
- [Dependency injection](../02-composition/dependency-injection.md)

---

**This ADR serves as a binding architectural decision for the project.**
