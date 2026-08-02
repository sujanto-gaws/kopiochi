// Package module defines the contract between a business capability
// ("module") and the composition root that wires it up.
//
// Deps is deliberately a struct rather than a map (e.g. map[string]any):
// every collaborator a module needs is a named, typed field, so a missing
// dependency is caught by the compiler at wiring time instead of surfacing
// as a nil-map/nil-interface panic at runtime.
package module

import (
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/uptrace/bun"
)

// Deps carries live collaborators from the composition root.
// It is a struct — not a map — so a missing dependency is a compile error.
type Deps struct {
	DB     bun.IDB
	Logger zerolog.Logger
}

// Module is what a business capability exposes to the host.
type Module struct {
	Name       string
	Routes     func(r chi.Router) // mounts onto the version group
	Migrations fs.FS              // embedded, owned by the module
	Close      func() error       // optional cleanup
}
