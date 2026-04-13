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
