//go:build !swagger

package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// swaggerEnabled reports whether this binary serves the API browser.
const swaggerEnabled = false

// mountSwagger answers /swagger/* with a problem document explaining that this
// binary was not built with the UI.
//
// Registering something is better than registering nothing: without it the
// path falls through to the 404 handler, which says "no route matches" — and
// an operator who knows the endpoint used to exist reads that as a routing
// bug rather than a build-time choice.
func mountSwagger(r *chi.Mux) {
	r.Get("/swagger/*", func(w http.ResponseWriter, req *http.Request) {
		WriteProblem(w, req, http.StatusNotFound,
			"swagger_not_built",
			"API Browser Not Available",
			"This binary was built without the swagger UI. Rebuild with -tags swagger, "+
				"after running `make swagger-docs` to generate the spec.")
	})
}
