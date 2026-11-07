package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"splitpay/internal/domain"
)

type mockAuthService struct {
	signUpUser    *domain.User
	signUpErr     error
	signInTokens  domain.TokenPair
	signInErr     error
	refreshTokens domain.TokenPair
	refreshErr    error
}

func (m *mockAuthService) SignUp(context.Context, domain.SignUpInput) (*domain.User, error) {
	return m.signUpUser, m.signUpErr
}

func (m *mockAuthService) SignIn(context.Context, domain.SignInInput) (domain.TokenPair, error) {
	return m.signInTokens, m.signInErr
}

func (m *mockAuthService) RefreshTokens(context.Context, string) (domain.TokenPair, error) {
	return m.refreshTokens, m.refreshErr
}

func TestAuthHandler_SignUp(t *testing.T) {
	user := &domain.User{
		ID: uuid.New(), Name: "Иван", Email: "ivan@example.com",
		CreatedAt:    time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		PasswordHash: "секретный хеш",
	}
	const validBody = `{"name":"Иван","email":"ivan@example.com","password":"password123"}`
	for _, tc := range []struct {
		name, body  string
		err         error
		status      int
		code, field string
	}{
		{"успех", validBody, nil, 201, "", ""},
		{"неверный JSON", `{`, nil, 400, "invalid_body", ""},
		{"нет имени", `{"email":"ivan@example.com","password":"password123"}`,
			&domain.ValidationError{Fields: map[string]string{"name": "Имя обязательно"}}, 400, "validation_failed", "name"},
		{"неверный email", `{"name":"Иван","email":"wrong","password":"password123"}`,
			&domain.ValidationError{Fields: map[string]string{"email": "Неверный формат email адреса"}}, 400, "validation_failed", "email"},
		{"email занят", validBody, fmt.Errorf("регистрация: %w", domain.ErrEmailTaken), 409, "email_taken", ""},
		{"сбой сервиса", validBody, errors.New("секретная причина сбоя хранилища"), 500, "internal", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{signUpUser: user, signUpErr: tc.err})
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-up", strings.NewReader(tc.body))
			handler.SignUp(rr, req)
			if tc.code != "" {
				checkErrorResponse(t, rr, tc.status, tc.code, tc.field)
				return
			}
			if rr.Code != 201 || rr.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("Ожидался JSON с кодом 201, получено: %d %s", rr.Code, rr.Body.String())
			}
			var result domain.User
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ID != user.ID || result.Name != user.Name || result.Email != user.Email || !result.CreatedAt.Equal(user.CreatedAt) {
				t.Fatalf("Неверные данные пользователя: %s", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "password_hash") || strings.Contains(rr.Body.String(), user.PasswordHash) {
				t.Fatal("Хеш пароля попал в ответ")
			}
		})
	}
}

func TestAuthHandler_SignIn(t *testing.T) {
	tokens := domain.TokenPair{AccessToken: "access", RefreshToken: "refresh"}
	for _, tc := range []struct {
		name, body  string
		err         error
		status      int
		code, field string
	}{
		{"успех", `{"email":"ivan@example.com","password":"password123"}`, nil, 200, "", ""},
		{"неверный JSON", `[]`, nil, 400, "invalid_body", ""},
		{"пустые поля", `{}`, nil, 400, "validation_failed", "email"},
		{"неверный пароль", `{"email":"ivan@example.com","password":"wrong"}`,
			fmt.Errorf("вход: %w", domain.ErrInvalidCredentials), 401, "invalid_credentials", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{signInTokens: tokens, signInErr: tc.err})
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-in", strings.NewReader(tc.body))
			handler.SignIn(rr, req)
			if tc.code != "" {
				checkErrorResponse(t, rr, tc.status, tc.code, tc.field)
				return
			}
			var result domain.TokenPair
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if rr.Code != 200 || result != tokens {
				t.Fatalf("Неверный ответ: %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAuthHandler_Refresh(t *testing.T) {
	tokens := domain.TokenPair{AccessToken: "new-access", RefreshToken: "new-refresh"}
	for _, tc := range []struct {
		name, body  string
		err         error
		status      int
		code, field string
	}{
		{"успех", `{"refresh_token":"old-refresh"}`, nil, 200, "", ""},
		{"неверный JSON", `{`, nil, 400, "invalid_body", ""},
		{"пустой токен", `{"refresh_token":""}`, nil, 400, "validation_failed", "refresh_token"},
		{"неверный токен", `{"refresh_token":"wrong"}`,
			fmt.Errorf("refresh: %w", domain.ErrInvalidRefreshToken), 401, "invalid_refresh_token", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewAuthHandler(&mockAuthService{refreshTokens: tokens, refreshErr: tc.err})
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(tc.body))
			handler.Refresh(rr, req)
			if tc.code != "" {
				checkErrorResponse(t, rr, tc.status, tc.code, tc.field)
				return
			}
			var result domain.TokenPair
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if rr.Code != 200 || result != tokens {
				t.Fatalf("Неверный ответ: %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAuthHandlerBodyTooLarge(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-up", strings.NewReader(`{"name":"Иван","email":"ivan@example.com","password":"password123"}`))
	req.Body = http.MaxBytesReader(rr, req.Body, 8)
	NewAuthHandler(&mockAuthService{}).SignUp(rr, req)
	checkErrorResponse(t, rr, 400, "invalid_body", "")
}
