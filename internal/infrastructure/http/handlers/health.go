package handlers

import (
	"encoding/json"
	"net/http"
)

func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status":  "ok",
			"version": "1.0.0",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
