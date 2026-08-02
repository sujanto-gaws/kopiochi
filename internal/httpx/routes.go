// Package httpx assembles the top-level HTTP route tree: unversioned
// operational endpoints (health, swagger) plus every business module's
// routes mounted under /api/v1.
//
// See docs/architectures/02-composition/routing-and-versioning.md for the bug
// this package fixes: a chi.Router shadowing mistake that mounted every
// module's routes at the root instead of under /api/v1.
package httpx

import (
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/sujanto-gaws/kopiochi/docs" // generated swagger docs
	"github.com/sujanto-gaws/kopiochi/internal/module"
)

// Deps carries the process-wide collaborators Mount itself needs — as
// opposed to module.Deps, which each business module needs.
//
// App (cmd/api) is deliberately not one of these: it lives in package main,
// which httpx cannot import without an import cycle. Mount instead takes the
// plain []*module.Module it actually needs.
type Deps struct{}

// Mount wires the full route tree onto r: unversioned health/swagger
// endpoints, and every module's routes under /api/v1.
//
// There is deliberately only one router in scope for the /api/v1 group — v1,
// passed directly into each module's Routes — so the shadowing bug where
// routes silently mounted on the wrong router cannot recur: there is no
// second router left in scope to accidentally register against.
func Mount(r *chi.Mux, modules []*module.Module, deps Deps) {
	// Operational endpoints: unversioned, unauthenticated.
	r.Get("/healthz", healthzHandler())
	r.Get("/readyz", healthzHandler()) // TODO(1.7): check the DB pool

	// Swagger documentation.
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Versioned API. v1 is passed directly into each module — there is no
	// shadow copy of this router lying around for a module to mount onto by
	// mistake.
	r.Route("/api/v1", func(v1 chi.Router) {
		for _, m := range modules {
			m.Routes(v1)
		}
	})
}
