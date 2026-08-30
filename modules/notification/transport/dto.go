// Package transport is the notification module's HTTP layer: the mailbox and
// preference endpoints, mounted behind the auth middleware the composition root
// supplies.
//
// There is deliberately no send endpoint. Enqueue is an internal capability
// other modules reach through an adapter at the composition root; exposing it
// over HTTP would let any authenticated caller mint mail addressed to anybody.
package transport

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/modules/notification/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// The wire types live here rather than in application, for the reason
// application/dto.go states from the other side: the use case's vocabulary and
// the API's contract are two different things, and a struct that is both cannot
// be changed on one side without changing the other. These carry the JSON tags;
// the application types carry none.

// NotificationResponse is one row of the caller's in-app mailbox.
//
// It is the projection of application.NotificationView and carries no delivery
// state: status, attempt count and last error are operational, and LastError in
// particular holds SMTP responses and internal host names.
//
// ReadAt is null while unread. Unread is left derived from it rather than
// duplicated into a bool, so a client cannot be shown two answers that
// disagree.
type NotificationResponse struct {
	ID          string         `json:"id"`
	Category    string         `json:"category"`
	TemplateKey string         `json:"template_key"`
	Payload     map[string]any `json:"payload"`
	CreatedAt   time.Time      `json:"created_at"`
	ReadAt      *time.Time     `json:"read_at"`
}

