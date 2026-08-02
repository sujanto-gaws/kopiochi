# Design Principles

**Status:** Partially implemented
**Date:** 2026-08-02
**Last verified:** 2026-08-02, after Phases 0 and 1

Nine principles, each derived from a concrete failure found in the codebase at
review time. They are ordered by how much damage their violation caused. Where a
violation has since been fixed, the **Applied** block says so and names the
commit; the principles themselves are unchanged.

---

## 1. Wiring must fail loudly, never silently

> A component that is not connected must break the build or the boot — never
> return a successful empty result.

**Violation found:** the old `container.New()` returned `(*Container, error)`
whatever it appended to its registrar list. Its completeness was a property of a
comment — "To add a new handler: wire it above and append it here" — so a handler
left out of the slice was simply not served, with no compile error, no boot
failure, and no test to notice.

> *An earlier revision of this document quoted `container.New()` as returning
> `&Container{registrars: []{}}, nil` and described an application that "serves
> nothing". That excerpt matches no commit in this repository's history —
> `794d783` and `0fbab20` both return two registrars — and has been withdrawn. The
> principle is unchanged; it rests on the design property above, not on the
> container ever having been empty.*

**Applied** — all three shipped in `ef76759` / `d92480c`:
- `BuildApp()` returns an error when zero modules are registered
  (`cmd/api/container.go:66-68`). The guard is currently unreachable, since two
  modules are appended unconditionally; it encodes the rule for future edits.
- Module constructors return `(*Module, error)`, not `*Module`.
- A smoke test asserts the expected route table is non-empty
  (`cmd/api/routes_test.go`), alongside `cmd/api/container_test.go`.

---

## 2. One way to do each thing

> Two mechanisms for the same job is not flexibility — it is a fork in the
> codebase that nobody merges.

**Violation found:** `internal/extension/` (Manager) and `internal/plugin/`
(Registry) both provide registration, lifecycle, and service lookup. ~1,100 LOC
total; one is unreachable, and it has a 1,076-LOC parallel identity
implementation (`internal/extension/identity/`) attached to it. Separately,
business code sits in two layouts: `modules/identity/` and the layer-first
`internal/{domain,application,infrastructure}` profile-user stack.

**Applied:** one module contract ([ADR-004](../adr/004%20-%20Consolidate%20on%20a%20Single%20Extension%20Framework.md)),
one layout ([ADR-005](../adr/005%20-%20Module%20Boundaries%20and%20Dependency%20Direction.md)).
When a replacement lands, the replaced code is deleted **in the same PR**.

---

## 3. Dependencies point inward and downward

> `domain` depends on nothing. `modules/` never depends on the host's private
> helpers beyond a named, stable shared kernel.

**Violation found:** nothing enforces this. There is no `.golangci.yml`, no
`depguard` configuration, no architecture test, and no CI — so the rule holds
only for as long as every reviewer remembers it.

> *An earlier revision cited "11 files under `extensions/identity/` import
> `internal/utils`" as the violation. Neither path appears in any commit of this
> repository; the claim has been withdrawn. The principle stands on its own.*

**Applied:** shared code moves to an explicit `internal/platform` (shared kernel)
with a stable, documented API, or stays inside the module that owns the concept.
Enforced by an import linter in CI, not by review discipline.

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
`.gitignore` itself malformed by stray markdown fences. *All three fixed in
Phase 0 — `b74b358`, `8652534`, `4c72a83` — but only in the working tree: the
values are still in git history until the Phase 5.1 rewrite, and rotating them is
an out-of-repo action that has not been confirmed.*

**Applied:** secrets from environment or secret store only, and a repaired ignore
file covering `keys/`, `*.pem`, `.env*`, `bin/` — both done. Placeholder
detection that fails startup is **not** done (Phase 2.9); `.env.example` ships
`CHANGEME_*` values that the loader accepts without complaint.

---

## 8. The repository holds sources, not outputs

> If a command can regenerate it, it does not belong in version control.

**Violation found:** ~120 MB of compiled binaries in git history (`bin/kopiochi`
twice, `bin/kopiochi-migrate`, a root `kopiochi.exe`), leaving `.git` at 58 MB
for an 8k-LOC project. *Still true of history.* The working tree stopped adding
to it in `1c5ac2c` / `4c72a83`.

**Applied:** artifacts ignored (done) and purged from history (**not** done —
Phase 5.1)
([ADR-011](../adr/011%20-%20Build%20Artifacts%20Excluded%20from%20Version%20Control.md));
generated swagger output regenerated in CI rather than hand-committed.

---

## 9. Every rule has an automated check

> An unenforced convention is a comment.

**Violation found:** zero test files; `make test`, `make test-coverage`, and
`make ci` all pass vacuously. `gofmt -l` flags 100% of files because of CRLF with
no `.gitattributes`, so formatting signal is pure noise. *Both fixed: seven test
files (`d92480c`, `720e580`), and `gofmt -l .` is clean (`b294de2`, `3dbd1b4`).*

**Applied:** `.gitattributes` normalises line endings so `gofmt` means something
— done. CI runs build, vet, import-linter, tests with a coverage floor,
`govulncheck`, and a secret scan — **not done; the repository has no CI
configuration at all.** Each principle above maps to at least one CI check, and
none of those checks is currently automated, which makes this the least-applied
principle in the list.

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
