# Security SPI (`internal/authn`) — Impact Analysis, Before / After

Target repo: `sujanto-gaws/kopiochi`
Companion to: `notification-module-blueprint.md`
Scope: extract the minimal auth contract into `internal/authn`, demote `modules/identity`
to the default implementation, refit consumers. No API surface changes for HTTP clients
(one caveat, §8).

---

## 1. Summary of the change

| | Before | After |
|---|---|---|
| Auth contract | Implicit: "a middleware, plus whatever context key identity uses" | Explicit: `internal/authn` — `Principal`, `Middleware`, `WithPrincipal`, `FromContext` |
| Contract owner | `modules/identity` (de facto) | Shared kernel (`internal/authn`) |
| Consumer coupling | `user` handlers semantically bound to identity's context key | Consumers import only `internal/authn` |
| Enforceability | Invisible to depguard/archtest (context value, not import) | Real import edge, mechanically checkable |
| Swap auth cost | Rewrite consumer handlers + reverse-engineer identity internals | New module honoring the contract + 1 line in `BuildApp` + pass conformance suite |
| 401 semantics | Defined by identity's implementation | Defined by `httpx.Unauthorized` helper; contract requires it |
| Transport tests (non-identity modules) | Need real JWTs (keypair, `MintToken`) | Inject a `Principal` with a 3-line fake middleware |

Net LOC estimate: +~350 (authn ~60, authntest ~150, httpx helper ~40, docs/policy ~100),
−~60 (deleted duplication in consumers), refactor-in-place ~200.

---

## 2. New package: `internal/authn`

**Before:** does not exist. The concepts live scattered: middleware type appears as a raw
`func(http.Handler) http.Handler` in `user.Config`; the caller's identity is a claims
struct + unexported context key inside `modules/identity`.

**After:**

```go
package authn // internal/authn — imports: context, net/http. Nothing else.

type Principal struct {
    Subject string            // stable account id. Required; empty Subject is invalid.
    Scopes  []string          // optional
    Extra   map[string]string // opaque to the kernel; custom claims live here
}

type Middleware = func(http.Handler) http.Handler

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context
func FromContext(ctx context.Context) (Principal, bool)
func MustFromContext(ctx context.Context) Principal // panics; for handlers behind Middleware
```

Rules encoded here, deliberately:

- No `Validate(token)` on the contract — token format is an implementation detail.
- No `Authorizer`/roles interface — no consumer needs one today (YAGNI; see the deleted
  plugin registries for what speculative contracts become).
- `Extra` is `map[string]string`, not `map[string]any` — forces custom claims to stay
  serializable and comparison-friendly; richer needs mean a custom middleware wrapper,
  which is the intended extension path anyway.

Impact elsewhere: `internal/**` gains one package; nothing in `internal/**` imports it
except `httpx` (for the helper) — the kernel stays flat.

---

## 3. `modules/identity`

**Before (per README):** owns RS256 validation *and* the middleware *and* the context
contract. `AuthMiddleware` on success stores its own claims value under its own key.
Consumers that need the subject must read identity's key — the hidden edge.

**After:** identity keeps 100% of its crypto and token logic. The middleware's success
path changes by ~5 lines:

```go
// modules/identity/transport/middleware.go (illustrative)
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        claims, err := m.verifier.Verify(r)          // unchanged
        if err != nil {
            httpx.Unauthorized(w, r)                 // was: local error write
            return
        }
        p := authn.Principal{Subject: claims.Subject, Scopes: claims.Scopes}
        next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), p)))
    })
}
```

Identity's own handlers (logout, MFA setup — the ones behind its middleware) switch to
`authn.FromContext` too, eating their own dog food.

What identity **loses**: ownership of the context key and of the 401 response shape.
What it **keeps**: token issuance, verification, key rotation, refresh families, MFA —
all provider-internal, untouched.

Layer-rule check: `transport` may import `internal/**` already? It may not today
(`transport → application, domain` per README). Two options: (a) widen transport's
allowed imports to include `internal/authn` + `internal/httpx` — reasonable, transport
is HTTP; (b) keep the table strict and have the middleware live in infrastructure.
Recommend (a), updating the depguard rule and the README table in the same PR. This is
the one place the change touches the enforced rules, so make it explicit and reviewed.

---

## 4. `modules/user` (template for every future consumer, incl. notification)

**Before:**

```go
type Config struct {
    Auth func(http.Handler) http.Handler // untyped; meaning by convention
}
// handler: subject comes from identity's context key — an import that isn't an import
```

Transport tests must mint real tokens (`testsupport.MintToken`, a keypair via
`testsupport.Config`) just to exercise a CRUD handler.

**After:**

