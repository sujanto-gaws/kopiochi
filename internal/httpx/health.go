package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/sujanto-gaws/kopiochi/internal/version"
)

// healthzHandler answers liveness: the process is up and serving requests.
// It never touches a dependency — that is what distinguishes it from
// /readyz.
func healthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	}
}
