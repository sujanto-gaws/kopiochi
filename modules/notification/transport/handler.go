package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	"github.com/sujanto-gaws/kopiochi/modules/notification/application"
	domain "github.com/sujanto-gaws/kopiochi/modules/notification/domain"
)

// Service is the set of application operations Handler depends on.
//
// EVERY METHOD TAKES THE CALLER FIRST, and none of them takes an owner from
// anywhere else. That is R5 — ownership is a query shape, not a check — and it
// is why this interface has no method the handler could call with somebody
// else's id and get an answer. The recipient predicate is in the SQL
// (ListForRecipient, MarkRead, MarkAllRead), so there is no branch here that
// decides whether the caller may proceed, and therefore no branch to forget.
type Service interface {
	ListForUser(ctx context.Context, userID uuid.UUID, f domain.ListFilter) ([]application.NotificationView, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) (int, error)
	GetPreferences(ctx context.Context, userID uuid.UUID) ([]application.PreferenceView, error)
	UpdatePreferences(ctx context.Context, userID uuid.UUID, updates []application.PreferenceUpdate) ([]application.PreferenceView, error)
}

// Handler serves the caller's mailbox and preferences.
type Handler struct {
	svc    Service
	authMW authn.Middleware
}

// NewHandler creates the notification handler. authMW protects every route it
// exposes and is injected by the composition root rather than resolved here, so
// a missing verifier fails at wiring time instead of leaving routes served
// unauthenticated.
//
// It does not reject a nil middleware, and that is not an omission: Config.
// Validate does, inside notification.New, which is the same place user.New
// refuses it. One gate rather than two that can disagree about what "required"
// means.
func NewHandler(svc Service, authMW authn.Middleware) *Handler {
	return &Handler{svc: svc, authMW: authMW}
}

// caller extracts the authenticated subject as a uuid.
//
// MustFromContext and not FromContext: Routes mounts every handler inside the
// group guarded by h.authMW, so an absent Principal is a wiring mistake and not
// a runtime condition. The panic is the point — the alternative is a handler
// that treats "" as an account id and reads a mailbox belonging to nobody.
//
// A subject that is not a uuid is a 401 and not a 400: the credential verified
// but does not identify an account this module can act for. That is an
// authentication problem, not the client's request being wrong, and it is
// answered with the canonical 401 so it is indistinguishable from every other
// rejection.
func caller(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(authn.MustFromContext(r.Context()).Subject)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// List handles GET /notifications
// @Summary List the caller's in-app notifications
// @Description Returns one page of the authenticated caller's in-app mailbox,
// @Description newest first. Pagination is keyset: pass the next_cursor of the
// @Description previous response back as `cursor`. There is no route that
// @Description returns anybody else's mailbox — the recipient is the caller.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param unread query bool false "Only notifications that have not been read"
// @Param limit query int false "Page size (1-100, default 20)"
// @Param cursor query string false "Opaque position from a previous response"
// @Success 200 {object} transport.ListNotificationsResponse "One page of the mailbox"
// @Failure 400 {object} internal_httpx.Problem "Malformed query parameter"
// @Failure 401 {object} internal_httpx.Problem "Not authenticated"
// @Failure 500 {object} internal_httpx.Problem "Internal server error"
// @Router /notifications [get]
func (h *Handler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		f, err := parseListFilter(r.URL.Query())
		if err != nil {
			writeBadRequest(w, r, err)
			return
		}

		views, err := h.svc.ListForUser(r.Context(), id, f)
		if err != nil {
			writeInternal(w, r, "Could not read the mailbox.")
			return
		}

		resp := ListNotificationsResponse{Items: toNotificationResponses(views)}
		// A full page means there may be more. The cursor is built from the
		// last row the caller received, so the next request seeks from exactly
		// where this one stopped whatever is inserted meanwhile — which is the
		// whole reason this is a keyset and not an offset.
		if len(views) == f.Limit {
			last := views[len(views)-1]
			resp.NextCursor = encodeCursor(domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// MarkRead handles POST /notifications/{id}/read
// @Summary Mark one notification read
// @Description Marks one of the caller's own notifications read. Marking an
// @Description already-read notification is a successful no-op. An id that is
// @Description not the caller's is answered exactly as an id that does not
// @Description exist.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param id path string true "Notification id"
// @Success 204 "Marked read"
// @Failure 401 {object} internal_httpx.Problem "Not authenticated"
// @Failure 404 {object} internal_httpx.Problem "No such notification"
// @Failure 500 {object} internal_httpx.Problem "Internal server error"
// @Router /notifications/{id}/read [post]
func (h *Handler) MarkRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recipient, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		// A malformed id gets the same answer as a well-formed one that is not
		// the caller's, and as one that does not exist. Three inputs, one
		// response, one code path — which is the only construction that cannot
		// drift into an oracle. A 400 here would be defensible in isolation and
		// would still add a second shape to a route whose entire security
		// property is that it has one.
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeNotFound(w, r)
			return
		}

		// The caller is the recipient the query is scoped by. It comes from the
		// Principal and from nowhere else: there is no second path segment, no
		// body, and no query parameter that could name a different recipient.
		// See Service.
		if err := h.svc.MarkRead(r.Context(), recipient, id); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				// The repository returns ErrNotFound for "no such row" and for
				// "somebody else's row" without distinguishing them, so this
				// branch cannot tell which happened even if it wanted to. That
				// is deliberate: existence is information, and a 403 here would
				// confirm the id for anyone who guessed it.
				writeNotFound(w, r)
				return
			}
			writeInternal(w, r, "Could not mark the notification read.")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// MarkAllRead handles POST /notifications/read-all
// @Summary Mark every unread notification read
// @Description Marks every unread in-app notification of the authenticated
// @Description caller read, and reports how many rows changed.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} transport.MarkAllReadResponse "How many rows changed"
// @Failure 401 {object} internal_httpx.Problem "Not authenticated"
// @Failure 500 {object} internal_httpx.Problem "Internal server error"
// @Router /notifications/read-all [post]
func (h *Handler) MarkAllRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		n, err := h.svc.MarkAllRead(r.Context(), id)
		if err != nil {
			writeInternal(w, r, "Could not mark the notifications read.")
			return
		}

		writeJSON(w, http.StatusOK, MarkAllReadResponse{MarkedRead: n})
	}
}

