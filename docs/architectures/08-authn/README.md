# The Authentication Contract (`internal/authn`)

**Status:** Implemented — authn SPI Phases A and B
(`6288dcd`, `4a969f2`, `ea0d30f`, `1a72344`, `9c0ede5`, `36784b6`)
**Date:** 2026-08-11
**Last verified:** 2026-08-11, against `main` at `2dd621c` — every claim below is
cited to a file and line in that tree.

`internal/authn` is the package a module depends on to learn *who the caller is*
without learning *how the caller proved it*. It holds three things — a
`Principal`, a `Middleware` type, and the context plumbing between them — and it
imports `context` and `net/http` and nothing else
(`internal/authn/authn.go:32-35`).

Before it existed the same coupling was there but invisible: a module's config
carried a bare `func(http.Handler) http.Handler`, and its handlers read the
caller's id out of an unexported context key owned by a *different* module. A
context value is not an import, so neither depguard nor `tools/archtest` could
see the edge (`internal/authn/authn.go:9-13`). Naming the contract turns that
edge into one both tools can police, which is what
[R1 in the dependency rules](../01-modularity/dependency-rules.md) now does.

This document is the contract, the rejection shape it produces, the fence around
it, and the recipe for replacing the implementation behind it.

---

## 1. The contract

Two exported types and three functions. Quoted verbatim from
`internal/authn/authn.go`:

```go
type Principal struct {
	Subject string
	Scopes  []string
	Extra   map[string]string
}                                                              // :41-54

type Middleware = func(http.Handler) http.Handler              // :64

func WithPrincipal(ctx context.Context, p Principal) context.Context  // :99
func FromContext(ctx context.Context) (Principal, bool)               // :113
func MustFromContext(ctx context.Context) Principal                   // :126
```

`Middleware` is a type **alias** (`=`), not a defined type
(`internal/authn/authn.go:56-64`). Every plain `func(http.Handler) http.Handler`
in the tree assigns to it and from it with no conversion, so adopting the name
cost no call site an edit — `modules/identity/transport/auth.go:38` still spells
the handler's field as the bare func type and is the identical type by alias.
The trade was made deliberately in favour of adoption over self-documentation.

**Reads and writes are asymmetric on purpose.** `WithPrincipal` copies `Scopes`
and `Extra` on the way in, closing a data race across the trust boundary from a
producer that reuses a cached claims value; `FromContext` and `MustFromContext`
hand back the same slice and map every caller sees, because copying on every read
would tax every handler on every request to prevent a same-goroutine bug
(`internal/authn/authn.go:82-98`). **Treat a `Principal` obtained from the
context as read-only.**

`MustFromContext` panics rather than returning a zero value, with two distinct
messages so a production panic says which failure happened
(`internal/authn/authn.go:77-80`):

- `authn: no Principal in context: this handler is not behind an authn.Middleware`
- `authn: Principal in context has an empty Subject`

Use it on handlers that are only ever routed behind a `Middleware`
(`modules/identity/transport/auth.go:109-115`); use `FromContext` on routes
reachable both with and without authentication.

### 1.1 The admission rule

The rule that keeps the package small, quoted verbatim from
`internal/authn/authn.go:18`:

> A field enters `Principal` only when two or more consumer modules need it;
> everything else rides in `Extra` or a wrapper middleware.

What follows from it, and what is therefore absent on purpose until a second
consumer asks (`internal/authn/authn.go:20-29`):

| Absent | Why |
|---|---|
| `Validate(token)` | Token format is an implementation detail of the provider, not part of the contract. |
| `Authorizer`, roles, permissions | No consumer needs one today. |
| `map[string]any` for `Extra` | `map[string]string` forces custom claims to stay serializable and comparable. A consumer needing richer data writes a wrapper middleware — the intended extension path anyway. |

`Scopes` is optional and this package never interprets its contents
(`internal/authn/authn.go:47-49`). Identity leaves it nil deliberately: the
`scope` claim on the access tokens this service issues is the constant `"access"`
— a duplicate of the token class, not an authorization grant — so copying it into
an authorization-shaped field would rebuild the hazard the class check exists to
remove (`modules/identity/transport/middleware.go:57-66`). See
[token architecture](../04-security/token-architecture.md).

---

## 2. Who may import it

