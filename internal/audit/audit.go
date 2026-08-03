// Package audit emits security-relevant events to a stream separate from the
// request log.
//
// # Why a separate stream
//
// Request logs are high-volume, sampled in many deployments, and rotated on a
// short retention. Audit events are the opposite: rare, never sampled, and the
// thing an incident review reads six months later. Mixing them means the
// interesting records are diluted by four orders of magnitude of routine
// traffic and expire before anyone looks.
//
// Every event carries the marker field `audit: true`, so a log pipeline can
// route on it without parsing the action vocabulary.
//
// # What must never be in an event
//
// Tokens, token hashes, passwords, secrets and full Authorization headers. An
// audit stream is retained longer and read by more people than the request
// log, which makes a leak into it worse, not better. Identify a token by its
// family or its jti — never by its value.
package audit

import (
	"context"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// Action is the event vocabulary. Fixed strings, not free text: these are
// grepped, alerted on and counted, and an interpolated message would make
// every one of those unreliable.
type Action string

const (
	ActionLoginSucceeded Action = "auth.login.success"
	ActionLoginFailed    Action = "auth.login.failure"
	ActionAccountLocked  Action = "auth.account.locked"
	ActionLogout         Action = "auth.logout"

	ActionMFAEnrolled Action = "auth.mfa.enrolled"
	ActionMFAFailed   Action = "auth.mfa.failed"

	// ActionRefreshReuseDetected is the highest-severity event this package
	// defines: a refresh token was presented twice, which means two parties
	// hold credentials from one login.
	ActionRefreshReuseDetected Action = "auth.token.refresh_reuse_detected"
)

// Outcome is whether the attempt succeeded. Kept separate from Action so a
// dashboard can count failures per action without string matching.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
)

// Event is one audit record.
type Event struct {
	Action  Action
	Outcome Outcome

	// ActorID is who did it, where known. For a failed login there is no
	// authenticated actor, so Subject carries the attempted identifier
	// instead.
	ActorID string
	// TargetID is what was acted on, when different from the actor — a role
	// grant, an admin deleting someone else's account.
	TargetID string
	// Subject is an unauthenticated identifier, typically an attempted
	// username. Separate from ActorID so a reader cannot mistake an attempt
	// for a confirmed identity.
	Subject string
	// Reason is a short fixed code (bad_password, expired_token, wrong_class),
	// never an error message.
	Reason string
	// Fields carries event-specific context. Values must satisfy the "never
	// log" rule in the package doc.
	Fields map[string]any
}

// Logger emits audit events.
type Logger struct {
	log zerolog.Logger
}

// New builds an audit logger over the given base logger.
//
// It takes a logger rather than reaching for the zerolog global for the same
// reason everything else in this tree does: a test needs to assert that a
// security event was actually emitted, and a global cannot be swapped.
func New(base zerolog.Logger) *Logger {
	return &Logger{log: base.With().Bool("audit", true).Logger()}
}

// Emit writes the event.
//
// It always logs at warn or above. An audit record filtered out by a level
// threshold set for request logs is an audit record that does not exist, and
// the levels here are chosen so that cannot happen at any sane configuration:
// failures are warnings, and reuse detection is an error.
func (l *Logger) Emit(ctx context.Context, e Event) {
	if l == nil {
		return
	}

	evt := l.log.Warn()
	if e.Action == ActionRefreshReuseDetected {
		evt = l.log.Error()
	}

	// Correlate with the request that produced it. The audit record and the
	// access-log line for the same request then share an id, which is what
	// makes "what else was this client doing" answerable.
	if id := chimw.GetReqID(ctx); id != "" {
		evt = evt.Str("request_id", id)
	}

	evt = evt.
		Str("action", string(e.Action)).
		Str("outcome", string(e.Outcome))

	if e.ActorID != "" {
		evt = evt.Str("actor_id", e.ActorID)
	}
	if e.TargetID != "" {
		evt = evt.Str("target_id", e.TargetID)
	}
	if e.Subject != "" {
		evt = evt.Str("subject", e.Subject)
	}
	if e.Reason != "" {
		evt = evt.Str("reason", e.Reason)
	}
	for k, v := range e.Fields {
		evt = evt.Interface(k, v)
	}

	evt.Msg(string(e.Action))
}
