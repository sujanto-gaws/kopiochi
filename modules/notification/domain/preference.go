package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Preference is one user's decision about one (channel, category) pair.
//
// Rows are sparse: only pairs a user has actually changed are stored. Absence
// is not "unknown", it is enabled — see Allowed.
type Preference struct {
	UserID   uuid.UUID
	Channel  Channel
	Category Category
	Enabled  bool

	UpdatedAt time.Time
}

// protected reports whether a (channel, category) pair is outside the user's
// control.
//
// Exactly one pair is: security notifications by email. Preferences are a
// filter applied at enqueue time, so a disabled pair means the row is never
// written at all — and "your password was changed" is precisely the mail whose
// absence an attacker benefits from. Everything else, including security
// notifications in the in-app mailbox, is the user's to silence.
//
// This is a rule about the pair, not about a stored row, which is why it is
// checked in three places: when a preference is constructed, when one is
// flipped, and again when the effective value is resolved. A row that somehow
// exists with Enabled=false for the protected pair — written before this rule,
// or by hand in psql — still cannot suppress the mail.
func protected(ch Channel, cat Category) bool {
	return ch == ChannelEmail && cat == CategorySecurity
}

// NewPreference builds a validated preference.
//
// It is the only validating constructor, not the only one: the fields are
// exported — infrastructure has to hydrate a stored row into the struct — so a
// composite literal from any importing package assembles a Preference without
// passing through here, and the tests in this package do exactly that.
//
// So this is a first line of defence and not the guarantee. What actually makes
// the protected-pair rule hold is that Allowed re-checks it when the effective
// value is resolved, whatever the struct in front of it says. See protected.
func NewPreference(userID uuid.UUID, ch Channel, cat Category, enabled bool, now time.Time) (Preference, error) {
	if userID == uuid.Nil {
		return Preference{}, fmt.Errorf("%w: user id is required", ErrInvalidPreference)
	}
	if !ch.Valid() {
		return Preference{}, fmt.Errorf("%w: unknown channel %q", ErrInvalidPreference, ch)
	}
	if !cat.Valid() {
		return Preference{}, fmt.Errorf("%w: unknown category %q", ErrInvalidPreference, cat)
	}
	if !enabled && protected(ch, cat) {
		return Preference{}, fmt.Errorf("%w: %s/%s", ErrPreferenceProtected, ch, cat)
	}

	return Preference{
		UserID:    userID,
		Channel:   ch,
		Category:  cat,
		Enabled:   enabled,
		UpdatedAt: now,
	}, nil
}

// SetEnabled flips an existing preference, refusing to disable a protected
// pair and leaving the entity untouched when it refuses.
//
// Re-enabling a protected pair is allowed and is a no-op in effect — it is
// already on — but rejecting it would make an idempotent "turn everything on"
// request fail.
func (p *Preference) SetEnabled(enabled bool, now time.Time) error {
	if !enabled && protected(p.Channel, p.Category) {
		return fmt.Errorf("%w: %s/%s", ErrPreferenceProtected, p.Channel, p.Category)
	}
	p.Enabled = enabled
	p.UpdatedAt = now
	return nil
}

// Allowed reports whether a notification on ch/cat may be enqueued for the user
// whose stored preferences are prefs.
//
// Two defaults, both deliberate:
//
//   - Absence means enabled. A user who has never opened the preferences page
//     receives everything; the alternative — absence means disabled — would
//     silently mute every user the day the feature ships. This is the kind of
//     default that inverts unnoticed during a refactor, which is why it has a
//     test of its own.
//   - A protected pair is always allowed, whatever the stored row says. See
//     protected.
//
// prefs is scanned linearly because it holds at most one row per
// channel/category pair — nine today — and a map would cost more to build than
// the scan saves.
func Allowed(prefs []Preference, ch Channel, cat Category) bool {
	if protected(ch, cat) {
		return true
	}
	for _, p := range prefs {
		if p.Channel == ch && p.Category == cat {
			return p.Enabled
		}
	}
	return true
}