The contract is only replaceable while the set of things that know about it stays
small and named. Two mechanisms enforce that, and **they are not symmetrical** —
describing them as one fence would be wrong.

### 2.1 `tools/archtest` — the fence, four areas

`TestOnlyDesignatedPackagesImportAuthn` (`tools/archtest/arch_test.go:209`) walks
the real import graph and permits exactly four *areas*, each recursive over its
subpackages (`tools/archtest/arch_test.go:190-195`):

| Area | Why it is on the list |
|---|---|
| `modules/*` | The consumers. A module's transport depends on a `Principal` instead of on whichever context key identity happens to use today. |
| `internal/httpx` | Owns the canonical 401 the rejection path produces. **It does not import `authn` today** — `Unauthorized` takes only `w` and `r` — and is listed so that the day it needs to, the fence permits it rather than being widened under pressure. |
| `internal/testsupport` | `FakeAuth` returns an `authn.Middleware`, so it cannot be written without the import. |
| `internal/authn/authntest` | The conformance suite takes an `authn.Middleware` as its subject. |

Matching is segment-by-segment, not string-prefix, so `internal/httpxfoo` does
not pass as `internal/httpx` (`tools/archtest/arch_test.go:256-280`, pinned by
`TestMayImportAuthnSemantics` at `:288`). The rule also fails when *no* package
imports `authn` at all, because a fence that inspects nothing keeps passing
however it is broken (`tools/archtest/arch_test.go:241-243`).

**`cmd/**` is deliberately not on the list** (`tools/archtest/arch_test.go:186-189`).
The composition root wires identity's middleware into a module's config by
inference and never names the type — see §5.

### 2.2 The two layer rules the fence cannot express

`modules/*` is permitted recursively, because a module types its middleware at
the module root. That says *who* may import the contract, not *which layer*, so
without a second rule a `modules/*/domain` package could import `authn` with
nothing objecting. Two further tests close it, both added by T2:

- `TestDomainLayerStaysPure` denies `internal/authn` in every
  `modules/*/domain` package (`tools/archtest/arch_test.go:366`).
- `TestApplicationLayerDoesNotTouchInfrastructure` denies it in every
  `modules/*/application` package (`tools/archtest/arch_test.go:409`). The
  layer-specific reason: a use case that reads a `Principal` takes its caller
  identity from the HTTP request rather than from its own arguments, which makes
  it untestable without a request and unusable from any other entry point.
  Transport resolves the `Principal` and passes the subject down as a parameter
  (`tools/archtest/arch_test.go:397-403`).

So the fence is **four permitted areas plus two layer denials**, not one rule.

### 2.3 depguard, as it actually is

`.golangci.yml` matches on file globs and covers a different slice of the same
problem. Its `transport-kernel-access` rule (`.golangci.yml:87-97`) and its test
sibling `transport-test-kernel-access` (`.golangci.yml:124-135`) **allow**
`internal/authn` — they deny the rest of `internal/**` under `list-mode: lax`, so
the longer allow prefix wins over the shorter deny.

depguard does **not** deny `internal/authn` anywhere. `domain-purity`
(`.golangci.yml:41-54`) denies bun, chi, viper, zerolog and pgx;
`application-purity` (`.golangci.yml:58-65`) denies bun and chi. Neither names
the contract. The layer half of the fence lives in `tools/archtest` alone
(`tools/archtest/arch_test.go:345-353`) — that is the current state, not an
oversight to document as if it were symmetry.

Run both: `make lint` and `make arch`. `make arch` passes `-count=1`, which is
load-bearing — these tests read the whole repository while Go's cache keys only
on `tools/archtest`'s own files, so a plain `go test ./tools/archtest/...`
reports a cached PASS for a tree that now fails
(`tools/archtest/arch_test.go:11-22`).

---

## 3. The canonical 401

Every rejection from an `authn.Middleware` goes through one writer,
`httpx.Unauthorized` (`internal/httpx/unauthorized.go:52`):

```go
func Unauthorized(w http.ResponseWriter, r *http.Request)
```

It takes no reason and no error argument, so there is no way to leak one
(`internal/httpx/unauthorized.go:40-47`). Identity's middleware calls it on all
three of its rejection paths — no `Bearer ` prefix, a token that fails
`Validate`, and a token that verifies but carries no `sub`
(`modules/identity/transport/middleware.go:31, 42, 54`).

