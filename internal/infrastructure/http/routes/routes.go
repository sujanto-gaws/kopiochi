package routes

import (
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/sujanto-gaws/kopiochi/docs" // Import generated swagger docs
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
)

// Setup configures all API routes
func Setup(r *chi.Mux, userHandler *handlers.UserHandler) {
	// Health check
	r.Get("/health", handlers.HealthCheck())

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:8080/swagger/doc.json"),
	))

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// User routes
		r.Post("/users", userHandler.CreateUser())
		r.Get("/users/{id}", userHandler.GetUser())
		r.Put("/users/{id}", userHandler.UpdateUser())
		r.Delete("/users/{id}", userHandler.DeleteUser())
	})
}
