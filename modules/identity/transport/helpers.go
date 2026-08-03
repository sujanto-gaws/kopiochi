package transport

import (
	"encoding/json"
	"net/http"
	"time"
)

// This file duplicates the subset of internal/infrastructure/http/handlers's
// helpers.go that AuthHandler needs (writeJSON, the OAuth2/ProblemDetails
// error envelopes, and the refresh-token cookie helpers). The original
// helpers.go stays put because UserHandler (which is not part of this move)
// still depends on it. Deduplicating both into a shared internal/httpx
// package is left for a later phase.

const refreshTokenCookie = "refresh_token"

func setRefreshCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// writeJSON is a helper to write JSON responses
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		// The status line and headers are already committed, so a failed
		// encode cannot be turned into an error response. The client sees a
		// truncated body and a mismatched Content-Length; there is nothing
		// useful to do here beyond not pretending we checked.
		_ = json.NewEncoder(w).Encode(v)
	}
}

type OAuth2Error struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func writeOAuth2Error(w http.ResponseWriter, errCode, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(OAuth2Error{Error: errCode, ErrorDescription: description})
}

type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

func writeProblemDetails(w http.ResponseWriter, typ, title string, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ProblemDetails{
		Type:   typ,
		Title:  title,
		Status: status,
		Detail: detail,
	})
}