### 3.1 The spec

As written in §8.2 of the
[impact analysis](../../plans/authn-spi-impact-analysis.md), quoted verbatim:

```
HTTP/1.1 401 Unauthorized
Content-Type: application/problem+json
WWW-Authenticate: Bearer realm="api"

{"type":"about:blank","title":"Unauthorized","status":401,
 "detail":"authentication required","instance":"/api/v1/users/42"}
```

That block is **illustrative, not literal**. Two corrections a client author
needs, both from merged code:

1. **`request_id` is part of the body**, and is conditional. `WriteProblem` fills
   it from `chimw.GetReqID` and the field is `json:"request_id,omitempty"`
   (`internal/httpx/problem.go:27, 43`). `httpx.NewRouter` installs
   `chimw.RequestID` (`internal/httpx/router.go:47`), so **every response from
   the real server carries it**; it is absent only under a bare
   `chi.NewRouter()`, which is why the golden rig asserts it empty
   (`modules/identity/transport/unauthorized_golden_test.go:243, 314`). A client
   written against §8.2's example as a closed schema will reject the real
   response.
2. **`X-Content-Type-Options: nosniff` is also set** on the response
   (`internal/httpx/problem.go:47`), asserted at
   `modules/identity/transport/unauthorized_golden_test.go:231`.

### 3.2 The exact bytes

Recorded byte-for-byte by the goldens, identical across all five rejection cases
(`modules/identity/transport/testdata/golden/401_*.json`) and asserted by value
— not merely by mutual agreement — in `TestCanonical401Contract`
(`modules/identity/transport/unauthorized_golden_test.go:185-263`):

| Element | Value | Source |
|---|---|---|
| Status | `401` | `internal/httpx/unauthorized.go:60` |
| `Content-Type` | `application/problem+json` | `internal/httpx/problem.go:13, 46` |
| `WWW-Authenticate` | `Bearer realm="api"` | `internal/httpx/unauthorized.go:37, 55` |
| `X-Content-Type-Options` | `nosniff` | `internal/httpx/problem.go:47` |
| `type` | `about:blank` | `internal/httpx/unauthorized.go:13` |
| `title` | `Unauthorized` | `internal/httpx/unauthorized.go:17` |
| `detail` | `authentication required` | `internal/httpx/unauthorized.go:31` |
| `instance` | the request path | `internal/httpx/problem.go:42` |
| `request_id` | the request id, when one is present | `internal/httpx/problem.go:43` |

The challenge header is set *before* `WriteProblem` runs, and has to be:
`WriteProblem` commits the status line, after which added headers are silently
dropped (`internal/httpx/unauthorized.go:53-55`).

### 3.3 `detail` is invariant, and that is a security property

`detail` is the same string for every failure reason, without exception
(`internal/httpx/unauthorized.go:19-31`). Reporting "expired" versus "bad
signature" versus "revoked" turns the error body into an oracle telling an
attacker which tokens are structurally valid and which are merely stale. It is
also exactly where two implementations drift: identity and a future OIDC
middleware would never invent the same per-reason wording.

Do not add a reason parameter to `Unauthorized`, and do not vary the string.
Whatever an operator needs goes to the server log, keyed by the same
`request_id` the response carries. `TestCanonical401Contract` enforces this from
the other side too: the body is scanned against a vocabulary of leak words —
`missing`, `invalid`, `malformed`, `expired`, `signature`, `algorithm`, `alg`,
`class`, `token` — none of which may appear
(`modules/identity/transport/unauthorized_golden_test.go:208-211, 245-249`).

### 3.4 What the uniformity claim covers — and the one endpoint outside it

**The claim is scoped: it is about 401s emitted by the bearer-token
authentication middleware — the routes behind `AuthRequired`.** It is not a claim
about every 401 the API can produce.

The routes behind it, listed explicitly — not discovered — in
`cmd/api/protected_routes_test.go:31-39`, and matching the mounted route table
asserted at `cmd/api/routes_test.go:61-63, 69-72`:

```
POST   /api/v1/auth/logout
POST   /api/v1/auth/mfa/setup
POST   /api/v1/auth/mfa/setup/verify
POST   /api/v1/users
GET    /api/v1/users/{id}
PUT    /api/v1/users/{id}
DELETE /api/v1/users/{id}
```

