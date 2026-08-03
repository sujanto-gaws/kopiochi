# ADR-004: Consolidate on a Single Extension Framework

## Status
**Accepted — fully implemented** – *Decided: 2026-08-02 · Steps 1–3: 2026-08-02 (Phase 1) · Steps 4–6: 2026-08-03 (Phase 3.6, `de7e242`)*

The consolidation is complete. `internal/extension/`, `internal/plugin/`,
`internal/plugins/` and `examples/extension-demo/` were deleted — 4,023 lines —
leaving `internal/module.Module` as the single registration mechanism.

The outcome differs from the decision in one respect worth recording: the two
frameworks were not consolidated *into* one another. Nothing depended on either
of them, so both were deleted outright and the middleware they nominally
hosted — CORS and rate limiting — became direct construction from typed config
in `internal/httpx`. Roughly 1,100 lines of registration machinery collapsed
into one `if` per middleware. Consolidating them would have preserved a
mechanism that had no users.

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

1. ✅ Add `internal/module` with `Deps` and `Module` — `05b1051`. The shipped
   `Deps` carries `DB` and `Logger` only.
2. ✅ Add `modules/identity/module.go` implementing `New()` over the existing
   identity internals (move code, do not rewrite logic) — `5f6edfe`, `6d0c1b7`.
3. ✅ Wire it in `cmd/api/container.go`; verify routes respond — `ef76759`,
   `4fdc609`; verified by `cmd/api/routes_test.go` and `cmd/api/login_e2e_test.go`.
4. ⏳ Convert CORS and rate limiting to direct construction in `internal/httpx` —
   Phase 3.5, not started.
5. ⏳ Delete `internal/extension/`, `internal/plugin/`, `internal/plugins/`,
   `examples/extension-demo/` — Phase 3.6, not started.
6. ⏳ `go mod tidy`; drop the now-unused dependencies — Phase 3.7, not started.

Steps 1–3 are additive and independently shippable, and have shipped. Step 5 is
mandatory: until it runs, this ADR has added a mechanism rather than
consolidating on one.

## Compliance / Enforcement

- No new package may define a `Plugin`, `Extension`, or `Registry` interface.
- Code review rejects any `map[string]interface{}` parameter in a constructor.
- The import linter denies imports of the deleted packages by path, so a revert
  cannot reintroduce them silently. *Not in place — there is no `.golangci.yml`
  and no CI.*
- A route-table test asserts every registered module actually serves routes.
  *In place: `cmd/api/routes_test.go` (`d92480c`).*

## Related ADRs
- [ADR-005: Module Boundaries and Dependency Direction](005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)
- [ADR-006: Explicit Compile-Time Dependency Injection](006%20-%20Explicit%20Compile-Time%20Dependency%20Injection.md)
- [ADR-001: Adopt Domain-Driven Design](001%20-%20Adopt%20Domain%20Driven%20Design.md)

## Related Documents
- [Extension framework](../01-modularity/extension-framework.md)

---

## Correction (2026-08-02)

*Appended rather than edited into the Context, per the append-only rule for
accepted ADRs stated in [the documentation README](../README.md).*

**Withdrawn**, from the Context's list of observed consequences:

> The entire identity extension (~2,000 LOC of working authentication, MFA, and
> role management) is written against Framework A and is therefore **never
> loaded** by the server.

**Why:** that is not what happened. The live auth stack was ordinary DDD code
under `internal/{domain,application,infrastructure}/auth`, wired through the
container and reachable the whole time; Phase 1 moved it to `modules/identity/**`
(`5f6edfe`, `6d0c1b7`) and it now serves `/api/v1/auth/*`. No `extensions/`
directory has ever existed in this repository —
`git log --all --diff-filter=A -- extensions/` returns no commits — so the
Context's citation of `extensions/identity/extension.go` as a Framework A
importer is also withdrawn.

**Corrected fact.** Framework A's only importers are
`examples/extension-demo/main.go` and `internal/extension/identity/extension.go`.
The latter is a **parallel** identity implementation — 1,076 LOC across four
files, no transport layer, no importer of its own — not the live auth stack. It
is dead code that the framework's existence justifies, which strengthens rather
than weakens the case for deletion, but it is a quarter the size claimed and it
was never the application's authentication.

**Unaffected.** Every other item in the Context is verified and stands: the
`internal/plugins/adapters.go` indirection, the FIDO2 `cfg["user_store"]` defect
(`internal/plugins/auth/fido2.go:92-100`, 383 LOC that cannot initialise), the
`Registry.Initialize` re-init leak, and the silent config-type fallbacks in
`ratelimit.go:33`. The Decision, Consequences, Alternatives, and Implementation
Plan are unchanged.

---

**This ADR serves as a binding architectural decision for the project.**
