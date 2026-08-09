// Package user is the profile-user business module: CRUD over the app_user
// profile record, distinct from the identity module's authentication user.
//
// Before Phase 3.6b this stack was spread across internal/domain/user,
// internal/application/user, internal/infrastructure/persistence/{models,
// repository} and internal/infrastructure/http/handlers, and was assembled
// inline by a newUserModule helper in the composition root. It was live the
// whole time — which is why 3.6 deleted the dead frameworks but this code was
// *moved*, not deleted. Behaviour is unchanged; only its address and its
// constructor are new.
package user

import (
	"errors"
	"fmt"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/module"
	application "github.com/sujanto-gaws/kopiochi/modules/user/application"
	"github.com/sujanto-gaws/kopiochi/modules/user/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/user/transport"
)

// Config carries this module's settings.
//
// The module needs no configuration of its own today: its only requirement
// is an authentication middleware, which the composition root supplies as a
// dependency because it is owned by the identity module. Config exists so
// that requirement has somewhere to grow, and so this module's constructor
// has the same shape as every other one.
type Config struct {
	// AuthMiddleware protects every route this module exposes. It is a
	// required dependency, not an option: New refuses to build a module
	// that would serve user records unauthenticated.
	//
	// It arrives here as a middleware rather than as a token verifier, so
	// this module never learns how authentication is implemented —
	// module.Module deliberately exposes nothing but
	// Name/Routes/Migrations/Close, and cross-module wiring goes through
	// the composition root, not a back-channel into identity's private
	// dependency graph.
	//
	// The type is authn.Middleware, which is a type *alias* for
	// func(http.Handler) http.Handler and therefore the identical type the
	// field carried before: every existing call site assigns to it with no
	// conversion, and cmd/api needed no edit. What the name buys is that
	// the requirement is now stated in the shared contract package instead
	// of by convention, so the edge is one depguard and tools/archtest can
	// see. A bare func type said "some middleware"; this says "the
	// authentication middleware defined by internal/authn", and a handler
	// downstream can rely on an authn.Principal being in the context
	// because of it.
	AuthMiddleware authn.Middleware
}

// Validate rejects configuration that would produce an unprotected route
// surface.
func (c Config) Validate() error {
	if c.AuthMiddleware == nil {
		return errors.New("user: auth middleware is required")
	}
	return nil
}

// New builds the user module's dependency graph and returns a module.Module
// ready to be mounted by the composition root.
//
// It fails closed: without an authentication middleware the module does not
// construct at all, rather than constructing and quietly serving every user
// record to anonymous callers.
//
// Migrations is left unset for the same reason as identity's: this module's
// SQL still lives under the top-level migrations/ directory and the
// goose-based runner has no embed.FS support yet.
func New(deps module.Deps, cfg Config) (*module.Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("user: invalid config: %w", err)
	}

	repo := repository.NewUserRepository(deps.DB)
	svc := application.NewService(repo)
	handler := transport.NewUserHandler(svc, cfg.AuthMiddleware)

	return &module.Module{
		Name:   "user",
		Routes: handler.Routes,
	}, nil
}
