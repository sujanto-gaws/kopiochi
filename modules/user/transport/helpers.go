// Package transport is the user module's HTTP layer: its handlers and its
// route table, mounted behind the auth middleware supplied by the composition
// root.
package transport

import (
	"encoding/json"
	"net/http"
)

// This file once carried the refresh-token cookie helpers
// (refreshTokenCookie, setRefreshCookie, clearRefreshCookie) and the OAuth2 /
// RFC 7807 error writers (OAuth2Error, writeOAuth2Error, ProblemDetails,
// writeProblemDetails), back when it was the shared
// internal/infrastructure/http/handlers package serving both auth and user
// routes. Auth moved to modules/identity in Phase 1 and took live copies of
// all of them with it (modules/identity/transport/helpers.go); the copies
// here have had no caller since, which `unused` reported the moment linting
// was switched on in 3.2.
//
// They are deleted rather than kept "in case the user module needs them":
// identity owns tokens and cookies, and a second, drifting copy of the
// cookie-security attributes is exactly the duplication Phase 3 exists to
// remove. What remains is what the user handlers actually call.
//
// errorResponse went the same way, later and for the same reason (E32). Deleting
// the RFC 7807 writer from this file without replacing it is what left the module
// answering its failures with {"error": "..."} while the rest of the tree spoke
// problem+json — the handlers reached for the one writer still in the file. The
// replacement is internal/httpx.WriteProblem, which is the shared writer this
// file should have pointed at in the first place: unlike the copy deleted above,
// it cannot drift, and it fills instance and request_id from the request.
//
// writeJSON stays. It carries the 200s, where the module's own DTO is the body
// and there is nothing canonical to defer to.

// writeJSON is a helper to write JSON responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
