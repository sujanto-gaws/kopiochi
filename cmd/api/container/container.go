// Package container wires the full dependency graph for the API server.
// To add a new handler:
//  1. Instantiate its repository, service, and handler here.
//  2. Return it from Registrars().
package container

import (
	"fmt"

	"github.com/uptrace/bun"

	appAuth "github.com/sujanto-gaws/kopiochi/internal/application/auth"
	appUser "github.com/sujanto-gaws/kopiochi/internal/application/user"
	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/hasher"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
	infraMFA "github.com/sujanto-gaws/kopiochi/internal/infrastructure/mfa"
	authRepo "github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/auth/repository"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/token"
)

// Container holds all wired handlers and exposes them as RouteRegistrars.
type Container struct {
	registrars []handlers.RouteRegistrar
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
	authHandler := handlers.NewAuthHandler(authSvc, cfg.Auth.RefreshTokenTTL)

	// ── User ─────────────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	userSvc := appUser.NewService(userRepo)
	userHandler := handlers.NewUserHandler(userSvc)

	// ── Register all handlers ─────────────────────────────────────────────────
	// To add a new handler: wire it above and append it here.
	return &Container{
		registrars: []handlers.RouteRegistrar{
			authHandler,
			userHandler,
		},
	}, nil
}

// Registrars returns all RouteRegistrars to be passed to routes.Setup.
func (c *Container) Registrars() []handlers.RouteRegistrar {
	return c.registrars
}
