package routes

import (
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/sujanto-gaws/kopiochi/docs" // Import generated swagger docs
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"

	"github.com/go-chi/chi/v5"
)

// Setup mounts global routes (health, swagger) and delegates /api/v1 sub-routes to
// each RouteRegistrar. The caller is responsible for building the RouterGroup —
// including which middlewares protect which router — before calling Setup.
func Setup(r *chi.Mux, g handlers.RouterGroup, registrars ...handlers.RouteRegistrar) {
	// Health check
	r.Get("/health", handlers.HealthCheck())

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		for _, reg := range registrars {
			reg.RegisterRoutes(g)
		}
	})
}
