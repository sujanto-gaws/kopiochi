package domain

import "errors"

// The module's sentinel errors. Callers branch with errors.Is; the constructors
// and transitions wrap these with the offending value so an operator reading a
// log line learns what was rejected without the caller having to build the
// message itself.
var (
	// ErrNotFound reports that a notification or preference row the caller
	// named does not exist — or does not belong to the caller, which the
	// repositories deliberately make indistinguishable. Telling a user that a
	// notification exists but is somebody else's leaks the existence of the
	// row.
	ErrNotFound = errors.New("notification: not found")

	// ErrInvalidTransition reports a status change outside the state machine
	// in notification.go. Every rejected move leaves the entity untouched.
	ErrInvalidTransition = errors.New("notification: invalid status transition")

	// ErrDuplicateIdempotencyKey reports that an enqueue carried an
	// idempotency key that is already in the outbox, so nothing was inserted.
	//
	// It is an error at the repository boundary and a success at the use-case
	// boundary: "this notification was already queued" is exactly the outcome
	// the caller asked for. The application layer swallows it; it exists as a
	// distinct value so that swallowing is a deliberate line of code rather
	// than a silent no-op nobody can observe.
	ErrDuplicateIdempotencyKey = errors.New("notification: duplicate idempotency key")

	// ErrInvalidNotification reports that a notification failed construction
	// validation — unknown channel or category, missing id, recipient or
	// template key. Wrapped with the specific field.
	ErrInvalidNotification = errors.New("notification: invalid")

	// ErrInvalidPreference reports that a preference names an unknown channel,
	// an unknown category, or no user.
	ErrInvalidPreference = errors.New("notification: invalid preference")

	// ErrPreferenceProtected reports an attempt to disable a channel/category
	// combination that is not the user's to disable — see protected() in
	// preference.go.
	ErrPreferenceProtected = errors.New("notification: preference is protected and cannot be disabled")

	// ErrNonRetryable marks a delivery failure that another attempt cannot fix,
	// and is the whole of the error contract between the dispatch cycle and the
	// senders in infrastructure.
	//
	// The classification has one rule and one default:
	//
	//   - A sender error that wraps this sentinel settles the row as dead
	//     immediately, with the error text kept as LastError for the operator.
	//   - Anything else is retryable, and the row goes back to pending with the
	//     backoff schedule until its attempt budget runs out.
	//
	// Retryable is the default because the two mistakes are not symmetric.
	// Misclassifying a permanent failure as retryable wastes a bounded number of
	// attempts and still ends at dead; misclassifying a transient one as
	// permanent destroys the notification on the first network blip — and the
	// notification most likely to be sent during an outage is a security mail.
	//
	// A sender opts out by wrapping:
	//
	//	return fmt.Errorf("%w: smtp 550 mailbox unavailable", domain.ErrNonRetryable)
	//
	// A sentinel plus errors.Is rather than an interface with a Retryable() bool
	// method: wrapping composes with %w through every layer an SMTP or HTTP
	// client puts in the way, and it needs no new type in either package.
	//
	// It is declared here rather than in application because it decides a
	// DOMAIN state transition — dead versus pending — and because a sender must
	// reference it, and senders may not import application (R1). See E11.
	ErrNonRetryable = errors.New("notification: non-retryable delivery failure")
)
