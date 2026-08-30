package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/sujanto-gaws/kopiochi/internal/module"
	"github.com/sujanto-gaws/kopiochi/modules/notification/application"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/persistence/repository"
	"github.com/sujanto-gaws/kopiochi/modules/notification/infrastructure/template"
)

// Enqueuer is the write side of this module's outbox: the one capability a
// cross-module producer needs. It is exactly application.Service's Enqueue
// method, named here at the package boundary so a caller outside this module
// (cmd/api) can hold one without needing to know application.Service exists.
type Enqueuer interface {
	Enqueue(ctx context.Context, req application.EnqueueRequest) error
}

// NewEnqueuer builds a standalone Enqueuer: the same repositories over the
// same database handle, the same routing table buildSenders assembles for
// New, so an unroutable channel is refused identically whichever entry point
// wrote the row. It never dispatches — no goroutine starts, and rendering
// happens at drain, not at enqueue (application.TemplateRenderer), so there
// is nothing here for a template failure to dead-letter and nothing for a
// caller to Close.
//
// It exists because module.Module exposes nothing but
// Name/Routes/Migrations/Close (see the package doc's rationale for the
// two-value New), and a cross-module producer arriving after New has already
// built and returned one of those — identity, since D9 — has no other way to
// reach this module's Enqueue. The alternatives were changing New's return
// arity, which the package doc already declines, or duplicating sender and
// repository construction in cmd/api, which would drift from buildSenders the
// same way a second field mapping already has (E29). This is one new
// exported name instead of either, and cmd/api is expected to build the
// notification.Config it passes here once and reuse it for both this and New,
// so the two never see a different configuration for the same deployment.
//
// A disabled config returns a no-op Enqueuer rather than an error, matching
// New's own fail-safe posture for the same input: a deployment with
// notifications off must still be safe to call into.
func NewEnqueuer(deps module.Deps, cfg Config) (Enqueuer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("notification: invalid config: %w", err)
	}

	if !cfg.Enabled {
		return noopEnqueuer{}, nil
	}

	if deps.DB == nil {
		return nil, errors.New("notification: a database is required when the module is enabled")
	}

	renderer, err := template.New()
	if err != nil {
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
		// No observer here either, for the same reason New passes nil: the
		// metrics and audit adapters are D10's, and Enqueue never calls the
		// observer port — only the dispatch cycle does.
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
	return svc, nil
}

// noopEnqueuer is what a disabled module's Enqueuer does with an enqueue
// request: nothing, successfully. It exists so a caller that built one from a
// disabled Config does not need its own branch for "nowhere to write this" —
// the same shape New's own disabled() gives Routes and Close.
type noopEnqueuer struct{}

func (noopEnqueuer) Enqueue(context.Context, application.EnqueueRequest) error { return nil }
