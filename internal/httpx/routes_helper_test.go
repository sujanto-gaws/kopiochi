package httpx

import "github.com/go-chi/chi/v5"

// newTestMux returns a bare router for tests that exercise Mount directly.
func newTestMux() *chi.Mux { return chi.NewRouter() }
