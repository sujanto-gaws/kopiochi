package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	appUser "github.com/sujanto-gaws/kopiochi/internal/application/user"
	"github.com/sujanto-gaws/kopiochi/internal/domain/user"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	svc *appUser.Service
}

// NewUserHandler creates a new user handler
func NewUserHandler(svc *appUser.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

// CreateUser handles POST /users
// @Summary Create a new user
// @Description Create a new user with name and email
// @Tags users
// @Accept json
// @Produce json
// @Param request body user.CreateUserRequest true "User creation request"
// @Success 201 {object} user.UserResponse "User created successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users [post]
func (h *UserHandler) CreateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req user.CreateUserRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}

		resp, err := h.svc.CreateUser(r.Context(), &req)
		if err != nil {
			switch err {
			case user.ErrInvalidName, user.ErrInvalidEmail:
				writeJSON(w, http.StatusBadRequest, errorResponse(err.Error()))
			default:
				writeJSON(w, http.StatusInternalServerError, errorResponse("failed to create user"))
			}
			return
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// GetUser handles GET /users/{id}
// @Summary Get a user by ID
// @Description Retrieve a user by their unique ID
// @Tags users
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} user.UserResponse "User found"
// @Failure 400 {object} map[string]string "Invalid user ID"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/{id} [get]
func (h *UserHandler) GetUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid user ID"))
			return
		}

		resp, err := h.svc.GetUserByID(r.Context(), id)
		if err != nil {
			if err == user.ErrUserNotFound {
				writeJSON(w, http.StatusNotFound, errorResponse("user not found"))
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to fetch user"))
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// UpdateUser handles PUT /users/{id}
// @Summary Update an existing user
// @Description Update a user's name and/or email by their ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param request body user.UpdateUserRequest true "User update request"
// @Success 200 {object} user.UserResponse "User updated successfully"
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/{id} [put]
func (h *UserHandler) UpdateUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid user ID"))
			return
		}

		var req user.UpdateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid request body"))
			return
		}

		resp, err := h.svc.UpdateUser(r.Context(), id, &req)
		if err != nil {
			switch err {
			case user.ErrUserNotFound:
				writeJSON(w, http.StatusNotFound, errorResponse("user not found"))
			case user.ErrInvalidName, user.ErrInvalidEmail:
				writeJSON(w, http.StatusBadRequest, errorResponse(err.Error()))
			default:
				writeJSON(w, http.StatusInternalServerError, errorResponse("failed to update user"))
			}
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// DeleteUser handles DELETE /users/{id}
// @Summary Delete a user by ID
// @Description Delete a user by their unique ID
// @Tags users
// @Param id path int true "User ID"
// @Success 204 "User deleted successfully"
// @Failure 400 {object} map[string]string "Invalid user ID"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /users/{id} [delete]
func (h *UserHandler) DeleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse("invalid user ID"))
			return
		}

		if err := h.svc.DeleteUser(r.Context(), id); err != nil {
			if err == user.ErrUserNotFound {
				writeJSON(w, http.StatusNotFound, errorResponse("user not found"))
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse("failed to delete user"))
			return
		}

		writeJSON(w, http.StatusNoContent, nil)
	}
}

// errorResponse creates a standardized error JSON response
func errorResponse(message string) map[string]string {
	return map[string]string{"error": message}
}

// writeJSON is a helper to write JSON responses
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}