```go
type Config struct {
    Auth authn.Middleware // named type; self-documenting
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
    p := authn.MustFromContext(r.Context())
    // ownership checks against p.Subject
}
```

Transport tests shrink materially:

```go
fakeAuth := func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        next.ServeHTTP(w, r.WithContext(
            authn.WithPrincipal(r.Context(), authn.Principal{Subject: "u-123"})))
    })
}
```

No keypair, no JWT, no clock — the handler test tests the handler. `MintToken` remains,
but only identity's own integration tests need it. Expect the user transport test files
to get shorter and faster; this is the largest day-to-day quality-of-life gain.

`user.New` still fails closed on a nil `Auth` — unchanged posture, now with a typed nil
check.

---

## 5. `internal/httpx`

**Before:** problem+json helpers exist; the 401 shape is whatever identity's middleware
writes.

**After:** one canonical rejection, ~40 lines:

```go
// httpx.Unauthorized writes the canonical 401: problem+json body,
// WWW-Authenticate: Bearer, no detail leakage about *why* the credential failed.
func Unauthorized(w http.ResponseWriter, r *http.Request)
```

Design note: deliberately reason-free. Distinguishing "expired" vs "bad signature" vs
"revoked" in the body is an information leak and makes implementations diverge; clients
get one behavior — re-authenticate.

---

## 6. `cmd/api/container.go`

**Before/after: nearly a no-op.** The wiring already passes a middleware value; only the
type names tighten:

```go
idMod, authMW, err := identity.New(deps, cfg.Identity)   // authMW is authn.Middleware
userMod, err := user.New(deps, user.Config{Auth: authMW})
```

(If identity's constructor doesn't currently return the middleware separately, this is
the same provider-module question settled for notification — same answer: second return
value, typed by the contract, here the contract being `authn.Middleware`.)

The **replacement recipe** this enables, verbatim for the docs:

```go
// swap: comment one, uncomment the other. Nothing else changes.
_, authMW, err := identity.New(deps, cfg.Identity)
// _, authMW, err := oidc.New(deps, cfg.OIDC)
```

---

## 7. Tests, tooling, policy

