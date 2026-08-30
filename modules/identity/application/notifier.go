package application

import (
	"context"
	"time"
)

// SecurityNotifier is told about security-relevant account events so a
// downstream channel — email, in-app — can alert the account holder.
//
// Declared here, as a narrow interface of exactly what this module needs,
// rather than the module importing the notification module's types: this
// module must not import modules/notification (R2,
// docs/architectures/01-modularity/dependency-rules.md), so the composition
// root supplies an implementation that translates these primitives into
// whatever the notification module's EnqueueRequest wants, and a test
// supplies a fake. This mirrors Auditor exactly — same reasoning, same shape.
//
// Only events with a real producer in this module today are declared here. A
// "password changed" notification has no producer: there is no
// password-change route, use case or method anywhere in identity. It is
// deliberately absent, even though the notification module ships a
// security.password_changed template family — that family exists ahead of
// its producer and renders nothing until one arrives.
type SecurityNotifier interface {
	// AccountLocked reports that userID's account just transitioned into a
	// lockout, effective until lockedUntil.
	//
	// Callers emit this once per lockout episode, on the transition (see
	// login.go's `!wasLocked && user.IsLocked()` guard) — never once per
	// subsequent attempt against an already-locked account, which is what
	// keeps the event alertable instead of noisy on a sustained brute force.
	//
	// lockedUntil is the deterministic identity of the episode: it is minted
	// once, by domain.User.RecordFailedLogin, at the moment the lockout
	// starts, and is unchanged for as long as the account stays locked. A
	// caller building an idempotency key from it dedupes a retried report of
	// the same episode without needing a separately generated event id.
	AccountLocked(ctx context.Context, userID string, lockedUntil time.Time)

	// MFAEnabled reports that userID just enabled a second factor, at
	// enabledAt.
	//
	// Unlike AccountLocked, domain.User carries no persisted "enabled at"
	// timestamp — adding one is a domain change outside this interface's
	// task. enabledAt is instead the moment application code observed the
	// transition (mfa_verify_setup.go), which is stable for the one call a
	// single VerifyMFASetup invocation makes but is NOT a guard against two
	// separate confirmations: nothing in VerifyMFASetup checks whether MFA
	// is already enabled, and each confirmation reissues a fresh set of
	// backup codes, so each is a materially new security event and
	// correctly gets a distinct timestamp rather than being collapsed into
	// one.
	MFAEnabled(ctx context.Context, userID string, enabledAt time.Time)
}

// nopNotifier discards every event.
//
// It exists for the same reason nopAuditor does: NewService must never hold a
// nil SecurityNotifier, or a security event needing to be reported would
// panic instead of being recorded — the worst possible time to discover
// incomplete wiring. A caller that genuinely wants no notifications (the
// notification module disabled, or a test with nothing to assert on this
// port) says so explicitly by passing this.
type nopNotifier struct{}

func (nopNotifier) AccountLocked(context.Context, string, time.Time) {}
func (nopNotifier) MFAEnabled(context.Context, string, time.Time)    {}

// NopNotifier returns a SecurityNotifier that discards everything.
func NopNotifier() SecurityNotifier { return nopNotifier{} }
