package api

import (
	"net/http"
	"time"

	"github.com/benchristian88/atlas-dns/internal/domain"
	"github.com/benchristian88/atlas-dns/internal/useradmin"
)

type administeredUserResponse struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"displayName"`
	Role        domain.UserRole `json:"role"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	LastLoginAt *time.Time      `json:"lastLoginAt,omitempty"`
	MFAEnabled  bool            `json:"mfaEnabled"`
}

func administeredUser(user domain.User) administeredUserResponse {
	return administeredUserResponse{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role, Enabled: user.Enabled, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, LastLoginAt: user.LastLoginAt, MFAEnabled: user.MFAEnabled}
}

func (s *Server) handleListUsers(response http.ResponseWriter, request *http.Request) {
	users, err := s.users.List(request.Context())
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	items := make([]administeredUserResponse, 0, len(users))
	for _, user := range users {
		items = append(items, administeredUser(user))
	}
	writeJSON(response, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateUser(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Email       string          `json:"email"`
		DisplayName string          `json:"displayName"`
		Password    string          `json:"password"`
		Role        domain.UserRole `json:"role"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	user, err := s.users.Create(request.Context(), actor(request.Context()), useradmin.CreateInput{Email: input.Email, DisplayName: input.DisplayName, Password: input.Password, Role: input.Role})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	response.Header().Set("Location", "/api/v1/users/"+user.ID)
	writeJSON(response, http.StatusCreated, administeredUser(user))
}

func (s *Server) handleUpdateUser(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Email       string          `json:"email"`
		DisplayName string          `json:"displayName"`
		Role        domain.UserRole `json:"role"`
		Enabled     bool            `json:"enabled"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	user, err := s.users.Update(request.Context(), actor(request.Context()), request.PathValue("userId"), useradmin.UpdateInput{Email: input.Email, DisplayName: input.DisplayName, Role: input.Role, Enabled: input.Enabled})
	if err != nil {
		s.writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, administeredUser(user))
}

func (s *Server) handleResetUserPassword(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	if err := s.users.ResetPassword(request.Context(), actor(request.Context()), request.PathValue("userId"), input.Password); err != nil {
		s.writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChangeOwnPassword(response http.ResponseWriter, request *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		TOTPCode        string `json:"totpCode,omitempty"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		s.writeError(response, request, err)
		return
	}
	if err := s.users.ChangeOwnPassword(
		request.Context(),
		actor(request.Context()),
		authenticatedSession(request.Context()).ID,
		input.CurrentPassword,
		input.NewPassword,
		input.TOTPCode,
	); err != nil {
		s.writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