(The guard test spells the three `{id}` routes with a concrete `1`; the pattern
form above is the one the route table registers.)

**`POST /api/v1/auth/mfa/verify` deliberately keeps an OAuth2-style
`application/json` error body.** It is registered outside the `h.authMW` group
(`modules/identity/transport/auth.go:288`, with the group starting at `:290-291`)
because it authenticates a short-lived MFA token, not an access token — it sits
*ahead* of authentication, and it is an OAuth2-conformant endpoint, so it answers
in the OAuth2 error envelope its clients expect. Its two 401 paths call
`writeOAuth2Error` (`modules/identity/transport/auth.go:198, 210`), which sets
`Content-Type: application/json` and writes `{"error":…,"error_description":…}`
(`modules/identity/transport/helpers.go:60-64`, envelope at `:55-58`).

This is an **accepted permanent divergence**, not a defect and not a cleanup
item. A client integrating against `/auth/mfa/verify` must parse the OAuth2
shape; a client integrating against anything in the table above must parse
problem+json.

---

## 4. The break, and the cut-over decision

### 4.1 What changed on the wire

Recorded before the change (`9e50896`) and after, on the same five rejection
paths against `POST /api/v1/auth/logout`:

| | Before | After |
|---|---|---|
| Status | `401` | `401` — unchanged |
| `Content-Type` | `text/plain; charset=utf-8` | `application/problem+json` |
| `WWW-Authenticate` | **absent on every path** | `Bearer realm="api"` |
| Body | `{"error":"missing token"}` / `{"error":"invalid token"}` | RFC 9457 object, **no `error` member at all** |

Three things follow, in order of client impact:

1. **The content type changed.** The old bodies were JSON handed to
   `http.Error`, which sets `text/plain; charset=utf-8`
   (`git show 9e50896:modules/identity/transport/middleware.go`, lines 16 and
   27), so a client keying on the header saw text.
2. **The body has zero field-name overlap with its predecessor.** The old shape's
   only member was `error`; the new shape has `type`, `title`, `status`,
   `detail`, `instance` and `request_id` and no `error`. A client matching on
   `body.error` breaks outright rather than degrading — which is the safer of the
   two failure modes, and worth stating plainly.
3. **A challenge header is now sent where previously none was sent on any
   rejection path, ever.** It did not vary by path; it was uniformly absent.

The old `detail`-equivalent carried two values across the five paths —
`"missing token"` for an absent `Authorization` header and `"invalid token"` for
the other four, which were already byte-identical to each other. So the change is
not the collapse of many unstable shapes into one; the substantive wins are the
content type, the RFC 9457 body, and the challenge header.

### 4.2 Clean break — decided

Per §8.3 of the [impact analysis](../../plans/authn-spi-impact-analysis.md):

| Option | Verdict | Why |
|---|---|---|
| **Clean break** | **Chosen** | Boilerplate stage, no external consumers outside our control. **This option expires the day someone adopts the template.** |
| Deprecation window | Rejected for now | The standard move *when* external clients exist; unnecessary cost here. |
| Content negotiation (shape per `Accept`) | Rejected | Doubles the conformance surface and keeps the inconsistency alive indefinitely. |

### 4.3 If you have adopted this template and have live clients

The clean break is a decision about *this* repository at *this* stage. If you
have external clients you do not control, take the deprecation window instead:

1. Keep the old member alongside the new body for a dated window — add an
   `error` field to the struct in `internal/httpx/problem.go:21-28` (or write a
   401-specific struct) carrying the legacy value, and publish the removal date
   with the release that introduces it.
2. Ship `WWW-Authenticate` and the new `Content-Type` immediately regardless.
   Neither is a body-shape break: a client that ignored a header it never
   received keeps ignoring it, and no client can have been depending on
   `text/plain` for a JSON body deliberately.
3. Keep `detail` invariant throughout the window. The window is for the *shape*,
   never for the per-reason strings — carrying those forward re-creates the
   oracle §3.3 exists to close.
4. At the removal date, delete the legacy member and re-run
   `TestCanonical401Contract` and the conformance suite. Both are value
   assertions, so the removal has to be a deliberate edit in the test, not a
   silently re-recorded golden.

Content negotiation stays rejected under either path.

---

## 5. Replacing the authentication provider

The contract exists so this is a composition-root decision. The recipe, against
the tree as it is:

