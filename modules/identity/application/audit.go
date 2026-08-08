package application

import "context"

// Auditor is the set of security events this module emits.
//
// Declared here, as a narrow interface of exactly what the identity module
// needs, rather than the module importing a shared audit type: the composition
// root supplies the implementation, and a test supplies a recorder. Depending
// on a concrete logger would make "was this event emitted?" untestable, and
// that assertion is the whole point of an audit trail.
type Auditor interface {
	// LoginSucceeded records a completed authentication.
	LoginSucceeded(ctx context.Context, userID string)
	// LoginFailed records a rejected attempt. username is what was *tried*,
	// not a confirmed identity, and reason is a fixed code.
	LoginFailed(ctx context.Context, username, reason string)
	// AccountLocked records the lockout threshold being reached.
	AccountLocked(ctx context.Context, userID string)
	// LogoutSucceeded records a user-initiated session end.
	LogoutSucceeded(ctx context.Context, userID string)
	// MFAEnrolled records a second factor being activated.
	MFAEnrolled(ctx context.Context, userID string)
	// MFAFailed records a rejected second factor.
	MFAFailed(ctx context.Context, userID, reason string)
	// RefreshReuseDetected records a refresh token presented twice — the
	// highest-severity event this module produces. revoked is how many tokens
	// in the family were still live; err is non-nil if the revocation itself
	// failed, which is worse still and must not be silent.
	RefreshReuseDetected(ctx context.Context, userID, familyID string, revoked int, err error)
}

// Reasons passed to LoginFailed and MFAFailed. Fixed codes rather than error
// strings: these are counted and alerted on, and an interpolated message makes
// both unreliable — and risks putting an input into a retained stream.
const (
	ReasonBadPassword   = "bad_password"
	ReasonUnknownUser   = "unknown_user"
	ReasonAccountLocked = "account_locked"
	ReasonInvalidCode   = "invalid_code"
	ReasonInvalidToken  = "invalid_token"
)

// nopAuditor discards every event.
//
// It exists so NewService can never hold a nil Auditor: a nil interface would
// panic at exactly the moment a security event needed recording, turning a
// detected token theft into a 500. Callers that genuinely want no audit trail
// say so explicitly by passing this.
type nopAuditor struct{}

func (nopAuditor) LoginSucceeded(context.Context, string)                           {}
func (nopAuditor) LoginFailed(context.Context, string, string)                      {}
func (nopAuditor) AccountLocked(context.Context, string)                            {}
func (nopAuditor) LogoutSucceeded(context.Context, string)                          {}
func (nopAuditor) MFAEnrolled(context.Context, string)                              {}
func (nopAuditor) MFAFailed(context.Context, string, string)                        {}
func (nopAuditor) RefreshReuseDetected(context.Context, string, string, int, error) {}

// NopAuditor returns an Auditor that discards everything.
func NopAuditor() Auditor { return nopAuditor{} }
