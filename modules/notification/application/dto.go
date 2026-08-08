package application

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// The types here carry data across the application boundary. They have no JSON
// tags: the wire shape is transport's to choose, and a struct that is both the
// use case's vocabulary and the API's contract cannot be changed on one side
// without changing the other.

// EnqueueRequest is the intent to notify one user.
//
// It carries no id and no timestamps. The outbox row's id is an internal
// detail the caller has no use for, and the only handle a caller needs on a
// past enqueue is IdempotencyKey.
type EnqueueRequest struct {
	RecipientID uuid.UUID
	Channel     domain.Channel
	Category    domain.Category

	// TemplateKey names the template family to render at dispatch, e.g.
	// "security.password_changed".
	TemplateKey string

	// Payload is template data. A nil map is fine and becomes an empty one.
	Payload map[string]any

	// IdempotencyKey deduplicates enqueues of the same business event — a
	// retried request, a replayed message. Empty means no deduplication. A key
	// already in the outbox makes Enqueue a successful no-op, so the natural
	// form is one that is derivable from the event itself, e.g.
	// "password_changed:<userID>:<eventID>".
	IdempotencyKey string
}

// NotificationView is one row of a user's in-app mailbox.
//
// There is no Channel field because the mailbox is in-app by definition, and no
// Status: delivery state is an operational concern, and LastError in particular
// carries SMTP responses and internal host names that are never shown to a
// user.
//
// ID and CreatedAt are together the keyset cursor (domain.Cursor), so a caller
// paging through the list builds the next page's position from the last view it
// received without needing a second type.
type NotificationView struct {
	ID          uuid.UUID
	Category    domain.Category
	TemplateKey string
	Payload     map[string]any
	CreatedAt   time.Time

	// ReadAt is nil while the notification is unread. Unread is derived from
	// this rather than duplicated into a bool, so the two cannot disagree.
	ReadAt *time.Time
}

// PreferenceView is one cell of the effective preference matrix: what the
// system will actually do for a (channel, category) pair, which is not always
// what the stored rows say. See Service.GetPreferences.
type PreferenceView struct {
	Channel  domain.Channel
	Category domain.Category
	Enabled  bool
}

// PreferenceUpdate is one requested change to a preference.
//
// It is a separate type from PreferenceView with the same fields on purpose:
// the view is an answer about effective behaviour, this is a request about a
// stored row, and the two differ precisely where it matters — a request to
// disable a protected pair is refused, while the view of that pair reports the
// truth that it is still enabled.
type PreferenceUpdate struct {
	Channel  domain.Channel
	Category domain.Category
	Enabled  bool
}
