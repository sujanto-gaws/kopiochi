// notifier_adapter.go is the whole D9 adapter: it translates identity's
// application.SecurityNotifier primitives into the notification module's
// EnqueueRequest and hands them to a notification.Enqueuer. It lives here,
// and not in either module, because cmd/api is the only package this
// repository lets see both (R2, dependency-rules.md) — identity must not
// import modules/notification, and notification has no reason to know
// identity exists.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	identityapp "github.com/sujanto-gaws/kopiochi/modules/identity/application"
	notifapp "github.com/sujanto-gaws/kopiochi/modules/notification/application"
	notifdomain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// securityChannels is every channel a security notification goes out on.
//
// Both, unconditionally — Enqueue itself is what decides whether a channel is
// actually routable (an unrouted one is refused, not silently dropped, E13),
// and the in-app row and the email attempt are independent: an operator with
// email switched off must still get the in-app copy, and one channel being
// unroutable must not suppress the other. It mirrors the shipped
// security.password_changed family, which ships both an email and an in-app
// part for exactly this reason.
var securityChannels = []notifdomain.Channel{notifdomain.ChannelEmail, notifdomain.ChannelInApp}

// notifierEnqueuer is the subset of notification.Enqueuer this adapter needs.
//
// Declared here, on the consumer's side, per R2: notification.NewEnqueuer's
// return value already satisfies it structurally, and this type exists so the
// adapter's dependency is stated in its own words rather than by importing a
// wider interface than it uses.
type notifierEnqueuer interface {
	Enqueue(ctx context.Context, req notifapp.EnqueueRequest) error
}

// securityNotifierAdapter implements identity's application.SecurityNotifier
// over the notification module's outbox.
type securityNotifierAdapter struct {
	enqueue notifierEnqueuer
	log     zerolog.Logger
}

// newSecurityNotifierAdapter wraps enqueue. Callers that want "no
// notifications" pass identityapp.NopNotifier() at the identity.Config.
// Notifier call site instead of constructing this type with a no-op enqueuer —
// see newSecurityNotifier in container.go.
func newSecurityNotifierAdapter(enqueue notifierEnqueuer, log zerolog.Logger) *securityNotifierAdapter {
	return &securityNotifierAdapter{enqueue: enqueue, log: log}
}

var _ identityapp.SecurityNotifier = (*securityNotifierAdapter)(nil)

// AccountLocked notifies userID that their account just locked, until
// lockedUntil.
func (a *securityNotifierAdapter) AccountLocked(ctx context.Context, userID string, lockedUntil time.Time) {
	a.notify(ctx, userID, "account_locked", "security.account_locked",
		lockedUntil, map[string]any{"LockedUntil": formatEventTime(lockedUntil)})
}

// MFAEnabled notifies userID that a second factor was just enabled, at
// enabledAt.
func (a *securityNotifierAdapter) MFAEnabled(ctx context.Context, userID string, enabledAt time.Time) {
	a.notify(ctx, userID, "mfa_enabled", "security.mfa_enabled",
		enabledAt, map[string]any{"EnabledAt": formatEventTime(enabledAt)})
}

// notify enqueues one event across every security channel.
//
// Every parameter already comes from identity's own application layer (see
// identityapp.SecurityNotifier); the only translation left is into this
// module's vocabulary — an EnqueueRequest, a payload, an idempotency key
// derivable from the event.
//
// Enqueue failures are logged, never returned: identityapp.SecurityNotifier's
// methods return nothing, by design, so a login or an MFA confirmation can
// never fail because a downstream notification could not be written. This
// mirrors how modules/identity/infrastructure/auditlog treats the audit
// port — Emit has no error return either — but for a different reason: a log
// write cannot meaningfully fail its caller, while Enqueue genuinely can
// (an unroutable channel, a database that is down). Because that failure is
// real, this is not silent the way a swallowed log write would be: every
// failure is logged at Warn, once per channel, so the gap stays visible to an
// operator even though it never reaches the caller.
func (a *securityNotifierAdapter) notify(ctx context.Context, userID, event, templateKey string, eventTime time.Time, payload map[string]any) {
	recipient, err := uuid.Parse(userID)
	if err != nil {
		a.log.Warn().Err(err).Str("event", event).Str("user_id", userID).
			Msg("security notification: user id is not a UUID; nothing enqueued")
		return
	}

	for _, ch := range securityChannels {
		// The idempotency key carries the channel, which the plan's
		// <event>:<userID>:<eventID> format alone does not. Postgres'
		// idx_notifications_idem is one partial-unique index over the whole
		// outbox, not scoped by channel (see notification_repo.go), so two
		// Enqueue calls for the same event that shared a three-segment key
		// would collide on the second insert and ON CONFLICT DO NOTHING it —
		// silently losing that channel's copy, not deduplicating a retry.
		// Appending the channel keeps the two rows independent while keeping
		// each one idempotent against a retry of itself.
		key := fmt.Sprintf("%s:%s:%s:%s", event, userID, eventTime.UTC().Format(time.RFC3339Nano), ch)

		if err := a.enqueue.Enqueue(ctx, notifapp.EnqueueRequest{
			RecipientID:    recipient,
			Channel:        ch,
			Category:       notifdomain.CategorySecurity,
			TemplateKey:    templateKey,
			Payload:        payload,
			IdempotencyKey: key,
		}); err != nil {
			a.log.Warn().Err(err).Str("event", event).Str("channel", string(ch)).Str("user_id", userID).
				Msg("security notification: enqueue failed")
		}
	}
}

// formatEventTime renders a timestamp the way the shipped
// security.password_changed family's payload already does in its own tests
// (modules/notification/module_integration_test.go), e.g.
// "30 August 2026 at 09:14 UTC" — legible in an email or an in-app card, not
// a machine timestamp.
func formatEventTime(t time.Time) string {
	return t.UTC().Format("2 January 2006 at 15:04 MST")
}
