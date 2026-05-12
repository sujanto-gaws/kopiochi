package handlers

import "github.com/go-chi/chi/v5"

// RouterGroup holds the public and protected chi sub-routers for /api/v1.
// Handlers receive this in RegisterRoutes and choose which router to mount
// each of their endpoints on.
type RouterGroup struct {
	Public    chi.Router // no authentication required
	Protected chi.Router // auth middleware already applied
}

// RouteRegistrar is implemented by any handler group or extension that registers
// its own HTTP routes under /api/v1. Pass registrars to routes.Setup.
type RouteRegistrar interface {
	RegisterRoutes(g RouterGroup)
}
