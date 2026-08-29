# Changelog

Notable, client-visible changes to this repository. Entries are grouped under
`Unreleased` until a release is cut; the repository publishes no version tags
today, so `Unreleased` is where everything lives.

Each entry states what a client sees on the wire, not what moved internally.
Internal refactors that change no request or response belong in the commit log,
not here.

## Unreleased

### Changed — breaking (HTTP clients)

**401 responses from the bearer-token authentication middleware are now uniform
`application/problem+json`. Clients must key off `status`, never `detail`.**

This applies to the routes behind that middleware — `POST /api/v1/auth/logout`,
`POST /api/v1/auth/mfa/setup`, `POST /api/v1/auth/mfa/setup/verify`, and every
`/api/v1/users` route (`cmd/api/protected_routes_test.go`). It is not a
statement about every 401 the API can emit; see the carve-out below.

Three transport-level changes, in order of client impact:

1. **`Content-Type` changed from `text/plain; charset=utf-8` to
   `application/problem+json`.** The previous body was JSON served under the
   `http.Error` default content type.
2. **The body has no `error` member at all.** It was `{"error":"missing token"}`
   or `{"error":"invalid token"}`; it is now an RFC 9457 object —
   `type`, `title`, `status`, `detail`, `instance`, and `request_id` when a
   request id is present. There is zero field-name overlap, so a client matching
   on `body.error` breaks outright rather than degrading silently.
3. **`WWW-Authenticate: Bearer realm="api"` is now sent.** Previously no
   `WWW-Authenticate` header was sent on any rejection path, ever.

`detail` is the constant string `authentication required` for every failure
reason — expired, malformed, wrong algorithm, wrong token class, missing header.
That invariance is deliberate and permanent: a per-reason body tells an attacker
which tokens are structurally valid and which are merely stale. Do not branch on
it. Operator-facing detail goes to the server log, keyed by the same
`request_id` the response carries.

`request_id` is present on responses from the running server and omitted when no
request id is in scope, so treat it as optional when parsing.

Status codes, the route table, tokens and cookies are unchanged.

**Carve-out.** `POST /api/v1/auth/mfa/verify` deliberately keeps its OAuth2-style
`application/json` error body (`{"error":…,"error_description":…}`). It sits
ahead of authentication — it consumes the short-lived MFA token, not an access
token — and is an OAuth2-conformant endpoint, so it answers in the envelope its
clients expect. This is an accepted permanent divergence, not a pending cleanup.

**Cut-over.** A clean break, chosen because this repository is at the boilerplate
stage with no external consumers outside our control. That reasoning expires the
day someone adopts the template: adopters with live clients should take the
deprecation-window path instead, which is written out in
[`docs/architectures/08-authn/README.md`](docs/architectures/08-authn/README.md#43-if-you-have-adopted-this-template-and-have-live-clients).

### Added

- `internal/authn` — the authentication contract (`Principal`, `Middleware`, and
  the context plumbing) that consumer modules depend on instead of on the module
  that authenticates. Documented in
  [`docs/architectures/08-authn/README.md`](docs/architectures/08-authn/README.md).
- `internal/authn/authntest` — the conformance suite any replacement
  authentication middleware must pass.
- `internal/testsupport.FakeAuth` / `FakeAuthPrincipal` — an `authn.Middleware`
  for handler tests, so a protected-route test needs no minted token.
