package httpx

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// ProblemContentType is the media type RFC 7807 defines for error responses.
const ProblemContentType = "application/problem+json"

// Problem is an RFC 7807 problem detail.
//
// RequestID is an extension member, not part of RFC 7807 itself. It is here so
// that a user can quote the identifier from a failed response and have it
// match a log line exactly — without it, correlating a report with the logs
// means guessing from timestamps.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteProblem writes an RFC 7807 response, filling Instance and RequestID
// from the request so callers do not have to remember to.
//
// detail is shown to the client, so it must never carry internal state: no
// panic values, no stack traces, no driver errors. Anything an operator needs
// belongs in the log line, keyed by the same request_id.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, typ, title, detail string) {
	p := Problem{
		Type:      typ,
		Title:     title,
		Status:    status,
		Detail:    detail,
		Instance:  r.URL.Path,
		RequestID: chimw.GetReqID(r.Context()),
	}

	w.Header().Set("Content-Type", ProblemContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	// Status and headers are committed; a failed encode leaves a truncated
	// body and nothing actionable beyond not pretending we checked.
	_ = json.NewEncoder(w).Encode(p)
}

// NotFound returns the handler chi calls when no route matches.
//
// chi's default writes a bare "404 page not found" in text/plain, so a client
// that content-negotiated JSON gets a body it cannot parse for the two
// statuses it is most likely to hit while integrating. This makes every error
// this service emits the same shape.
func NotFound() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, r, http.StatusNotFound,
			"not_found", "Not Found",
			"No route matches this path.")
	}
}

// MethodNotAllowed returns the handler chi calls when a route exists at the
// path but not for this method.
//
// It takes the router because of a trap in chi's design: Mux.
// MethodNotAllowedHandler returns a registered custom handler *instead of*
// chi's default, and only that default sets the Allow header. So installing a
// custom 405 handler silently drops Allow — which RFC 9110 requires a 405 to
// carry, and which is the only thing that tells a client what to try next.
// chi keeps the allowed set in the unexported Context.methodsAllowed, so the
// only way to recover it is to ask the routing tree directly.
func MethodNotAllowed(routes chi.Routes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if allowed := allowedMethods(routes, r.URL.Path); len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
		}
		WriteProblem(w, r, http.StatusMethodNotAllowed,
			"method_not_allowed", "Method Not Allowed",
			"This path does not accept "+r.Method+".")
	}
}

// probeMethods is the set allowedMethods tests a path against. chi routes on
// this same fixed set (it has no wildcard method), so probing it is exhaustive
// rather than a sample.
var probeMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodConnect,
	http.MethodOptions,
	http.MethodTrace,
}

// allowedMethods asks the routing tree which methods are registered for path.
//
// Each probe needs its own RouteContext: Match populates it, and reusing one
// would leak the previous probe's URL params into the next match.
func allowedMethods(routes chi.Routes, path string) []string {
	if routes == nil {
		return nil
	}

	allowed := make([]string, 0, len(probeMethods))
	for _, m := range probeMethods {
		if routes.Match(chi.NewRouteContext(), m, path) {
			allowed = append(allowed, m)
		}
	}
	return allowed
}
