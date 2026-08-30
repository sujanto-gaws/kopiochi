// Package notification is the notification capability: an outbox other modules
// write to, a background dispatcher that drains it, and (from D7) a read model
// over a user's in-app mailbox and delivery preferences.
//
// Nothing is sent inside a request. Enqueue commits a row in the caller's
// transaction and returns; delivery, retries and dead-lettering happen later on
// the dispatcher this package starts. That is what keeps request latency
// independent of SMTP, and what makes a crash between the business action and
// the notification lose nothing.
//
// The constructor returns (*module.Module, error) — two values, like every
// other module in this tree. The plan and the blueprint each describe a third
// return value carrying the service, and nothing consumes one: module.Module
// already carries the Close the dispatcher needs, and when a cross-module
// producer arrives it will declare the narrow interface it needs on its own
// side and be satisfied at the composition root (R2). Adding a return value
// then is one line at one call site; publishing one now is a contract nobody
// asked for.
package notification

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/modules/notification/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/dispatcher"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/sender"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/template"
)

// Name is the module's name on the lifecycle stack and in the boot log.
const Name = "notification"

// New builds the module's dependency graph, starts the dispatcher, and returns
// a module.Module whose Close stops it.
//
// The dispatcher is started here rather than by the host because this package
// is what knows it exists. The host's only obligation is to call Close, which
// the composition root already does for every module — cmd/api pushes
// m.Close onto the lifecycle stack, which unwinds LIFO.
//
// Disabled, it returns a module that mounts no routes, starts no goroutine and
// touches no database, with a Close that is still safe to call. That state is
// reachable by configuration and must not be reachable by accident, which is
// why an enabled module with no database is an error rather than a module that
// builds and fails at the first tick.
func New(deps module.Deps, cfg Config) (*module.Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("notification: invalid config: %w", err)
	}

	if !cfg.Enabled {
		return disabled(), nil
	}

	if deps.DB == nil {
		return nil, errors.New("notification: a database is required when the module is enabled")
	}

	renderer, err := template.New()
	if err != nil {
		// The templates are embedded in this binary, so this is a build that
		// cannot render its own messages. Refusing to start is the only
		// honest response: the alternative is a dispatcher that dead-letters
		// every notification it claims.
		return nil, fmt.Errorf("notification: load templates: %w", err)
	}

	senders, err := buildSenders(deps, cfg)
	if err != nil {
		return nil, err
	}

	notifications := repository.NewNotificationRepo(deps.DB)
	preferences := repository.NewPreferenceRepo(deps.DB)

	svc, err := application.NewService(
		notifications,
		preferences,
		renderer,
		senders,
		systemClock{},
		jitter{},
		// No observer. The metrics and audit adapters are D10's; the port is
		// optional and nil means nothing is reported (E12).
		nil,
		application.DispatchConfig{
			BatchSize:   cfg.Dispatcher.BatchSize,
			MaxAttempts: cfg.Dispatcher.MaxAttempts,
			BackoffBase: cfg.Dispatcher.BackoffBase,
			BackoffCap:  cfg.Dispatcher.BackoffCap,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("notification: build service: %w", err)
	}

	disp, err := dispatcher.New(
		dispatcher.Deps{
			Batcher:       svc,
			Notifications: notifications,
			Clock:         systemClock{},
			Logger:        deps.Logger.With().Str("component", "notification-dispatcher").Logger(),
			Jitter:        jitter{},
		},
		dispatcher.Config{
			PollInterval: cfg.Dispatcher.PollInterval,
			Workers:      cfg.Dispatcher.Workers,
			BatchSize:    cfg.Dispatcher.BatchSize,
			StalledAfter: cfg.Dispatcher.StalledAfter,
			MaxAttempts:  cfg.Dispatcher.MaxAttempts,
			BackoffBase:  cfg.Dispatcher.BackoffBase,
			BackoffCap:   cfg.Dispatcher.BackoffCap,
			DrainTimeout: cfg.Dispatcher.DrainTimeout,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("notification: build dispatcher: %w", err)
	}
	disp.Start()

	return &module.Module{
		Name:   Name,
		Routes: noRoutes,
		// Whoever starts a goroutine registers the way to stop it, once. The
		// lifecycle stack calls this exactly once, in reverse construction
		// order, and there is deliberately no second `defer` anywhere.
		//
		// module.Module.Close takes no context, so it cannot inherit the
		// process's shutdown deadline — which is why the drain carries a
		// bound of its own (dispatcher.drain_timeout). See the finding in the
		// PR: a background-worker module has no way to be told how long
		// shutdown may take.
		Close: func() error {
			return disp.Stop(context.Background())
		},
	}, nil
}

// disabled is the module a `notification.enabled: false` deployment gets.
//
// Close is a no-op rather than nil so that "is this module closeable" is never
// a question a caller has to answer differently depending on configuration.
func disabled() *module.Module {
	return &module.Module{
		Name:   Name,
		Routes: noRoutes,
		Close:  func() error { return nil },
	}
}

// noRoutes mounts nothing.
//
// It is a function that does nothing rather than a nil field because
// httpx.Mount calls m.Routes unconditionally — Close and Migrations are
// nil-guarded there and Routes is not, so a genuinely routeless module would
// panic the router at boot. Reported as a finding; harmless to satisfy here.
func noRoutes(chi.Router) {}

// systemClock is the real clock. It exists because the application layer takes
// time as a port so its tests can fix it, which leaves exactly one place —
// here, at the composition seam — that is allowed to call time.Now.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// jitter spreads retry schedules across a herd of rows that failed on the same
// outage.
//
// math/rand/v2's top-level Float64 is race-free by design, so this needs no
// state and no lock. A package-level *rand.Rand would need both.
type jitter struct{}

func (jitter) Float64() float64 { return rand.Float64() }

// buildSenders assembles the routing table.
//
// The set built here is exactly the set of channels this deployment can
// enqueue: Enqueue refuses a channel with no sender (E13), which is what turns
// a misconfiguration into an error at the producer instead of a row that dies
// unattempted hours later.
//
// In-app is unconditional — the row is the notification, and without its
// no-op sender the mailbox could not be written to at all. Email is registered
// when it is configured, and only then: with email.enabled false there is no
// email sender, so an enqueue for that channel is refused at the producer
// rather than accepted and dropped.
func buildSenders(deps module.Deps, cfg Config) ([]application.ChannelSender, error) {
	senders := []application.ChannelSender{sender.NewInApp()}

	if cfg.Email.Enabled {
		smtpSender, err := sender.NewSMTP(
			sender.SMTPDeps{
				Resolver: cfg.EmailAddressResolver,
				Clock:    systemClock{},
			},
			sender.SMTPConfig{
				Host:        cfg.Email.SMTPHost,
				Port:        cfg.Email.SMTPPort,
				ImplicitTLS: sender.PortUsesImplicitTLS(cfg.Email.SMTPPort),
				Username:    cfg.Email.Username,
				Password:    cfg.Email.Password,
				From:        cfg.Email.From,
				Timeout:     cfg.Email.Timeout,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("notification: build smtp sender: %w", err)
		}
		senders = append(senders, smtpSender)
	}

	if cfg.LogSender.Enabled {
		logSender, err := sender.NewLog(
			deps.Logger.With().Str("component", "notification-log-sender").Logger(),
			domain.Channel(cfg.LogSender.Channel),
		)
		if err != nil {
			return nil, fmt.Errorf("notification: build log sender: %w", err)
		}
		senders = append(senders, logSender)
	}

	return senders, nil
}