// GetPreferences handles GET /notifications/preferences
// @Summary Get the caller's effective preference matrix
// @Description Returns every channel/category pair with the answer the system
// @Description will actually give at enqueue time — including the protected
// @Description pairs, which report enabled whatever is stored for them.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} transport.PreferencesResponse "The effective matrix"
// @Failure 401 {object} internal_httpx.Problem "Not authenticated"
// @Failure 500 {object} internal_httpx.Problem "Internal server error"
// @Router /notifications/preferences [get]
func (h *Handler) GetPreferences() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		views, err := h.svc.GetPreferences(r.Context(), id)
		if err != nil {
			writeInternal(w, r, "Could not read the delivery preferences.")
			return
		}

		writeJSON(w, http.StatusOK, toPreferencesResponse(views))
	}
}

// UpdatePreferences handles PUT /notifications/preferences
// @Summary Update the caller's delivery preferences
// @Description Applies every change or none, and returns the resulting
// @Description effective matrix. Disabling a protected pair — security
// @Description notifications by email — is refused with 422: preferences filter
// @Description at enqueue time, so a disabled pair means "your password was
// @Description changed" is never written at all.
// @Tags notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body transport.UpdatePreferencesRequest true "The changes to apply"
// @Success 200 {object} transport.PreferencesResponse "The resulting effective matrix"
// @Failure 400 {object} internal_httpx.Problem "Malformed body"
// @Failure 401 {object} internal_httpx.Problem "Not authenticated"
// @Failure 422 {object} internal_httpx.Problem "Unknown pair, or a protected pair the caller may not disable"
// @Failure 500 {object} internal_httpx.Problem "Internal server error"
// @Router /notifications/preferences [put]
func (h *Handler) UpdatePreferences() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		var req UpdatePreferencesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// 400 and not 422: the bytes are not JSON, so there is no content
			// to have semantics about. The decoder's message is not forwarded —
			// it quotes the body back, and a body echoed into an error is a
			// reflection primitive nobody asked for.
			writeBadRequest(w, r, badRequest("the request body is not valid JSON"))
			return
		}

		updates, err := toPreferenceUpdates(req)
		if err != nil {
			writeBadRequest(w, r, err)
			return
		}

		views, err := h.svc.UpdatePreferences(r.Context(), id, updates)
		if err != nil {
			writePreferenceError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, toPreferencesResponse(views))
	}
}

// writePreferenceError maps the two refusals UpdatePreferences can produce.
//
// Both are 422 and not 400: the body parsed, and what is wrong with it is what
// it means. They carry different types so a client can tell "that is not a
// channel" from "that switch is not yours to turn off" — the first is a bug in
// the caller and the second is a rule it must explain to a user. Neither is an
// oracle: preferences are not ownership-scoped, and the protected pair is a
// property of the system that this same API already reports in every matrix.
func writePreferenceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrPreferenceProtected):
		httpx.WriteProblem(w, r, http.StatusUnprocessableEntity,
			"preference_protected", "Preference Is Protected",
			"Security notifications by email cannot be switched off.")
	case errors.Is(err, domain.ErrInvalidPreference):
		httpx.WriteProblem(w, r, http.StatusUnprocessableEntity,
			"invalid_preference", "Invalid Preference",
			"A preference names an unknown channel or category, or the same pair twice.")
	default:
		writeInternal(w, r, "Could not save the delivery preferences.")
	}
}

// writeNotFound is the ONLY not-found this package emits, and it takes no
// argument, so there is nothing for a caller to vary. A cross-user id, an
// absent id and an unparsable id all arrive here and leave with the same
// status, the same headers and the same body — differing only in `instance`,
// which is the path the client itself wrote.
//
// The type slug matches internal/httpx's own 404 so a client sees one
// vocabulary; the detail differs because the questions do — this route matched,
// the row did not.
func writeNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, http.StatusNotFound,
		"not_found", "Not Found",
		"No such notification.")
}

// writeBadRequest reports a parameter the caller wrote. err's message is shown,
// which is safe by construction: badRequest is the only type this package
// creates for it, and every value of it is a fixed string about a parameter
// name.
func writeBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	// Unreachable through the current call sites, and cheap insurance against a
	// future one: an error of unknown provenance is summarised, never
	// forwarded.
	safe := badRequest("the request could not be understood")
	_ = errors.As(err, &safe)

	httpx.WriteProblem(w, r, http.StatusBadRequest,
		"invalid_request", "Bad Request", string(safe))
}

// writeInternal answers a failure the caller cannot do anything about. detail is
// a fixed sentence and never the error: what an operator needs is in the log
// line, keyed by the request id this response already carries.
func writeInternal(w http.ResponseWriter, r *http.Request, detail string) {
	httpx.WriteProblem(w, r, http.StatusInternalServerError,
		"internal_error", "Internal Server Error", detail)
}

// writeJSON writes a success body. Errors do not come through here — they go to
// httpx.WriteProblem, which is the one problem+json writer in the tree.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		// Status and headers are committed; a failed encode leaves a truncated
		// body and nothing actionable beyond not pretending we checked.
		_ = json.NewEncoder(w).Encode(v)
	}
}
