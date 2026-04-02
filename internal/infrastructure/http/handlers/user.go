package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/sujanto-gaws/kopiochi/internal/application/user"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	svc *user.Service
}

// NewUserHandler creates a new user handler
func NewUserHandler(svc *user.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

// CreateUser handles POST /users
func (h *UserHandler) CreateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		u, err := h.svc.CreateUser(r.Context(), req.Name, req.Email)
		if err != nil {
			if err.Error() == "invalid user name" || err.Error() == "invalid email" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
			return
		}

		writeJSON(w, http.StatusCreated, u)
	}
}

// GetUser handles GET /users/{id}
func (h *UserHandler) GetUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user ID"})
			return
		}

		u, err := h.svc.GetUserByID(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}

		writeJSON(w, http.StatusOK, u)
	}
}

// writeJSON is a helper to write JSON responses
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