| Artifact | Before | After |
|---|---|---|
| `tools/archtest` | Cannot see the ctx-key coupling | Add rule: only `modules/*` and `internal/httpx` may import `internal/authn`; consumers must not import `modules/identity` (already enforced, now also *sufficient*) |
| `.golangci.yml` depguard | transport layer forbids `internal/**` (if strict) | Allow `internal/authn`, `internal/httpx` in transport (§3 decision) |
| `tools/coverage/policy.json` | — | `internal/authn` floor 90% (it's pure logic), reason recorded |
| `internal/testsupport` | `MintToken` needed by every protected-route test | Add `testsupport.FakeAuth(subject string) authn.Middleware`; `MintToken` scoped to identity |
| **`internal/authn/authntest`** (new) | — | Conformance suite: `RunMiddlewareSuite(t, mw, mintValid, mintExpired)` asserting: valid → principal present + subject matches; missing/expired/garbage → canonical 401 incl. `WWW-Authenticate`; handler panic not swallowed; principal absent after rejection |

The conformance suite is the artifact that converts "replaceable" from a claim into a
guarantee — identity passes it in CI, and any adopter's replacement passes the same
suite before it's trusted.

---

## 8. Behavioral impact on HTTP clients — the 401 caveat, resolved

**None intended, one caveat, now with a resolution plan.** Route table, tokens, cookies,
status codes: identical. The caveat: identity's current 401 *body* may differ from the
canonical `httpx.Unauthorized` shape, so the error body changes for clients. Resolution
is four steps, folded into PR 1 (§11):

### 8.1 Capture the current shapes first (golden test, first commit of PR 1)

Before changing anything, record what the middleware emits today for every rejection
path, so the break is measured rather than guessed:

```go
func TestCurrent401Shapes(t *testing.T) {
    cases := map[string]func(*http.Request){
        "missing header":  func(r *http.Request) {},
        "malformed":       func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") },
        "expired":         func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testsupport.MintToken(t, Expired())) },
        "wrong algorithm": func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testsupport.MintToken(t, AlgNone())) },
        "wrong class":     func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testsupport.MintToken(t, RefreshClass())) },
    }
    // per case: hit a protected route; dump status, Content-Type, WWW-Authenticate,
    // and body to testdata/golden/401_<name>.json
}
```

Expected finding: the current rejections are already inconsistent *with each other*
(per-reason `detail` strings, header presence varying by path). That reframes the
caveat — this collapses several unstable shapes into one, not breaks one stable shape.
Committing the goldens first preserves the old shapes in git history as the audit trail.

### 8.2 Define the canonical shape in one place

`httpx.Unauthorized` emits exactly, documented as the contract:

```
HTTP/1.1 401 Unauthorized
Content-Type: application/problem+json
WWW-Authenticate: Bearer realm="api"

{"type":"about:blank","title":"Unauthorized","status":401,
 "detail":"authentication required","instance":"/api/v1/users/42"}
```

Two deliberate properties: RFC 9457, consistent with the existing problem+json
responses; and `detail` is **identical for every failure reason**. Distinguishing
expired vs invalid vs revoked in the body leaks which tokens are structurally valid and
is exactly where a replacement auth module would drift from identity. Debuggability goes
to the server log, keyed by request ID — never the body.

### 8.3 Cut-over strategy: clean break (decided)

| Option | Verdict | Why |
|---|---|---|
| **Clean break** | **Chosen** | Boilerplate stage, no external consumers outside our control. This option expires the day someone adopts the template — use it while it exists. |
| Deprecation window (old fields carried alongside problem+json, dated removal) | Rejected for now | The standard move *when* external clients exist; unnecessary cost today. Documented here so an adopter with live clients knows the alternative. |
| Content negotiation (shape per `Accept` header) | Rejected | Doubles the conformance surface and keeps the inconsistency alive indefinitely. |

Changelog / `BOILERPLATE.md` note ships with the change: "401 responses are uniform
problem+json; clients must key off `status`, never `detail`."

### 8.4 Lock it permanently

Two guards, different scopes: the §8.1 goldens — rewritten to the canonical shape in
the same PR — pin **identity's** behavior; the `authntest` conformance suite (§7)
asserts the shape (status, `Content-Type`, `WWW-Authenticate` presence, reason-invariant
`detail`) for **any** middleware implementation, so a future OIDC replacement cannot
reintroduce drift. Between them, the 401 shape is something the build verifies, not
something a client discovers.

---

## 9. Impact on the notification plan

| Blueprint item | Before SPI | After SPI |
|---|---|---|
| `notification.Config` | `Auth func(http.Handler) http.Handler` | `Auth authn.Middleware` |
| Recipient scoping in handlers | Would have read identity's ctx key (spreading the hidden edge to a 2nd module) | `authn.MustFromContext(r.Context()).Subject` |
| Transport tests | Real JWTs | `testsupport.FakeAuth("u-123")` |
| `SecurityNotifier` seam | Unchanged | Unchanged — producer-side, orthogonal |
| Blueprint sections needing edits | — | §3 (Config field type), §7 (handler note) — two-line edits |

Building notification *before* the SPI would have doubled the consumers to migrate
later. Doing the SPI first means notification is born on the contract.

---

## 10. Risks and mitigations

1. **Context-key migration must be atomic.** Identity writing the new key while user
   reads the old one = every protected route silently 500s (or worse, passes with empty
   subject). Mitigation: PR 1 changes writer *and* readers together; `MustFromContext`
   panicking on absence turns a missed reader into a loud test failure, not a silent
   empty-string ownership check.
2. **Contract bloat over time.** Every future need will "just add a field/method".
   Mitigation: the package doc states the admission rule — a field enters `Principal`
   only when ≥2 consumer modules need it; everything else rides in `Extra` or a custom
   wrapper middleware.
3. **Canonical 401 breaks a client depending on the old error body.** Resolved by §8:
   golden capture before the change (measured, not guessed), clean-break cut-over while
   no external consumers exist, changelog note, and the conformance suite preventing
   recurrence. Residual risk: a client parsing `detail` strings — explicitly
   unsupported by the documented contract.
4. **depguard widening (§3) creeps.** Allow exactly `internal/authn` and
   `internal/httpx` in transport, not `internal/**`.

---

## 11. Rollout — three PRs, each independently green

**PR 1 — contract + implementation.** Internally sequenced as two commits per §8:
*commit 1* adds the golden-capture test recording the old 401 shapes (audit trail in git
history); *commit 2* adds `internal/authn` + `httpx.Unauthorized`, switches identity's
middleware to populate the contract and emit the canonical 401, switches identity's own
protected handlers to `FromContext`, rewrites the goldens to the canonical shape, and
updates depguard + the README layer table. All tests green with zero consumer changes
*except* the rewritten goldens and any test asserting the old 401 body.

**PR 2 — consumers + guarantee.** `user.Config` typed, handlers on `FromContext`,
`testsupport.FakeAuth`, `authntest` suite added and wired into identity's CI run,
archtest rule, coverage policy entries.

**PR 3 — docs.** `docs/architectures/` page: the contract, the failure semantics, the
replacement recipe, the conformance requirement. `BOILERPLATE.md` "to add a module"
section updated: consumer modules take `authn.Middleware`.

Then the notification blueprint proceeds unmodified except the two-line edits in §9.
