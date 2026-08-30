package sender

import (
	"context"

	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// InApp delivers the in-app channel, which means it does nothing at all.
//
// There is nowhere to send to: the outbox row IS the in-app notification. It is
// already committed, the read model in transport serves it straight out of the
// notifications table, and the only thing left for a "delivery" to do is settle
// the row — which the dispatch cycle does when Send returns nil. sent therefore
// means "visible in the mailbox", and ReadAt, not Status, is what says whether
// anyone looked at it.
//
// A no-op with no state and no dependencies still has to exist, and this is the
// part worth remembering: the service builds its routing table from the senders
// it is given, dispatch dead-letters a channel with no sender, and Enqueue
// (E13) now refuses one outright. Without this file, an in-app notification is
// not merely undelivered — it cannot be queued, and the mailbox D7 exists to
// serve is permanently empty.
//
// Note that dispatch renders before it calls Send, so the rendered message
// arrives here and is discarded. That is not waste worth removing: it is a
// smoke test of every in-app template on every send, and it means a broken
// template dead-letters the row with a reason rather than putting a broken card
// in front of a user.
type InApp struct{}

// NewInApp returns the in-app sender. It takes nothing because it needs
// nothing.
func NewInApp() *InApp { return &InApp{} }

// Channel reports domain.ChannelInApp.
func (InApp) Channel() domain.Channel { return domain.ChannelInApp }

// Send succeeds without doing anything. See the type comment for why that is
// the correct implementation and not a stub awaiting one.
func (InApp) Send(_ context.Context, _ domain.RenderedMessage) error { return nil }
