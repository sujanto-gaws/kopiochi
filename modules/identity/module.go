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

	"github.com/sujanto-gaws/kopiochi/internal/audit"
	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/modules/identity/application"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/auditlog"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/hasher"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/mfa"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
	"github.com/sujanto-gaws/kopiochi/modules/identity/transport"
)

// Config carries this module's settings, decoded from the host's config via
// mapstructure (see internal/config).
type Config struct {
	PrivateKeyPath string `mapstructure:"private_key_path"`
	PublicKeyPath  string `mapstructure:"public_key_path"`
	// PreviousPublicKeyPath keeps a retired signing key in the verification
	// set during a rotation. Optional; empty means no rotation in progress.
	PreviousPublicKeyPath string        `mapstructure:"previous_public_key_path"`
	Issuer                string        `mapstructure:"issuer"`
	ClientID              string        `mapstructure:"client_id"`
	AccessTokenTTL        time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL       time.Duration `mapstructure:"refresh_token_ttl"`
	MFATemporaryTTL       time.Duration `mapstructure:"mfa_temporary_ttl"`
	MaxFailedAttempts     int           `mapstructure:"max_failed_attempts"`
	LockDuration          time.Duration `mapstructure:"lock_duration"`
	// TokenLeeway is the clock-skew allowance applied when validating a
	// token's exp (and iat/nbf, if present). Kept small and non-zero: see
	// docs/architectures/04-security/token-architecture.md.
	TokenLeeway time.Duration `mapstructure:"token_leeway"`

	// Notifier reports security-relevant events — account lockouts, MFA
	// enrollment — to a downstream channel. It is a dependency and not a
	// setting, and it is on Config anyway for the same reason
	// notification.Config carries EmailAddressResolver: Config is already
	// this module's whole contract with whoever builds it, and a second
	// constructor parameter would still need somewhere for a nil default to
	// live. The interface is declared in application/notifier.go, by the
	// consumer of the notification the composition root eventually sends
	// (R2); nothing here imports modules/notification. A nil value is
	// replaced with application.NopNotifier by application.NewService — the
	// same treatment Auditor gets.
	Notifier application.SecurityNotifier `mapstructure:"-"`
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
	if err == nil && cfg.PreviousPublicKeyPath != "" {
		// Fail rather than warn: a rotation configured with an unreadable
		// previous key would start, look healthy, and reject every token
		// issued before the switch — logging out every active session with
		// nothing in the logs pointing at the cause.
		err = jwtSvc.WithPreviousKey(cfg.PreviousPublicKeyPath)
	}
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
		// Security events go to the audit stream, not the request log: rare,
		// never sampled, and read months later during an incident review.
		auditlog.New(audit.New(deps.Logger)),
		// cfg.Notifier may be nil — a deployment with notifications off, or a
		// caller (a test) with nothing to wire — and NewService's own nil
		// check replaces it with NopNotifier rather than this package
		// choosing a default value.
		cfg.Notifier,
	)

	// authMW is derived from the same jwtSvc used to mint tokens, so it is
	// guaranteed non-nil here — construction above already fails (and returns
	// early) if the token service cannot be built.
	authMW := transport.AuthRequired(jwtSvc)
	authHandler := transport.NewAuthHandler(authSvc, cfg.RefreshTokenTTL, authMW, jwtSvc)

	return &module.Module{
		Name:   "identity",
		Routes: authHandler.Routes,
	}, nil
}
