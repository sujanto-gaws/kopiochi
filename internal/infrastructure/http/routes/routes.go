package routes

import (
	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
)

// Setup configures all API routes
func Setup(r *chi.Mux, userHandler *handlers.UserHandler) {
	// Health check
	r.Get("/health", handlers.HealthCheck())

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// User routes
		r.Post("/users", userHandler.CreateUser())
		r.Get("/users/{id}", userHandler.GetUser())
		r.Put("/users/{id}", userHandler.UpdateUser())
		r.Delete("/users/{id}", userHandler.DeleteUser())
	})
}
