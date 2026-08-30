// BuildApp is the composition root: it constructs every business module's
// dependency graph and returns the resulting App, ready to be mounted onto
// the HTTP router.
//
// To add a new module: build it here (wrapping errors with %w) and append it
// to mods.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog"
	"github.com/uptrace/bun"

	"github.com/google/uuid"
	"github.com/sujanto-gaws/kopiochi/internal/config"
	"github.com/sujanto-gaws/kopiochi/internal/module"

	"github.com/sujanto-gaws/kopiochi/internal/db"
	"github.com/sujanto-gaws/kopiochi/modules/identity"
	identityapp "github.com/sujanto-gaws/kopiochi/modules/identity/application"
	identityrepo "github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/identity/infrastructure/token"
	identitytransport "github.com/sujanto-gaws/kopiochi/modules/identity/transport"
	"github.com/sujanto-gaws/kopiochi/modules/notification"
	notifsender "github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/sender"
	"github.com/sujanto-gaws/kopiochi/modules/user"
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

	// Built once, ahead of both modules that need it: identity's
	// SecurityNotifier is derived from it below, and the notification module
	// itself is built from this same value further down. One Config, mapped
	// once, is what keeps the two from ever seeing a different notification
	// configuration for the same deployment.
	notificationCfg, err := notificationConfig(deps, cfg)
	if err != nil {
		return nil, fmt.Errorf("build notification config: %w", err)
	}

	securityNotifier, err := newSecurityNotifier(deps, notificationCfg, log)
	if err != nil {
		return nil, fmt.Errorf("build security notifier: %w", err)
	}

	identityMod, err := identity.New(deps, identity.Config{
		PrivateKeyPath:        cfg.Auth.PrivateKeyPath,
		PublicKeyPath:         cfg.Auth.PublicKeyPath,
		PreviousPublicKeyPath: cfg.Auth.PreviousPublicKeyPath,
		Issuer:                cfg.Auth.Issuer,
		ClientID:              cfg.Auth.ClientID,
		AccessTokenTTL:        cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL:       cfg.Auth.RefreshTokenTTL,
		MFATemporaryTTL:       cfg.Auth.MFATemporaryTTL,
		MaxFailedAttempts:     cfg.Auth.MaxFailedAttempts,
		LockDuration:          cfg.Auth.LockDuration,
		TokenLeeway:           cfg.Auth.TokenLeeway,
		Notifier:              securityNotifier,
	})
	if err != nil {
		return nil, fmt.Errorf("build identity module: %w", err)
	}
	mods = append(mods, identityMod)

	userMod, err := newUserModule(deps, cfg)
	if err != nil {
		return nil, fmt.Errorf("build user module: %w", err)
	}
	mods = append(mods, userMod)

	// The notification module is appended whether or not it is enabled: a
	// disabled one mounts no routes and starts no dispatcher, but it is still
	// a module the boot log names, and pretending it is absent would make
	// "which modules does this deployment run" a question with two answers.
	//
	// notification.New starts the dispatcher, so from here on the App owns a
	// goroutine. serve pushes every module's Close onto the lifecycle stack,
	// which is what stops it; a caller that builds an App and drops it — a
	// test — must call Close itself.
	notificationMod, err := notification.New(deps, notificationCfg)
	if err != nil {
		return nil, fmt.Errorf("build notification module: %w", err)
	}
	mods = append(mods, notificationMod)

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

// newUserModule builds the user module's one cross-module dependency — the
// authentication middleware that protects its routes — and hands it to
// user.New.
//
// It derives that middleware from its own token verifier, built from the
// same auth config the identity module uses to mint tokens, rather than
// reaching into identity's internals: module.Module intentionally exposes
// nothing but Name/Routes/Migrations/Close, so cross-module auth wiring goes
// through shared config and the composition root, not a back-channel into
// another module's private dependency graph. The small cost is that the RSA
// keys are parsed twice at startup; that trade-off is preferable to coupling
// two otherwise-independent modules.
//
// Since 3.6b the module itself owns its dependency graph (modules/user/
// module.go); this function is only the glue that satisfies user.Config.
func newUserModule(deps module.Deps, cfg *config.Config) (*module.Module, error) {
	// authMW is derived from a verifier this function builds, so it is
	// guaranteed non-nil here — construction already returned on error
	// otherwise. user.Config.Validate rejects a nil middleware regardless,
	// so the module fails closed rather than serving unprotected routes.
	authMW, err := newAuthMiddleware(cfg)
	if err != nil {
		return nil, err
	}

	return user.New(deps, user.Config{AuthMiddleware: authMW})
}

