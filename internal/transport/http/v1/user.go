package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"splitpay/internal/domain"
	"splitpay/internal/transport/http/middleware"
	"splitpay/internal/transport/http/response"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type UserService interface {
	GetMe(ctx context.Context, userID uuid.UUID) (domain.UserProfile, error)
	UpdateMe(ctx context.Context, userID uuid.UUID, name string) (domain.UserProfile, error)
}

type UserHandler struct {
	service  UserService
	validate *validator.Validate
}

func NewUserHandler(service UserService) *UserHandler {
	return &UserHandler{
		service:  service,
		validate: newValidator(),
	}
}

func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userIDStr, err := middleware.GetUserID(r.Context())
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userProfile, err := h.service.GetMe(r.Context(), userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(userProfile)
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userIDStr, err := middleware.GetUserID(r.Context())
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	var input struct {
		Name string `json:"name" validate:"required,max=50"`
	}

	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}

	if err := h.validate.Struct(input); err != nil {
		response.WriteError(w, r, validationError(err))
		return
	}

	userProfile, err := h.service.UpdateMe(r.Context(), userID, input.Name)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(userProfile)
}
