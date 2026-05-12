package handlers

import (
	"encoding/json"
	"net/http"
)

// errorResponse creates a standardized error JSON response
func errorResponse(message string) map[string]string {
	return map[string]string{"error": message}
}

// writeJSON is a helper to write JSON responses
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

type OAuth2Error struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func writeOAuth2Error(w http.ResponseWriter, errCode, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(OAuth2Error{Error: errCode, ErrorDescription: description})
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
	json.NewEncoder(w).Encode(ProblemDetails{
		Type:   typ,
		Title:  title,
		Status: status,
		Detail: detail,
	})
}