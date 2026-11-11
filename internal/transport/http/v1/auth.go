package v1

import (
	"context"
	"encoding/json"
	"net/http"

	"splitpay/internal/domain"
	"splitpay/internal/transport/http/response"

	"github.com/go-playground/validator/v10"
)

type AuthService interface {
	SignUp(ctx context.Context, input domain.SignUpInput) (*domain.User, error)
	SignIn(ctx context.Context, input domain.SignInInput) (domain.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (domain.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type AuthHandler struct {
	service  AuthService
	validate *validator.Validate
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{
		service:  service,
		validate: newValidator(),
	}
}

func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var input domain.SignUpInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	defer r.Body.Close()

	user, err := h.service.SignUp(r.Context(), input)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated) // 201
	json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	var input domain.SignInInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	defer r.Body.Close()

	if err := h.validate.Struct(input); err != nil {
		response.WriteError(w, r, validationError(err))
		return
	}

	tokens, err := h.service.SignIn(r.Context(), input)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // 200
	json.NewEncoder(w).Encode(tokens)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	defer r.Body.Close()

	if input.RefreshToken == "" {
		response.WriteError(w, r, &domain.ValidationError{Fields: map[string]string{
			"refresh_token": "Это поле обязательно для заполнения",
		}})
		return
	}

	tokens, err := h.service.RefreshTokens(r.Context(), input.RefreshToken)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tokens)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	defer r.Body.Close()

	if err := h.validate.Struct(input); err != nil {
		response.WriteError(w, r, validationError(err))
		return
	}

	err := h.service.Logout(r.Context(), input.RefreshToken)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