// newAuthMiddleware builds an access-token middleware from the shared auth
// config.
//
// One function with two callers rather than a copy per module, because the
// thing being duplicated is a security decision — which key, which issuer,
// which audience, which leeway — and two copies of it are two places for a
// deployment to end up verifying tokens differently. The RSA keys are parsed
// once per call, which is once per module at boot and never again.
//
// It deliberately does NOT extract identity's verifier: module.Module exposes
// nothing but Name/Routes/Migrations/Close, so cross-module auth wiring goes
// through shared config and this file, not a back-channel into another
// module's private dependency graph (R2).
//
// The return type is spelled out rather than written as authn.Middleware, which
// is the same type — an alias — but naming it would put an import edge from
// cmd/api to internal/authn, and tools/archtest fences that contract to
// modules/*, internal/httpx, internal/testsupport and the conformance suite.
// The composition root supplies the middleware; it is not a consumer of the
// contract.
func newAuthMiddleware(cfg *config.Config) (func(http.Handler) http.Handler, error) {
	jwtSvc, err := token.NewJWTService(cfg.Auth.PrivateKeyPath, cfg.Auth.PublicKeyPath, cfg.Auth.Issuer, cfg.Auth.ClientID, cfg.Auth.TokenLeeway)
	if err != nil {
		return nil, fmt.Errorf("init token verifier: %w", err)
	}
	return identitytransport.AuthRequired(jwtSvc), nil
}

// notificationConfig maps the host's notification config onto the module's
// own Config. It builds the Config only — not the module — so BuildApp can
// hand the identical value to both notification.NewEnqueuer (identity's
// SecurityNotifier) and notification.New (the module itself), rather than
// mapping cfg.Notification twice and risking the two drift apart the way a
// single mapping already can (E29: a field added to the module's Config and
// not mirrored here is silently dropped, not a compile error).
//
// The mapping is written out field by field, like identity's, rather than
// handing the module the host's struct: the module's Config is its contract
// with whoever builds it, and a shared struct would make every host — a test,
// a second binary — carry the whole application config to construct one
// module. The cost is this function; the benefit is that a renamed field here
// is a compile error rather than a silently-zeroed setting.
//
// Validation belongs to the module and runs inside New and NewEnqueuer alike
// (fail closed: email enabled without a host, a from address or
// APP_NOTIFICATION_EMAIL_PASSWORD is a boot failure, not a default).
// internal/config cannot do it, because the shared kernel must not import a
// business module.
func notificationConfig(deps module.Deps, cfg *config.Config) (notification.Config, error) {
	n := cfg.Notification

	// The address resolver is the cross-module edge, and it is built here for
	// the same reason the user module's auth middleware is: neither module
	// imports the other, and the composition root is the only place that knows
	// both exist. identity.New's signature is untouched — this constructs its
	// own repository over the same handle, exactly as newUserModule
	// constructs its own token service rather than extracting identity's.
	//
	// Only wired when email is on. Building it unconditionally would attach a
	// repository to a module that will never call it.
	var resolver notifsender.AddressResolver
	if n.Email.Enabled {
		resolver = identityEmailResolver{users: identityrepo.NewUserRepo(deps.DB)}
	}

	// The middleware every notification route mounts behind, built here for
	// exactly the same reason as the resolver above and as the user module's:
	// neither module imports the other, and the composition root is the only
	// place that knows both exist.
	//
	// Built only for an enabled module. A disabled one mounts no routes, so it
	// needs no middleware — and it must not need a keypair either, or "switch
	// notifications off" would stop being an answer for a deployment that has
	// no auth keys on disk.
	var authMW func(http.Handler) http.Handler
	if n.Enabled {
		var err error
		if authMW, err = newAuthMiddleware(cfg); err != nil {
			return notification.Config{}, err
		}
	}

	return notification.Config{
		Enabled:              n.Enabled,
		Auth:                 authMW,
		EmailAddressResolver: resolver,
		Dispatcher: notification.DispatcherConfig{
			PollInterval: n.Dispatcher.PollInterval,
			BatchSize:    n.Dispatcher.BatchSize,
			Workers:      n.Dispatcher.Workers,
			MaxAttempts:  n.Dispatcher.MaxAttempts,
			BackoffBase:  n.Dispatcher.BackoffBase,
			BackoffCap:   n.Dispatcher.BackoffCap,
			StalledAfter: n.Dispatcher.StalledAfter,
			DrainTimeout: n.Dispatcher.DrainTimeout,
		},
		Email: notification.EmailConfig{
			Enabled:  n.Email.Enabled,
			SMTPHost: n.Email.SMTPHost,
			SMTPPort: n.Email.SMTPPort,
			From:     n.Email.From,
			Username: n.Email.Username,
			Timeout:  n.Email.Timeout,
			// Still a secret.String on both sides: it is copied, never
			// revealed, and the SMTP sender is the only thing that calls
			// Reveal, at dial time.
			Password: n.Email.Password,
		},
		LogSender: notification.LogSenderConfig{
			Enabled: n.LogSender.Enabled,
			Channel: n.LogSender.Channel,
		},
	}, nil
}