// ListNotificationsResponse is one page of the mailbox.
//
// Items is never null: a client rendering a list should not have to distinguish
// "no notifications" from "the field was omitted".
//
// NextCursor is present exactly when this page was filled to the limit, which
// is the only thing the query can know without fetching a row it was not asked
// for. So a next page may turn out to be empty — that is the honest answer for
// a keyset scan, and the alternative (over-fetching by one on every request to
// avoid one empty request at the end) costs more than it saves.
type ListNotificationsResponse struct {
	Items      []NotificationResponse `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

// MarkAllReadResponse reports how many rows the call changed, because that is
// the number a client needs to update an unread badge without re-listing.
type MarkAllReadResponse struct {
	MarkedRead int `json:"marked_read"`
}

// PreferenceResponse is one cell of the effective preference matrix: what the
// system will actually do for this pair, which is not always what the stored
// row says. A protected pair reports enabled however it is stored — see
// domain.Allowed.
type PreferenceResponse struct {
	Channel  string `json:"channel"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

// PreferencesResponse is the whole matrix: every channel crossed with every
// category, never a sparse subset. A caller then never has to know what an
// absent pair means.
type PreferencesResponse struct {
	Preferences []PreferenceResponse `json:"preferences"`
}

// UpdatePreferencesRequest is the body of PUT /notifications/preferences.
//
// It is a whole-request unit and not a patch per cell: the repository writes
// the slice together, so a request that is half-legal is refused whole rather
// than landing half-applied. Enabled is a pointer so that a member which omits
// it is a client error rather than a silent "disable this", which is the one
// direction of this API that can lose a security email.
type UpdatePreferencesRequest struct {
	Preferences []PreferenceUpdateRequest `json:"preferences"`
}

// PreferenceUpdateRequest is one requested change.
type PreferenceUpdateRequest struct {
	Channel  string `json:"channel"`
	Category string `json:"category"`
	Enabled  *bool  `json:"enabled"`
}

// toNotificationResponses projects a page of views onto the wire.
//
// The slice is allocated even when empty so that the JSON is [] and not null.
func toNotificationResponses(views []application.NotificationView) []NotificationResponse {
	out := make([]NotificationResponse, 0, len(views))
	for _, v := range views {
		out = append(out, NotificationResponse{
			ID:          v.ID.String(),
			Category:    string(v.Category),
			TemplateKey: v.TemplateKey,
			Payload:     v.Payload,
			CreatedAt:   v.CreatedAt,
			ReadAt:      v.ReadAt,
		})
	}
	return out
}

// toPreferencesResponse projects the effective matrix onto the wire.
func toPreferencesResponse(views []application.PreferenceView) PreferencesResponse {
	out := make([]PreferenceResponse, 0, len(views))
	for _, v := range views {
		out = append(out, PreferenceResponse{
			Channel:  string(v.Channel),
			Category: string(v.Category),
			Enabled:  v.Enabled,
		})
	}
	return PreferencesResponse{Preferences: out}
}

// toPreferenceUpdates projects the request onto the use case's vocabulary.
//
// It does not validate the channel or the category. domain.NewPreference does,
// inside UpdatePreferences, which is also where the protected-pair rule lives —
// a copy of either check here would be a second opinion that can drift from the
// one that actually decides. What this rejects is the one thing the domain
// cannot see: a member that never said what it wanted.
func toPreferenceUpdates(req UpdatePreferencesRequest) ([]application.PreferenceUpdate, error) {
	out := make([]application.PreferenceUpdate, 0, len(req.Preferences))
	for i, p := range req.Preferences {
		if p.Enabled == nil {
			return nil, fmt.Errorf("preferences[%d] is missing \"enabled\"", i)
		}
		out = append(out, application.PreferenceUpdate{
			Channel:  domain.Channel(p.Channel),
			Category: domain.Category(p.Category),
			Enabled:  *p.Enabled,
		})
	}
	return out, nil
}

// cursorPayload is what an encoded cursor carries: the sort key of the last row
// the caller has already seen, which is exactly domain.Cursor.
//
// The field names are one letter because they are never read by a human: the
// value is base64 and opaque by construction. See encodeCursor.
type cursorPayload struct {
	C time.Time `json:"c"`
	I uuid.UUID `json:"i"`
}

// encodeCursor renders a keyset position as an opaque token.
//
// Opaque, and not "created_at,id" in the clear, for two reasons that outlast
// this endpoint. The pair is an implementation detail of the ordering — the day
// the mailbox gains a second sort key, a client that parsed the old form breaks
// and one that echoed it back does not. And a token a caller cannot hand-craft
// is a token that cannot be used to probe: nothing here is ownership-scoped, but
// a seek position assembled by a client is one more input that has to be
// correct rather than merely well-formed.
//
// It is not signed. A forged cursor selects a window of the caller's OWN
// mailbox — the recipient predicate is in the query and no cursor can move it —
// so there is nothing for a signature to protect.
//
// RawURLEncoding: no padding to percent-escape, and the result is safe in a
// query string as-is.
func encodeCursor(c domain.Cursor) string {
	// A struct of a time and a uuid cannot fail to marshal, and there is no
	// half-written cursor to report: the error is unreachable.
	raw, _ := json.Marshal(cursorPayload{C: c.CreatedAt, I: c.ID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// decodeCursor parses a token produced by encodeCursor.
//
// Every failure is one failure to the caller — "that is not a cursor" — because
// the distinctions (bad base64, bad JSON, a uuid that is not one) are about the
// token's internals, which the client did not write and cannot fix.
func decodeCursor(s string) (*domain.Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errNotACursor
	}

	var p cursorPayload
	// DisallowUnknownFields is deliberately not set: a cursor issued by a newer
	// version of this service with an extra field should still seek, not 400.
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errNotACursor
	}
	if p.C.IsZero() || p.I == uuid.Nil {
		return nil, errNotACursor
	}

	return &domain.Cursor{CreatedAt: p.C, ID: p.I}, nil
}

// errNotACursor is the single reason decodeCursor reports. It is a value rather
// than a formatted string so the detail a client sees cannot accidentally start
// echoing the token back.
var errNotACursor = badRequest("the cursor parameter is not a cursor this service issued")

// badRequest is a client error whose message is safe to show. Everything this
// package answers 400 for is a parameter the caller wrote, so naming it is
// help, not disclosure.
type badRequest string

func (e badRequest) Error() string { return string(e) }

// parseListFilter turns the query string into a domain.ListFilter.
//
// It returns the filter already Normalized, which the repository will do again
// — Normalize is documented idempotent, and this layer needs the effective
// limit before the call in order to decide whether the page it gets back is
// full. Deriving that from the raw parameter instead would emit a next_cursor
// for a page of 20 that the caller asked to be 500 long.
func parseListFilter(q url.Values) (domain.ListFilter, error) {
	var f domain.ListFilter

	if raw := q.Get("unread"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return f, badRequest("the unread parameter must be true or false")
		}
		f.UnreadOnly = v
	}

	if raw := q.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			// Refused rather than defaulted. Normalize reads a non-positive
			// limit as "unspecified", which is right for a caller that said
			// nothing and wrong for one that said 0 — that caller has a bug,
			// and silently serving them 20 rows hides it.
			//
			// A limit ABOVE the maximum is not refused: Normalize caps it, and
			// the cap is a property of the service rather than a mistake by the
			// caller. The bound is documented; the answer is the largest page
			// this service will serve.
			return f, badRequest(fmt.Sprintf(
				"the limit parameter must be a positive integer (at most %d)", domain.MaxListLimit))
		}
		f.Limit = v
	}

	if raw := q.Get("cursor"); raw != "" {
		before, err := decodeCursor(raw)
		if err != nil {
			return f, err
		}
		f.Before = before
	}

	return f.Normalize(), nil
}