**1. Write the middleware.** Any function returning an `authn.Middleware`. The
reference implementation is
`func AuthRequired(tokenIssuer domain.TokenIssuer) authn.Middleware`
(`modules/identity/transport/middleware.go:26`). It must publish a `Principal`
with a non-empty `Subject` on success (`:67-68`) and emit the canonical 401 of §3
on every rejection. Identity does that by calling `httpx.Unauthorized`
(`:31, 42, 54`), which is the easy way; the contract is the bytes, not the
writer — the conformance suite deliberately asserts the response rather than
which function produced it (`internal/authn/authntest/suite.go:18-21`).

**2. Run it through the conformance suite**, from a test file inside the module
that owns it — `internal/**` must not import `modules/**`, so the suite knows
nothing about tokens and the caller supplies the requests
(`internal/authn/authntest/suite.go:12-21`):

```go
func RunMiddlewareSuite(t *testing.T, mw authn.Middleware,
	mintValid func(t *testing.T, subject string) *http.Request,
	mintInvalid map[string]func(t *testing.T) *http.Request,
)                                          // internal/authn/authntest/suite.go:98-101
```

`mintInvalid` needs **at least two** cases — the detail-invariance check compares
the rejections to each other and is vacuous with one
(`internal/authn/authntest/suite.go:199-203`). What the suite asserts:
the handler is reached with the right `Principal.Subject` on the success path;
401, a problem+json `Content-Type` and a non-empty `WWW-Authenticate` on every
failure; an identical `detail` across all of them; no principal left downstream
after a rejection; and a handler panic propagating rather than being swallowed
(`internal/authn/authntest/suite.go:77-97`). Identity's wiring is the worked
example: `modules/identity/transport/conformance_test.go:36-54`.

**3. Swap the constructor in `BuildApp`.** `cmd/api/container.go` is where a
*consumer* module's implementation is chosen. Today `newUserModule` builds one
from its own token verifier and hands it over:

```go
authMW := identitytransport.AuthRequired(jwtSvc)                 // container.go:103
return user.New(deps, user.Config{AuthMiddleware: authMW})       // container.go:105
```

Replacing the provider is replacing line 103. No consumer changes: `Middleware`
is an alias, so any `func(http.Handler) http.Handler` assigns to
`user.Config.AuthMiddleware` unconverted.

Two things this step does not cover, and both are real:

- **Identity builds its own middleware for its own protected routes**
  (`modules/identity/module.go:138`), derived from the same `jwtSvc` it mints
  with. A replacement that is meant to protect `/auth/logout` and the MFA setup
  routes too has to be threaded in there as well, or those routes keep using
  identity's.
- **`cmd/**` is not a permitted importer of `internal/authn`**
  (`tools/archtest/arch_test.go:186-189`). The composition root gets away with it
  today only because it never names the type. If your replacement forces
  `cmd/api` to write `authn.Middleware` in a declaration, `make arch` fails —
  widening `authnAreas` is then a decision to take deliberately rather than a
  diff to wave through.

**4. In tests, do not mint tokens.** `internal/testsupport` supplies two fakes
that satisfy the contract without any credential:

```go
func FakeAuth(subject string) authn.Middleware              // internal/testsupport/fakeauth.go:62
func FakeAuthPrincipal(p authn.Principal) authn.Middleware  // internal/testsupport/fakeauth.go:91
```

`FakeAuth("")` panics at construction, in the caller's own frame, rather than
letting an unauthenticated `Principal` surface later as a panic inside somebody
else's `MustFromContext` (`internal/testsupport/fakeauth.go:26-28, 62-65`).
`FakeAuthPrincipal` is the literal injector with no guard, for tests that need
scopes or that are reproducing what a misbehaving middleware does
(`internal/testsupport/fakeauth.go:78-81`). A transport test may import
`internal/testsupport`; `transport-test-kernel-access` allows exactly it,
`internal/authn` and `internal/httpx`, and denies the rest of `internal/**`
(`.golangci.yml:124-135`).

---

## 6. Consumer modules take `authn.Middleware`

A module that protects routes does **not** import the module that authenticates.
It declares the dependency in its own `Config` and the composition root satisfies
it — the R2 pattern, named as such in `tools/archtest/arch_test.go:110-113`.

`modules/user` is the template (`modules/user/module.go`):

