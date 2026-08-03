//go:build swagger

// This file is compiled only with `-tags swagger`. Its sibling
// swagger_disabled.go provides the same function for every other build.
//
// The split exists because importing the swagger UI links swaggo/swag, the
// http-swagger handler, swaggo/files (which embeds the whole Swagger UI
// distribution) and four go-openapi packages into the *server*. None of it is
// reachable at runtime unless someone browses /swagger, and all of it ships in
// every production image. See docs/architectures/06-quality/repository-hygiene.md,
// problem 5.
package httpx

import (
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/sujanto-gaws/kopiochi/docs" // generated swagger spec
)

// swaggerEnabled reports whether this binary serves the API browser.
const swaggerEnabled = true

// mountSwagger registers the Swagger UI.
//
// The generated package it imports is not committed — run `make swagger-docs`
// first, or this build will not compile. That is deliberate: a stale committed
// spec is worse than no spec, because it is confidently wrong.
func mountSwagger(r *chi.Mux) {
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))
}
