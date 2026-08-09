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
)
