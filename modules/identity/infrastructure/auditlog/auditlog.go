// Package auditlog adapts internal/audit to the identity module's Auditor
// port.
//
// The adapter lives on the module side of the boundary because it is the
// module that decides which of its events matter and what to call them.
// internal/audit only knows how to write a record.
package auditlog

import (
	"context"

	"github.com/sujanto-gaws/kopiochi/internal/audit"
)

// Auditor implements the identity module's application.Auditor over an audit
// logger.
type Auditor struct {
	log *audit.Logger
}

// New wraps an audit logger.
func New(log *audit.Logger) *Auditor { return &Auditor{log: log} }

func (a *Auditor) LoginSucceeded(ctx context.Context, userID string) {
	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionLoginSucceeded,
		Outcome: audit.OutcomeSuccess,
		ActorID: userID,
	})
}

// LoginFailed records the attempted username under Subject, never ActorID:
// nobody authenticated, and a reader must not mistake an attempt for a
// confirmed identity.
func (a *Auditor) LoginFailed(ctx context.Context, username, reason string) {
	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionLoginFailed,
		Outcome: audit.OutcomeFailure,
		Subject: username,
		Reason:  reason,
	})
}

func (a *Auditor) AccountLocked(ctx context.Context, userID string) {
	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionAccountLocked,
		Outcome: audit.OutcomeFailure,
		ActorID: userID,
	})
}

func (a *Auditor) LogoutSucceeded(ctx context.Context, userID string) {
	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionLogout,
		Outcome: audit.OutcomeSuccess,
		ActorID: userID,
	})
}

func (a *Auditor) MFAEnrolled(ctx context.Context, userID string) {
	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionMFAEnrolled,
		Outcome: audit.OutcomeSuccess,
		ActorID: userID,
	})
}

func (a *Auditor) MFAFailed(ctx context.Context, userID, reason string) {
	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionMFAFailed,
		Outcome: audit.OutcomeFailure,
		ActorID: userID,
		Reason:  reason,
	})
}

// RefreshReuseDetected reports the family id, never the token or its hash — an
// audit stream is retained longer and read by more people than a request log,
// so a credential leaked into it is worse, not better.
//
// revocation_failed is surfaced as its own field because a detection whose
// revocation did not land is the one case where the attacker still holds a
// working session, and it must not be indistinguishable from a clean one.
func (a *Auditor) RefreshReuseDetected(ctx context.Context, userID, familyID string, revoked int, err error) {
	fields := map[string]any{
		"family_id":      familyID,
		"tokens_revoked": revoked,
	}
	if err != nil {
		fields["revocation_failed"] = true
		fields["revocation_error"] = err.Error()
	}

	a.log.Emit(ctx, audit.Event{
		Action:  audit.ActionRefreshReuseDetected,
		Outcome: audit.OutcomeFailure,
		ActorID: userID,
		Fields:  fields,
	})
}
