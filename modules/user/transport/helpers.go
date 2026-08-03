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

// errorResponse creates a standardized error JSON response.
func errorResponse(message string) map[string]string {
	return map[string]string{"error": message}
}

// writeJSON is a helper to write JSON responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
