package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"splitpay/internal/domain"

	"github.com/go-playground/validator/v10"
)

type AuthService interface {
	SignUp(ctx context.Context, input domain.SignUpInput) error
	SignIn(ctx context.Context, input domain.SignInInput) (domain.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (domain.TokenPair, error)
}

type AuthHandler struct {
	service AuthService
	validate *validator.Validate
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{
		service:  service,
		validate: validator.New(),
	}
}

func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var input domain.SignUpInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body format", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := h.validate.Struct(input); err != nil {
		HandleValidationErrors(w, err)
		return
	}

	err := h.service.SignUp(r.Context(), input)
	if err != nil {
		if err.Error() == "user with this email already exists" {
			http.Error(w, err.Error(), http.StatusConflict) // 409
			return
		}

		http.Error(w, "internal server error", http.StatusInternalServerError) // 500
		return
	}

	w.WriteHeader(http.StatusCreated) // 201
}

func (h *AuthHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	var input domain.SignInInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body format", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := h.validate.Struct(input); err != nil {
		HandleValidationErrors(w, err)
		return
	}

	tokens, err := h.service.SignIn(r.Context(), input)
	if err != nil {
		if err.Error() == "invalid email or password" {
			http.Error(w, err.Error(), http.StatusUnauthorized) // 401
			return
		}

		http.Error(w, "internal server error", http.StatusInternalServerError) // 500
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
		http.Error(w, "invalid request body format", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if input.RefreshToken == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	tokens, err := h.service.RefreshTokens(r.Context(), input.RefreshToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tokens)
}
