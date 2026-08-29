package domain

import "github.com/google/uuid"

// RenderedMessage is one notification turned into the content a sender
// transmits.
//
// It is filled in two halves. A TemplateRenderer supplies Subject and Body,
// which are all a template can know; the dispatch cycle then stamps
// NotificationID, RecipientID and Channel from the outbox row, because routing
// is the row's property and not the template's. A renderer that sets the
// routing fields itself will have them overwritten.
//
// The recipient is an id, not an address. Resolving it — to a mailbox, a
// webhook URL, or to nothing at all because the row *is* the in-app
// notification — is the sender's job, and is the reason no address appears
// anywhere in this module.
//
// It lives in domain, not in application, for one reason: it is the type a
// sender must NAME to spell its own Send method, and senders live in
// infrastructure. R1 lets infrastructure import domain and forbids it importing
// application, so declaring this in application would have forced a choice
// between an unbuildable adapter and a weakened rule. This is the ordinary
// shape of an outbound port in this tree — NotificationRepository and
// PreferenceRepository are declared here and implemented in infrastructure for
// exactly the same reason. See E11 on the task board.
//
// Subject and Body are rendered presentation data, and domain never constructs
// a RenderedMessage. That is a real cost, accepted deliberately: the
// alternative was to let infrastructure import application, which no
// import-level rule can then distinguish from infrastructure calling a use
// case — the coupling R1 exists to prevent.
type RenderedMessage struct {
	NotificationID uuid.UUID
	RecipientID    uuid.UUID
	Channel        Channel

	// Subject is the message's one-line heading. Channels that have no notion
	// of one ignore it.
	Subject string

	// Body is the rendered content for Channel. One body per channel, not one
	// per MIME type: the renderer resolves a template per (key, channel), so a
	// channel that needs several representations is a channel that names
	// several templates.
	Body string
}
