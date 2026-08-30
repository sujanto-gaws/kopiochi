package notification_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/modules/notification"
	napp "github.com/sujanto-gaws/kopiochi/modules/notification/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// TestNewEnqueuer_DisabledIsANoOp mirrors New's own "off means off": nothing
// built, nothing to fail an Enqueue call against, and nothing here needs a
// database.
func TestNewEnqueuer_DisabledIsANoOp(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Enabled = false

	e, err := notification.NewEnqueuer(deps(t, nil), cfg)
	if err != nil {
		t.Fatalf("NewEnqueuer: %v", err)
	}
	if err := e.Enqueue(context.Background(), napp.EnqueueRequest{
		RecipientID: uuid.New(),
		Channel:     domain.ChannelInApp,
		Category:    domain.CategorySecurity,
		TemplateKey: "does.not.exist",
	}); err != nil {
		t.Errorf("Enqueue on a disabled Enqueuer = %v, want nil", err)
	}
}

// TestNewEnqueuer_EnabledBuildsOverTheSameConfigAsNew: an enabled Enqueuer
// builds against the same routing table New would, so an unroutable channel
// is refused identically. In-app is unconditional; email is not configured
// here, so it must be refused.
func TestNewEnqueuer_EnabledBuildsAndRoutesLikeNew(t *testing.T) {
	t.Parallel()

	cfg := enabledConfig()

	e, err := notification.NewEnqueuer(deps(t, lazyDB(t)), cfg)
	if err != nil {
		t.Fatalf("NewEnqueuer: %v", err)
	}

	err = e.Enqueue(context.Background(), napp.EnqueueRequest{
		RecipientID: uuid.New(),
		Channel:     domain.ChannelEmail,
		Category:    domain.CategorySecurity,
		TemplateKey: "security.account_locked",
	})
	if !strings.Contains(err.Error(), "no sender registered for channel") {
		t.Errorf("Enqueue for an unconfigured email channel = %v, want ErrChannelNotRoutable", err)
	}
}

// TestNewEnqueuer_RefusesAnInvalidConfig: fail closed, matching New.
func TestNewEnqueuer_RefusesAnInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := enabledConfig()
	cfg.Dispatcher.Workers = 0

	if _, err := notification.NewEnqueuer(deps(t, lazyDB(t)), cfg); err == nil {
		t.Fatal("NewEnqueuer accepted a config with no workers")
	}
}

// TestNewEnqueuer_EnabledRefusesToBuildWithoutADatabase: the same fail-closed
// posture New has for the same reason — a wiring mistake belongs at boot, not
// discovered by the first caller of Enqueue.
func TestNewEnqueuer_EnabledRefusesToBuildWithoutADatabase(t *testing.T) {
	t.Parallel()

	if _, err := notification.NewEnqueuer(deps(t, nil), enabledConfig()); err == nil {
		t.Fatal("NewEnqueuer built an enabled Enqueuer with no database")
	}
}
