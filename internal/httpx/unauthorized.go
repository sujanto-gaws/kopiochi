package httpx

import "net/http"

// Constants for the canonical 401. They are values, not knobs: every rejection
// in the service emits this exact set, so the only thing that varies between
// two 401s is the instance path (and the request id, which correlates the
// response with its log line).
const (
	// unauthorizedType is RFC 9457's default type URI. The failure needs no
	// type-specific semantics beyond the status itself, and a bespoke URI
	// would be one more thing a replacement auth module could get wrong.
	unauthorizedType = "about:blank"

	// unauthorizedTitle is the status phrase, as RFC 9457 prescribes for
	// about:blank.
	unauthorizedTitle = "Unauthorized"

	// unauthorizedDetail is the detail sent for EVERY authentication
	// failure, without exception.
	//
	// This is a security property, not a style choice. Reporting "expired"
	// versus "bad signature" versus "revoked" tells an attacker which tokens
	// are structurally valid and which merely stale — it turns the error body
	// into an oracle. It is also where implementations drift: identity and a
	// future OIDC middleware would never invent the same per-reason wording.
	//
	// Whatever an operator needs to debug a rejection goes to the server log,
	// keyed by the same request id this response carries. Do not add a reason
	// parameter to Unauthorized, and do not vary this string.
	unauthorizedDetail = "authentication required"

	// unauthorizedChallenge is the WWW-Authenticate value RFC 9110 requires a
	// 401 to carry. The realm is fixed: the whole API is one protection
	// space, so a per-route realm would only tell clients apart from each
	// other, not from anything they could act on.
	unauthorizedChallenge = `Bearer realm="api"`
)

// Unauthorized writes the canonical 401: problem+json body, a Bearer
// challenge, and no hint about *why* the credential failed.
//
// It is the single rejection path for authentication. Middleware calls it for
// a missing header, a malformed token, an expired token, a wrong algorithm, a
// wrong token class — all of them — and the client sees one behavior:
// re-authenticate. It deliberately takes no reason or error argument, so there
// is no way to leak one.
//
// It is a writer rather than a handler factory (unlike NotFound and
// MethodNotAllowed below it) because its callers are middleware that have
// already decided to reject, mid-chain, and need to write rather than mount.
func Unauthorized(w http.ResponseWriter, r *http.Request) {
	// Must precede WriteProblem: that commits the status line, after which
	// added headers are silently dropped.
	w.Header().Set("WWW-Authenticate", unauthorizedChallenge)

	// WriteProblem sets Content-Type, nosniff, the status code, and fills
	// instance and request_id from r. Nothing here writes a status itself —
	// a second WriteHeader would be a runtime warning and a bug.
	WriteProblem(w, r, http.StatusUnauthorized,
		unauthorizedType, unauthorizedTitle, unauthorizedDetail)
}
