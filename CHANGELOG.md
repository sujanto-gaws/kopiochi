# Changelog

Notable, client-visible changes to this repository. Entries are grouped under
`Unreleased` until a release is cut; the repository publishes no version tags
today, so `Unreleased` is where everything lives.

Each entry states what a client sees on the wire, not what moved internally.
Internal refactors that change no request or response belong in the commit log,
not here.

## Unreleased

### Removed — breaking (HTTP clients)

**`POST /api/v1/users` and `GET`, `PUT`, `DELETE /api/v1/users/{id}` are gone.
They are replaced by `POST /api/v1/users/me` and `GET /api/v1/users/me`.**

On the wire:

| Before | Now |
|---|---|
| `POST /api/v1/users` with `{"name":…,"email":…}` → `201` | `POST /api/v1/users/me`, **no body** → `200` |
| `GET /api/v1/users/{id}` → `200` with `{id,name,email,…}` | `GET /api/v1/users/me` → `200` with `{id,created_at,updated_at}` |
| `PUT /api/v1/users/{id}` → `200` | **removed** — there is no writable field |
| `DELETE /api/v1/users/{id}` → `204` | **removed** — account deletion belongs to the identity |

A request to any removed route now fails to route: it does not reach a handler.

**Why, stated plainly: any valid access token could read, overwrite or delete
any other user's record.** The profile's id was an integer unrelated to the
caller's identity, so a handler had no value to compare a caller against — the
defect was a missing *column*, not a missing check. The profile is now keyed by
the authenticated identity, so the id a handler acts on is the one in the
caller's own token and there is no parameter to supply another. The status codes
also distinguished "exists but is not yours" from "does not exist", which made
the id space enumerable.

**The response no longer carries `name` or `email`.** They live on the identity,
which is their single source of truth; the profile's copies had one consumer,
which was this API echoing back what had just been posted.

**`POST /api/v1/users/me` is idempotent** — calling it twice returns the same
profile rather than an error, so a client that retries after a lost response
does not have to distinguish the cases.

**Migration for clients:** replace any call that names a user id with the `/me`
form, drop the request bodies, and read `name`/`email` from wherever your
identity data comes from rather than from this endpoint.

### Changed — breaking (HTTP clients)

**401 responses from the bearer-token authentication middleware are now uniform
`application/problem+json`. Clients must key off `status`, never `detail`.**

This applies to the routes behind that middleware — `POST /api/v1/auth/logout`,
`POST /api/v1/auth/mfa/setup`, `POST /api/v1/auth/mfa/setup/verify`, and every
`/api/v1/users` route (`cmd/api/protected_routes_test.go`) — which since the
removal above means `POST` and `GET /api/v1/users/me`. It is not a
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
