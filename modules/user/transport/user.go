package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/sujanto-gaws/kopiochi/internal/authn"
	"github.com/sujanto-gaws/kopiochi/internal/httpx"
	domain "github.com/sujanto-gaws/kopiochi/modules/user/domain"
)

// UserService is the set of application operations UserHandler depends on.
//
// Both take the caller and nothing else. Before E16 this interface took an id
// from the URL, which is what made the IDOR expressible: the handler could name
// a row that was not the caller's, and nothing stopped it. There is no longer
// an argument to get wrong.
type UserService interface {
	EnsureOwnProfile(ctx context.Context, caller uuid.UUID) (*domain.UserResponse, error)
	GetOwnProfile(ctx context.Context, caller uuid.UUID) (*domain.UserResponse, error)
}

// UserHandler handles HTTP requests for profile operations.
type UserHandler struct {
	svc    UserService
	authMW authn.Middleware
}

// NewUserHandler creates a new user handler. authMW protects every route this
// handler exposes — all profile routes require authentication — and is injected
// by the composition root rather than resolved by the handler itself, so a
// missing/misconfigured verifier fails at wiring time (see cmd/api/container.go)
// instead of the handler silently serving unprotected routes.
func NewUserHandler(svc UserService, authMW authn.Middleware) *UserHandler {
	return &UserHandler{svc: svc, authMW: authMW}
}

// caller extracts the authenticated subject as a uuid.
//
// It reads the Principal the auth middleware put in the context, which since
// E16 is the ONLY source of the id these handlers act on. A malformed subject
// is a 401 and not a 400: the token verified but does not identify anybody this
// service can act for, and that is an authentication problem, not the client's
// request being wrong.
//
// The 401 itself is httpx.Unauthorized, not a hand-rolled body. This package
// answered it with writeJSON(w, 401, {"error": ...}) until now — the only place
// in the tree where an authentication failure did not look like the others, and
// written by the same change that closed E16, which was busy with the ownership
// hole and hand-rolled the one response shape A3 exists to make uniform.
//
// It was unreachable through this issuer: every "sub" this service signs is
// user.ID.String() on a uuid column, so a non-uuid subject needs the private key.
// It stops being unreachable the moment a second middleware exists, which is the
// entire premise of internal/authn — a real OIDC subject is not a uuid (Google's
// is numeric, Auth0's is auth0|abc123), so this becomes the COMMON path for every
// federated caller on the day one is mounted.
func caller(r *http.Request) (uuid.UUID, bool) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(p.Subject)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// EnsureOwnProfile handles POST /users/me
// @Summary Create the caller's profile
// @Description Creates the authenticated caller's own profile if it does not
// @Description already exist, and returns it either way. Takes no body and no
// @Description id: the profile created is always the caller's.
// @Tags users
// @Produce json
// @Success 200 {object} domain.UserResponse "The caller's profile"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/me [post]
func (h *UserHandler) EnsureOwnProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		resp, err := h.svc.EnsureOwnProfile(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to create profile"))
			return
		}

		// 200 rather than 201, because the call is idempotent and the second
		// request did not create anything. A client cannot tell which of its
		// requests won a race, so it is told the same thing either way.
		writeJSON(w, http.StatusOK, resp)
	}
}

// GetOwnProfile handles GET /users/me
// @Summary Get the caller's profile
// @Description Returns the authenticated caller's own profile. There is no
// @Description route that returns anybody else's.
// @Tags users
// @Produce json
// @Success 200 {object} domain.UserResponse "The caller's profile"
// @Failure 401 {object} map[string]string "Not authenticated"
// @Failure 404 {object} map[string]string "No profile yet"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/me [get]
func (h *UserHandler) GetOwnProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := caller(r)
		if !ok {
			httpx.Unauthorized(w, r)
			return
		}

		resp, err := h.svc.GetOwnProfile(r.Context(), id)
		if err != nil {
			if errors.Is(err, domain.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse("user not found"))
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to fetch profile"))
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// Routes mounts this handler's endpoints onto r, behind the auth middleware.
//
// Two routes, both /users/me, and NO route carrying an id. That is how E16 is
// closed: the IDOR was not an absent ownership check but an addressable id with
// nothing to compare it against, and an endpoint that takes no id cannot be
// asked for somebody else's row. It also removes E16's second leg — the
// unrestricted POST /users that minted a profile from any valid token.
//
// The enumeration oracle E16-P and E16-P2 recorded (200 vs 404 on GET, 200 vs
// 404 on PUT, 204 vs 404 on DELETE) is gone with the verbs that produced it.
// Byte-identity between a cross-user answer and a not-found answer is moot when
// there is no way to phrase a cross-user request.
func (h *UserHandler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authMW)
		r.Post("/users/me", h.EnsureOwnProfile())
		r.Get("/users/me", h.GetOwnProfile())
	})
}
