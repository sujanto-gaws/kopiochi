// Package container wires the full dependency graph for the API server.
// To add a new handler:
//  1. Instantiate its repository, service, and handler here.
//  2. Return it from Registrars().
package container

import (
	"fmt"

	"github.com/uptrace/bun"

	appUser "github.com/sujanto-gaws/kopiochi/internal/application/user"
	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/repository"
	appAuth "github.com/sujanto-gaws/kopiochi/modules/identity/application"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/hasher"
	infraMFA "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/mfa"
	authRepo "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
	identitytransport "github.com/sujanto-gaws/kopiochi/modules/identity/transport"
)

// Container holds all wired handlers and exposes them as RouteRegistrars.
type Container struct {
	registrars []handlers.RouteRegistrar
}

// authRegistrar adapts the identity module's AuthHandler — whose Routes
// method self-mounts on a chi.Router per the module.Module contract (see
// modules/identity/module.go) — to the legacy handlers.RouteRegistrar
// interface that routes.Setup still expects.
//
// This shim is scaffolding only: it exists so this task leaves main.go and
// routes.go untouched (their migration to mounting module.Module.Routes
// directly is task 1.4/1.5). Delete it once that migration lands.
type authRegistrar struct {
	h *identitytransport.AuthHandler
}

func (a authRegistrar) RegisterRoutes(g handlers.RouterGroup) {
	g.Public.Post("/auth/login", a.h.Login)
	g.Public.Post("/auth/refresh", a.h.Refresh)
	g.Public.Post("/auth/mfa/verify", a.h.MFAVerify)

	g.Protected.Post("/auth/logout", a.h.Logout)
	g.Protected.Post("/auth/mfa/setup", a.h.MFASetup)
	g.Protected.Post("/auth/mfa/setup/verify", a.h.MFAVerifySetup)
}

// New builds the full dependency graph and returns a ready Container.
func New(cfg *config.Config, db bun.IDB) (*Container, error) {
	// ── Shared infrastructure ────────────────────────────────────────────────
	bcryptHasher := hasher.BcryptHasher{}

	jwtSvc, err := token.NewJWTService(
		cfg.Auth.PrivateKeyPath,
		cfg.Auth.PublicKeyPath,
		cfg.Auth.Issuer,
	)
	if err != nil {
		return nil, fmt.Errorf("init jwt service: %w", err)
	}

	// ── Auth ─────────────────────────────────────────────────────────────────
	authUserRepo := authRepo.NewUserRepo(db)
	refreshTokenStore := authRepo.NewRefreshTokenStore(db)
	mfaStore := authRepo.NewMFAStore(db, bcryptHasher)
	totpSvc := infraMFA.NewTOTPService(cfg.Auth.Issuer)

	authSvc := appAuth.NewService(
		authUserRepo,
		bcryptHasher,
		jwtSvc,
		refreshTokenStore,
		appAuth.Config{
			AccessTokenTTL:    cfg.Auth.AccessTokenTTL,
			RefreshTokenTTL:   cfg.Auth.RefreshTokenTTL,
			MaxFailedAttempts: cfg.Auth.MaxFailedAttempts,
			LockDuration:      cfg.Auth.LockDuration,
			ClientID:          cfg.Auth.ClientID,
			MFATemporaryTTL:   cfg.Auth.MFATemporaryTTL,
		},
		totpSvc,
		mfaStore,
	)
	// authMW is derived from the same jwtSvc used to mint tokens, so it is
	// guaranteed non-nil here — construction above already returned on error.
	authMW := identitytransport.AuthRequired(jwtSvc)
	authHandler := identitytransport.NewAuthHandler(authSvc, cfg.Auth.RefreshTokenTTL, authMW)

	// ── User ─────────────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	userSvc := appUser.NewService(userRepo)
	userHandler := handlers.NewUserHandler(userSvc)

	// ── Register all handlers ─────────────────────────────────────────────────
	// To add a new handler: wire it above and append it here.
	return &Container{
		registrars: []handlers.RouteRegistrar{
			authRegistrar{h: authHandler},
			userHandler,
		},
	}, nil
}

// Registrars returns all RouteRegistrars to be passed to routes.Setup.
func (c *Container) Registrars() []handlers.RouteRegistrar {
	return c.registrars
}
