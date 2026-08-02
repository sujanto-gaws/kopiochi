// Package identity is the identity/auth business module: login, refresh,
// MFA, and the access-token verification middleware that protects its own
// routes. See docs/architectures/01-modularity/extension-framework.md and
// docs/architectures/02-composition/routing-and-versioning.md for the
// contract this module implements.
package identity

import (
	"errors"
	"fmt"
	"time"

	"github.com/sujanto-gaws/kopiochi/internal/module"
	application "github.com/sujanto-gaws/kopiochi/modules/identity/application"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/hasher"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/mfa"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
	"github.com/sujanto-gaws/kopiochi/modules/identity/transport"
)

// Config carries this module's settings, decoded from the host's config via
// mapstructure (see internal/config).
type Config struct {
	PrivateKeyPath    string        `mapstructure:"private_key_path"`
	PublicKeyPath     string        `mapstructure:"public_key_path"`
	Issuer            string        `mapstructure:"issuer"`
	ClientID          string        `mapstructure:"client_id"`
	AccessTokenTTL    time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL   time.Duration `mapstructure:"refresh_token_ttl"`
	MFATemporaryTTL   time.Duration `mapstructure:"mfa_temporary_ttl"`
	MaxFailedAttempts int           `mapstructure:"max_failed_attempts"`
	LockDuration      time.Duration `mapstructure:"lock_duration"`
	// TokenLeeway is the clock-skew allowance applied when validating a
	// token's exp (and iat/nbf, if present). Kept small and non-zero: see
	// docs/architectures/04-security/token-architecture.md.
	TokenLeeway time.Duration `mapstructure:"token_leeway"`
}

// Validate rejects configuration that would silently produce an unusable or
// insecure module (e.g. an unprotected route surface or tokens that never
// expire).
func (c Config) Validate() error {
	if c.Issuer == "" {
		return errors.New("identity: issuer is required")
	}
	if c.PrivateKeyPath == "" {
		return errors.New("identity: private_key_path is required")
	}
	if c.PublicKeyPath == "" {
		return errors.New("identity: public_key_path is required")
	}
	if c.AccessTokenTTL <= 0 {
		return errors.New("identity: access_token_ttl must be positive")
	}
	if c.RefreshTokenTTL <= 0 {
		return errors.New("identity: refresh_token_ttl must be positive")
	}
	if c.MFATemporaryTTL <= 0 {
		return errors.New("identity: mfa_temporary_ttl must be positive")
	}
	if c.MaxFailedAttempts <= 0 {
		return errors.New("identity: max_failed_attempts must be positive")
	}
	if c.TokenLeeway < 0 {
		return errors.New("identity: token_leeway must not be negative")
	}
	return nil
}

// New builds the identity module's dependency graph — the same graph
// cmd/api/container/container.go historically built inline — and returns a
// module.Module ready to be mounted by the composition root.
//
// The access-token verification middleware is derived from the same
// tokenIssuer used to mint tokens and is always non-nil by the time Routes is
// wired: if it cannot be built, New returns an error instead of a module that
// would silently serve unprotected routes (fail-closed, not fail-open).
//
// Migrations is left unset: the SQL migration files for this module still
// live under the top-level migrations/ directory and the goose-based runner
// has no embed.FS support yet. Wiring that up is a separate task.
func New(deps module.Deps, cfg Config) (*module.Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("identity: invalid config: %w", err)
	}

	bcryptHasher := hasher.BcryptHasher{}

	// audience is pinned to cfg.ClientID: every access/MFA token this
	// service mints carries aud=cfg.ClientID, and Validate requires it on
	// every token it verifies (see token-architecture.md, "aud").
	jwtSvc, err := token.NewJWTService(cfg.PrivateKeyPath, cfg.PublicKeyPath, cfg.Issuer, cfg.ClientID, cfg.TokenLeeway)
	if err != nil {
		return nil, fmt.Errorf("identity: init jwt service: %w", err)
	}

	authUserRepo := repository.NewUserRepo(deps.DB)
	refreshTokenStore := repository.NewRefreshTokenStore(deps.DB)
	mfaStore := repository.NewMFAStore(deps.DB, bcryptHasher)
	totpSvc := mfa.NewTOTPService(cfg.Issuer)

	authSvc := application.NewService(
		authUserRepo,
		bcryptHasher,
		jwtSvc,
		refreshTokenStore,
		application.Config{
			AccessTokenTTL:    cfg.AccessTokenTTL,
			RefreshTokenTTL:   cfg.RefreshTokenTTL,
			MaxFailedAttempts: cfg.MaxFailedAttempts,
			LockDuration:      cfg.LockDuration,
			ClientID:          cfg.ClientID,
			MFATemporaryTTL:   cfg.MFATemporaryTTL,
		},
		totpSvc,
		mfaStore,
	)

	// authMW is derived from the same jwtSvc used to mint tokens, so it is
	// guaranteed non-nil here — construction above already fails (and returns
	// early) if the token service cannot be built.
	authMW := transport.AuthRequired(jwtSvc)
	authHandler := transport.NewAuthHandler(authSvc, cfg.RefreshTokenTTL, authMW)

	return &module.Module{
		Name:   "identity",
		Routes: authHandler.Routes,
	}, nil
}
