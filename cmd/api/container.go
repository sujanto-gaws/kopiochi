// BuildApp is the composition root: it constructs every business module's
// dependency graph and returns the resulting App, ready to be mounted onto
// the HTTP router.
//
// To add a new module: build it here (wrapping errors with %w) and append it
// to mods.
package main

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"

	appUser "github.com/sujanto-gaws/kopiochi/internal/application/user"
	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/http/handlers"
	"github.com/sujanto-gaws/kopiochi/internal/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/modules/identity"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
	identitytransport "github.com/sujanto-gaws/kopiochi/modules/identity/transport"
)

// App is the fully wired application: every business module the composition
// root has assembled. The HTTP layer never reaches into a module's
// internals — only Name, Routes, Migrations, and Close.
type App struct {
	Modules []*module.Module
}

// BuildApp constructs the application's module graph.
//
// It refuses to return an application with zero modules: an empty App would
// start, log "application starting", answer the health check, and serve
// nothing — exactly the failure mode that went unnoticed before this guard
// existed.
func BuildApp(cfg *config.Config, db bun.IDB, log zerolog.Logger) (*App, error) {
	deps := module.Deps{DB: db, Logger: log}

	var mods []*module.Module

	identityMod, err := identity.New(deps, identity.Config{
		PrivateKeyPath:    cfg.Auth.PrivateKeyPath,
		PublicKeyPath:     cfg.Auth.PublicKeyPath,
		Issuer:            cfg.Auth.Issuer,
		ClientID:          cfg.Auth.ClientID,
		AccessTokenTTL:    cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL:   cfg.Auth.RefreshTokenTTL,
		MFATemporaryTTL:   cfg.Auth.MFATemporaryTTL,
		MaxFailedAttempts: cfg.Auth.MaxFailedAttempts,
		LockDuration:      cfg.Auth.LockDuration,
	})
	if err != nil {
		return nil, fmt.Errorf("build identity module: %w", err)
	}
	mods = append(mods, identityMod)

	userMod, err := newUserModule(cfg, db)
	if err != nil {
		return nil, fmt.Errorf("build user module: %w", err)
	}
	mods = append(mods, userMod)

	if len(mods) == 0 {
		return nil, errors.New("no modules registered: refusing to start an empty application")
	}

	names := make([]string, len(mods))
	for i, m := range mods {
		names[i] = m.Name
	}
	log.Info().Strs("modules", names).Msg("modules registered")

	return &App{Modules: mods}, nil
}

// newUserModule wraps the legacy user handler (internal/infrastructure/http/
// handlers) as a module.Module so its routes keep serving under /api/v1
// alongside modules that implement the module.Module contract natively.
//
// It builds its own token verifier from the same auth config the identity
// module uses to mint tokens, rather than reaching into identity's internals:
// module.Module intentionally exposes nothing but Name/Routes/Migrations/
// Close, so cross-module auth wiring goes through shared config, not a
// back-channel into another module's private dependency graph. The small
// cost is that the RSA keys are parsed twice at startup; that trade-off is
// preferable to coupling two otherwise-independent modules.
func newUserModule(cfg *config.Config, db bun.IDB) (*module.Module, error) {
	jwtSvc, err := token.NewJWTService(cfg.Auth.PrivateKeyPath, cfg.Auth.PublicKeyPath, cfg.Auth.Issuer)
	if err != nil {
		return nil, fmt.Errorf("init token verifier: %w", err)
	}
	// authMW is derived from the same jwtSvc used above, so it is guaranteed
	// non-nil here — construction already returned on error otherwise
	// (fail-closed, never a module serving unprotected routes silently).
	authMW := identitytransport.AuthRequired(jwtSvc)

	userRepo := repository.NewUserRepository(db)
	userSvc := appUser.NewService(userRepo)
	userHandler := handlers.NewUserHandler(userSvc, authMW)

	return &module.Module{
		Name:   "user",
		Routes: userHandler.Routes,
	}, nil
}
