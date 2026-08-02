# ADR-004: Consolidate on a Single Extension Framework

## Status
**Proposed** – *Date: 2026-08-02*

## Context

The codebase contains two complete, competing frameworks for registering and
managing pluggable functionality:

**Framework A — `Manager`** (`internal/extension/`, ~600 LOC). Yii-inspired, with
a five-pass bootstrap (`EarlyBootstrap`, `Bootstrap`, `Routes`, `Services`,
`Events`), a string-keyed service registry, and an event bus. It is imported by
`examples/extension-demo/` and by `extensions/identity/extension.go`.

**Framework B — `Registry`** (`internal/plugin/` + `internal/plugins/`, ~500 LOC).
A factory registry with typed accessors (`GetAuth`, `GetMiddleware`, `GetCache`)
and `map[string]interface{}` initialisation. It is the one `cmd/api/main.go`
actually instantiates.

Consequences observed:

- The entire identity extension (~2,000 LOC of working authentication, MFA, and
  role management) is written against Framework A and is therefore **never
  loaded** by the server.
- `internal/plugins/adapters.go` exists solely to wrap concrete plugins so they
  satisfy Framework B's interfaces — indirection that exists because the
  interfaces were designed twice.
- The `map[string]interface{}` initialisation contract cannot express live
  collaborators. `internal/plugins/auth/fido2.go:92-100` requires
  `cfg["user_store"]` to be a Go value implementing `UserStore`, but the config
  originates from Viper, which produces only strings, numbers, maps, and slices.
  The plugin returns `"user_store is required"` under every possible
  configuration — 383 LOC that cannot execute.
- `Registry.Initialize` overwrites an already-initialised plugin without calling
  `Close()` on it, leaking any goroutine or connection it held.
- Plugin config is parsed with unchecked type assertions that silently fall back
  to defaults (`ratelimit.go:33`), so a YAML type error produces wrong behaviour
  with no diagnostic.

The functionality actually required is modest: mount some routes, apply some
middleware, own some migrations. Neither framework's lifecycle machinery, event
bus, or service locator is used by any code that runs.

## Decision

We will **delete both frameworks** and replace them with a plain constructor
contract.

1. **One module type.** `internal/module` defines `Deps` (live collaborators as a
   struct) and `Module` (name, `Routes func(chi.Router)`, `Migrations fs.FS`,
   optional `Close`).
2. **Constructor per module.** Each module exposes `New(deps module.Deps, cfg
   Config) (*module.Module, error)`. Misconfiguration fails at boot.
3. **Typed config per module.** Each module declares its own config struct with
   `mapstructure` tags and a `Validate() error`. `map[string]interface{}`
   disappears from module and plugin initialisation.
4. **Dependencies are parameters, not config keys.** Anything a YAML file cannot
   express is passed as a typed constructor argument.
5. **Cross-cutting HTTP middleware is not a plugin.** CORS and rate limiting
   become direct construction in `internal/httpx`, guarded by a config boolean.
6. **Deletion is part of the change.** `internal/extension/`, `internal/plugin/`,
   `internal/plugins/`, `internal/plugins/adapters.go`, and
   `examples/extension-demo/` are removed in the same effort, not left in place.

We will **not** adopt Go's `plugin` package or any dynamic loading mechanism.
Modules are compiled in.

## Consequences

### Positive
- **Dead code becomes reachable.** The identity stack is wired through the same
  path as everything else.
- **Wiring errors become compile errors.** A missing or mistyped dependency
  cannot reach runtime.
- **~1,100 LOC of framework is deleted**, along with the adapter layer.
- **The FIDO2 failure mode is unrepresentable** — a store is a constructor
  parameter, so its absence does not compile.
- **Fewer dependencies.** Removing the unreachable FIDO2 plugin drops
  `go-webauthn`, `go-tpm`, and `fxamacker/cbor` from the module graph.
- **No re-init leak**, because modules are constructed exactly once.

### Negative
- **Loss of runtime pluggability.** Enabling a module now requires a code change
  and redeploy, not a config edit. Given that no module has ever been toggled in
  production and that config-driven activation produced the FIDO2 bug, this is
  an acceptable trade.
- **The composition root grows.** `cmd/api/container.go` gains a block per
  module. This is explicit and greppable, which is the point.
- **Migration effort.** The identity extension's `extension.go` must be rewritten
  against the new contract (its domain/application code moves unchanged).
- **The event bus is lost.** Nothing currently uses it; if inter-module events are
  needed later, they should be introduced deliberately with a real requirement.

## Alternatives Considered

| Alternative | Reason for rejection |
|---|---|
| **Keep Framework A (`Manager`)** | Its five-pass bootstrap, event bus, and string-keyed service locator are unused complexity; `RegisterService` eagerly invokes "lazy" factories; `AddRoute` silently warns and drops routes when no router is set. |
| **Keep Framework B (`Registry`)** | Its `map[string]interface{}` contract is the direct cause of the FIDO2 defect and of silent config-type fallbacks; it cannot express dependencies. |
| **Keep both, document when to use which** | Two mechanisms for one job is what produced the split in the first place. |
| **Adopt a DI framework (`wire`, `fx`, `dig`)** | Disproportionate for two modules; adds codegen or reflection. Revisit past ~12 modules. See [ADR-006](006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md). |
| **Go `plugin` package / dynamic loading** | Linux-only in practice, brittle across Go versions, no benefit here. |

## Implementation Plan

1. Add `internal/module` with `Deps` and `Module`.
2. Add `modules/identity/module.go` implementing `New()` over the existing
   identity internals (move code, do not rewrite logic).
3. Wire it in `cmd/api/container.go`; verify routes respond.
4. Convert CORS and rate limiting to direct construction in `internal/httpx`.
5. Delete `internal/extension/`, `internal/plugin/`, `internal/plugins/`,
   `examples/extension-demo/`.
6. `go mod tidy`; drop the now-unused dependencies.

Steps 1–3 are additive and independently shippable. Step 5 is mandatory.

## Compliance / Enforcement

- No new package may define a `Plugin`, `Extension`, or `Registry` interface.
- Code review rejects any `map[string]interface{}` parameter in a constructor.
- The import linter denies imports of the deleted packages by path, so a revert
  cannot reintroduce them silently.
- A route-table test asserts every registered module actually serves routes.

## Related ADRs
- [ADR-005: Module Boundaries and Dependency Direction](005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [ADR-006: Explicit Compile-Time Dependency Injection](006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
- [ADR-001: Adopt Domain-Driven Design](001%20-%20Adopt%20Domain%20Driven%20Design.md)

## Related Documents
- [Extension framework](../01-modularity/extension-framework.md)

---

**This ADR serves as a binding architectural decision for the project.**
