# Design Principles

**Status:** Proposed
**Date:** 2026-08-02

Nine principles, each derived from a concrete failure found in the current
codebase. They are ordered by how much damage their violation caused.

---

## 1. Wiring must fail loudly, never silently

> A component that is not connected must break the build or the boot — never
> return a successful empty result.

**Violation found:** `container.New()` returns `&Container{registrars: []{}}, nil`.
The application starts, logs "application starting", reports healthy, and serves
nothing. Nothing anywhere reports a problem.

**Applied:**
- `Build()` returns an error when zero modules are registered.
- Module constructors return `(*Module, error)`, not `*Module`.
- A smoke test asserts the expected route table is non-empty.

---

## 2. One way to do each thing

> Two mechanisms for the same job is not flexibility — it is a fork in the
> codebase that nobody merges.

**Violation found:** `internal/extension/` (Manager) and `internal/plugin/`
(Registry) both provide registration, lifecycle, and service lookup. ~1,100 LOC
total; one is unreachable. Separately, three directory layouts coexist.

**Applied:** one module contract ([ADR-004](../adr/004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)),
one layout ([ADR-005](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)).
When a replacement lands, the replaced code is deleted **in the same PR**.

---

## 3. Dependencies point inward and downward

> `domain` depends on nothing. `extensions`/`modules` never depend on the host's
> private helpers.

**Violation found:** 11 files under `extensions/identity/` import
`internal/utils`. The tree cannot be extracted or versioned separately.

**Applied:** shared code moves to an explicit `internal/platform` (shared kernel)
with a stable, documented API, or is vendored into the module. Enforced by an
import linter in CI, not by review discipline.

---

## 4. Configuration is data; dependencies are types

> Anything a YAML file cannot express must not be passed through a config map.

**Violation found:** the FIDO2 plugin requires `cfg["user_store"]` to be a live
Go value implementing `UserStore`. Viper can only produce strings, numbers, maps,
and slices — so the plugin returns `"user_store is required"` unconditionally.
383 LOC that can never execute.

**Applied:** `map[string]interface{}` disappears from plugin/module init. Config
is a typed struct; collaborators arrive through a typed `Deps` struct.

---

## 5. Never hold a lock across work you do not control

> Locks protect state, not call trees.

**Violation found:** the rate limiter takes a global mutex with `defer` and then
calls `next.ServeHTTP` inside the critical section. Server throughput drops to
one concurrent request.

**Applied:** compute the decision under the lock, release, then call downstream.
Reviews reject any `defer mu.Unlock()` in a function that invokes a callback or
handler.

---

## 6. Security defaults are restrictive; permissive is opt-in

> The default configuration must be the safe one.

**Violations found:**
- CORS defaults to `["*"]` and reflects any `Origin` back to the caller.
- JWT parsing does not pin the signing algorithm.
- `X-Forwarded-For` is trusted with no trusted-proxy check.
- Token validation skips `iss`, `aud`, and scope.

**Applied:** empty allowlist means *deny*; algorithms are pinned explicitly;
proxy headers are honoured only from configured CIDRs; every token check
validates issuer, audience, and expected scope.

---

## 7. Secrets never enter the repository

> A credential in git is compromised the moment it is pushed, regardless of what
> happens next.

**Violations found:** DB password `"gaws"` and a JWT secret in
`config/default.yaml`; `keys/private.pem` present and **not** git-ignored;
`.gitignore` itself malformed by stray markdown fences.

**Applied:** placeholder detection that fails startup, secrets from environment
or secret store only, and a repaired ignore file covering `keys/`, `*.pem`,
`.env*`, `bin/`.

---

## 8. The repository holds sources, not outputs

> If a command can regenerate it, it does not belong in version control.

**Violation found:** ~120 MB of compiled binaries in git history (`bin/kopiochi`
twice, `bin/kopiochi-migrate`, a root `kopiochi.exe`), leaving `.git` at 58 MB
for an 8k-LOC project.

**Applied:** artifacts ignored and purged from history
([ADR-011](../adr/011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md));
generated swagger output regenerated in CI rather than hand-committed.

---

## 9. Every rule has an automated check

> An unenforced convention is a comment.

**Violation found:** zero test files; `make test`, `make test-coverage`, and
`make ci` all pass vacuously. `gofmt -l` flags 100% of files because of CRLF with
no `.gitattributes`, so formatting signal is pure noise.

**Applied:** `.gitattributes` normalises line endings so `gofmt` means something;
CI runs build, vet, import-linter, tests with a coverage floor, `govulncheck`,
and a secret scan. Each principle above maps to at least one CI check.

---

## Applying these to a new feature

Before merging, confirm:

- [ ] The feature is reachable — a route table test proves it, not a manual check.
- [ ] It uses the existing mechanism; nothing parallel was introduced.
- [ ] `domain` imports no infrastructure library.
- [ ] Config is typed; collaborators come from `Deps`.
- [ ] No lock spans a handler or callback invocation.
- [ ] Defaults deny; permissive behaviour is explicit in config.
- [ ] No secret, key, or artifact is added to the repo.
- [ ] Tests cover the new behaviour and CI enforces them.

---

## Related documents

- [Current state](current-state.md) — the violations in full
- [Target architecture](target-architecture.md) — the structure these produce
- [Dependency rules](../01-modularity/dependency-rules.md) — mechanical enforcement
