package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/sujanto-gaws/kopiochi/internal/version"
)

// pingTimeout bounds how long /readyz waits on the database before reporting
// not-ready. It is kept short and independent of any client-supplied timeout
// so a slow database cannot make the readiness probe itself expensive.
const pingTimeout = 2 * time.Second

// healthzHandler answers liveness: the process is up and serving requests.
// It never touches a dependency — that is what distinguishes it from
// /readyz, and why it can never itself report a false negative.
func healthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeHealth(w, http.StatusOK, "ok")
	}
}

// readyzHandler answers readiness: the process is up AND its dependencies
// are reachable. It pings the DB pool with a short timeout — never a query —
// so orchestrators can cheaply and quickly stop routing traffic when the
// database is unreachable.
//
// A nil pinger is treated as not-ready rather than ok: readiness with no way
// to check a dependency isn't readiness, it's a wiring bug, and this endpoint
// fails closed rather than rubber-stamping it.
//
// draining, when non-nil, reports that shutdown has begun. It is checked
// before the ping and before anything else: once the server starts draining,
// the process must stop advertising itself as ready immediately, so the load
// balancer removes it while in-flight requests finish rather than after. A
// nil draining func means "nothing to report" and is the right value for
// tests and for any caller with no server to ask.
func readyzHandler(pinger Pinger, draining func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if draining != nil && draining() {
			writeHealth(w, http.StatusServiceUnavailable, "draining")
			return
		}

		if pinger == nil {
			writeHealth(w, http.StatusServiceUnavailable, "unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()

		if err := pinger.Ping(ctx); err != nil {
			writeHealth(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeHealth(w, http.StatusOK, "ok")
	}
}

func writeHealth(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Status and headers are already committed; a failed encode leaves a
	// truncated body and nothing actionable. Probes read the status code.
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  state,
		"version": version.Version,
	})
}
