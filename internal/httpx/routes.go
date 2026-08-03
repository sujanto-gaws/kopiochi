// Package httpx assembles the top-level HTTP route tree: unversioned
// operational endpoints (health, swagger) plus every business module's
// routes mounted under /api/v1.
//
// See docs/architectures/02-composition/routing-and-versioning.md for the bug
// this package fixes: a chi.Router shadowing mistake that mounted every
// module's routes at the root instead of under /api/v1.
package httpx

import (
	"context"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/sujanto-gaws/kopiochi/docs" // generated swagger docs
	"github.com/sujanto-gaws/kopiochi/internal/module"
)

// Pinger checks connectivity to a live dependency without running a query.
// *pgxpool.Pool satisfies this via its Ping method; it is what /readyz uses
// to check the database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps carries the process-wide collaborators Mount itself needs — as
// opposed to module.Deps, which each business module needs.
//
// App (cmd/api) is deliberately not one of these: it lives in package main,
// which httpx cannot import without an import cycle. Mount instead takes the
// plain []*module.Module it actually needs.
type Deps struct {
	Pinger Pinger // used by /readyz; nil is treated as "not ready" (fail closed)
	// Draining, when set, makes /readyz report not-ready as soon as
	// shutdown begins — before the drain, not after it — so an orchestrator
	// stops sending new requests while in-flight ones finish. *Server
	// supplies this via its Draining method. nil means "no shutdown signal
	// to report".
	Draining func() bool
}

// Mount wires the full route tree onto r: unversioned health/readiness/
// swagger endpoints, and every module's routes under /api/v1.
//
// There is deliberately only one router in scope for the /api/v1 group — v1,
// passed directly into each module's Routes — so the shadowing bug where
// routes silently mounted on the wrong router cannot recur: there is no
// second router left in scope to accidentally register against.
func Mount(r *chi.Mux, modules []*module.Module, deps Deps) {
	// Operational endpoints: unversioned, unauthenticated.
	r.Get("/healthz", healthzHandler())
	r.Get("/readyz", readyzHandler(deps.Pinger, deps.Draining))
	r.Get("/health", healthzHandler()) // deprecated alias for /healthz; drop once clients migrate

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
