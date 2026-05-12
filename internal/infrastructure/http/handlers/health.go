package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sujanto-gaws/kopiochi/internal/version"
)

// HealthCheck handles GET /health
// @Summary Health check endpoint
// @Description Check if the API is running and healthy
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{} "Health check response"
// @Router /health [get]
func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status":  "ok",
			"version": version.Version,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