```go
import "github.com/sujanto-gaws/kopiochi/internal/authn"       // :17

type Config struct {
	AuthMiddleware authn.Middleware                            // :53
}

func (c Config) Validate() error {                             // :58
	if c.AuthMiddleware == nil {
		return errors.New("user: auth middleware is required")
	}
	return nil
}

func New(deps module.Deps, cfg Config) (*module.Module, error) // :75
```

Four properties worth copying:

1. **It arrives as a middleware, not as a token verifier.** The module never
   learns how authentication is implemented (`modules/user/module.go:36-41`).
2. **It is required, not optional.** `Validate` rejects nil and `New` returns the
   error, so the module fails to construct rather than constructing and quietly
   serving records to anonymous callers (`modules/user/module.go:58-63, 76-78`).
3. **The handler takes it too and applies it itself.**
   `func NewUserHandler(svc UserService, authMW authn.Middleware) *UserHandler`
   (`modules/user/transport/user.go:41`), used in the route group at
   `modules/user/transport/user.go:199`. Routing and protection are declared
   together, so a route added outside the group is visible in the diff.
4. **Handlers read the caller through the contract**, e.g.
   `authn.FromContext(r.Context())` in `modules/user/transport/user_test.go:198`
   and `authn.MustFromContext(r.Context()).Subject` in
   `modules/identity/transport/auth.go:115`.

Identity's own `NewAuthHandler` still spells its parameter as the bare
`func(http.Handler) http.Handler` (`modules/identity/transport/auth.go:47`).
That is the identical type — `Middleware` is an alias — and is exactly the
"adoption costs nothing" property from §1; new code should use the name.

---

## 7. What the build checks

| Check | Where | What it would catch |
|---|---|---|
| Fence: four permitted areas | `tools/archtest/arch_test.go:209` | Any new importer of `internal/authn` outside `modules/*`, `internal/httpx`, `internal/testsupport`, `internal/authn/authntest`. |
| Fence semantics | `tools/archtest/arch_test.go:288` | A matcher that quietly permits everything, or that lets `internal/httpxfoo` through on a string prefix. |
| Domain / application layer denial | `tools/archtest/arch_test.go:354, 404` | A `Principal` read from inside a use case or an entity. |
| transport kernel access | `.golangci.yml:87-97, 124-135` | A transport package (or its tests) reaching past `authn`/`httpx` into the rest of `internal/**`. |
| Identity's exact 401, by value | `modules/identity/transport/unauthorized_golden_test.go:185` | A reason word leaking into the body; a changed media type, challenge or member. |
| Identity's exact 401, byte-for-byte | `modules/identity/transport/testdata/golden/401_*.json` | Any drift at all, with a readable diff. |
| The contract, for *any* middleware | `internal/authn/authntest/suite.go:98` | A replacement that reintroduces per-reason bodies, drops the challenge, or leaves a principal behind after rejecting. |
| Routes are actually protected | `cmd/api/protected_routes_test.go:31-39, 62` | A route that forgot its middleware — it answers 200 or 500 there instead of 401. |
| The mounted route table | `cmd/api/routes_test.go:58-72` | A route serving at the wrong prefix, or disappearing. |

Run them with `make arch` (which passes `-count=1`), `make lint`, and
`go test ./...`.

---

## Related documents

- [Dependency rules](../01-modularity/dependency-rules.md) — R1's transport row
  is what admits `internal/authn` into a module's transport layer.
- [Module layout](../01-modularity/module-layout.md) — where a module's
  `Config` and constructor live.
- [Dependency injection](../02-composition/dependency-injection.md) — why
  `BuildApp` is the only place that chooses an implementation.
- [Token architecture](../04-security/token-architecture.md) — token classes,
  RS256, and why `Scopes` is left nil.
- [Testing strategy](../06-quality/testing-strategy.md) — the fail-open and
  escalation guards this document's §7 table refers to.
- [Impact analysis](../../plans/authn-spi-impact-analysis.md) — the
  before/after per package that produced this design. §8.2's example body is
  illustrative; §3.1 above corrects it against merged code.
- [`BOILERPLATE.md`](../../../BOILERPLATE.md) — the add-a-module recipe.
- [`CHANGELOG.md`](../../../CHANGELOG.md) — the client-facing statement of the
  401 break.