// newSecurityNotifier builds identity's SecurityNotifier over the
// notification module's outbox.
//
// It uses notification.NewEnqueuer, not the *application.Service the
// notification module builds for itself inside New: module.Module exposes
// nothing but Name/Routes/Migrations/Close (R2), so by the time notification.
// New has returned one there is no handle left to reach Enqueue through.
// NewEnqueuer is the module's own answer to that — a second, non-dispatching
// build from the same Config — documented at modules/notification/
// enqueuer.go and modules/notification/module.go.
//
// A disabled notification module gets identity's own NopNotifier directly,
// rather than an adapter wrapping NewEnqueuer's no-op Enqueuer: both behave
// identically, but skipping the adapter means "notifications are off" has one
// obvious shape to read here instead of two equivalent ones.
func newSecurityNotifier(deps module.Deps, notificationCfg notification.Config, log zerolog.Logger) (identityapp.SecurityNotifier, error) {
	if !notificationCfg.Enabled {
		return identityapp.NopNotifier(), nil
	}

	enqueue, err := notification.NewEnqueuer(deps, notificationCfg)
	if err != nil {
		return nil, fmt.Errorf("build notification enqueuer: %w", err)
	}
	return newSecurityNotifierAdapter(enqueue, log), nil
}

// identityEmailResolver answers "what is this recipient's email address" out of
// the identity module's user table.
//
// A recipient id is an auth_users id and nothing else — there is no profile
// hop, because E16 keyed the profile by that same id and removed the addressable
// one — so this is a single lookup with no mapping to get wrong.
//
// It is the whole adapter: one call and one error translation. Anything more
// here would be business logic in the composition root.
type identityEmailResolver struct {
	users *identityrepo.UserRepo
}

// ResolveEmail translates the storage layer's vocabulary into the port's.
//
// The translation is the point, and it is the reason #55 converted this
// repository to db.ErrNotFound: a recipient who does not exist can never be
// delivered to, so it becomes notifsender.ErrNoAddress, which wraps
// domain.ErrNonRetryable and dead-letters the row on the first attempt.
// Everything else — a dropped connection, a timeout — passes through as a
// plain error and is retried.
//
// Backwards one way, a deleted user is retried until its budget is gone;
// backwards the other, a security mail is destroyed by a network blip.
// Classified with errors.Is, never by message.
func (r identityEmailResolver) ResolveEmail(ctx context.Context, recipientID uuid.UUID) (string, error) {
	user, err := r.users.FindByID(ctx, recipientID.String())
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", fmt.Errorf("%w: no user %s", notifsender.ErrNoAddress, recipientID)
		}
		return "", fmt.Errorf("look up recipient %s: %w", recipientID, err)
	}

	// An account with no address on file is the same answer as no account:
	// there is nowhere to deliver, and no retry creates one.
	if user.Email == "" {
		return "", fmt.Errorf("%w: user %s has no email on file", notifsender.ErrNoAddress, recipientID)
	}
	return user.Email, nil
}
