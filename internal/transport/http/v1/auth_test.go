package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"splitpay/internal/domain"
)

// === 1. Mock для AuthService ===

type mockAuthService struct {
	signUpErr     error
	signInTokens  domain.TokenPair
	signInErr     error
	refreshTokens domain.TokenPair
	refreshErr    error
}

func (m *mockAuthService) SignUp(ctx context.Context, input domain.SignUpInput) error {
	return m.signUpErr
}

func (m *mockAuthService) SignIn(ctx context.Context, input domain.SignInInput) (domain.TokenPair, error) {
	return m.signInTokens, m.signInErr
}

func (m *mockAuthService) RefreshTokens(ctx context.Context, refreshToken string) (domain.TokenPair, error) {
	return m.refreshTokens, m.refreshErr
}

// === 2. Тесты для SignUp ===

func TestAuthHandler_SignUp(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  string
		mockError    error
		expectedCode int
	}{
		{
			name:         "Success",
			requestBody:  `{"email": "test@test.com", "password": "password123"}`,
			mockError:    nil,
			expectedCode: http.StatusCreated,
		},
		{
			name:         "Invalid JSON",
			requestBody:  `{"email": "test@test.com", "pass`,
			mockError:    nil,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "User Already Exists",
			requestBody:  `{"email": "test@test.com", "password": "password123"}`,
			mockError:    errors.New("user with this email already exists"),
			expectedCode: http.StatusConflict,
		},
		{
			name:         "Internal Server Error",
			requestBody:  `{"email": "test@test.com", "password": "password123"}`,
			mockError:    errors.New("db error"),
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockAuthService{signUpErr: tc.mockError}
			handler := NewAuthHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-up", bytes.NewBufferString(tc.requestBody))
			recorder := httptest.NewRecorder()

			handler.SignUp(recorder, req)

			if recorder.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, recorder.Code)
			}
		})
	}
}

// === 3. Тесты для SignIn ===

func TestAuthHandler_SignIn(t *testing.T) {
	expectedTokens := domain.TokenPair{
		AccessToken:  "valid.access.token",
		RefreshToken: "valid.refresh.token",
	}

	tests := []struct {
		name           string
		requestBody    string
		mockTokens     domain.TokenPair
		mockError      error
		expectedCode   int
		expectingToken bool
	}{
		{
			name:           "Success",
			requestBody:    `{"email": "test@test.com", "password": "password123"}`,
			mockTokens:     expectedTokens,
			mockError:      nil,
			expectedCode:   http.StatusOK,
			expectingToken: true,
		},
		{
			name:           "Invalid JSON",
			requestBody:    `not-a-json`,
			mockTokens:     domain.TokenPair{},
			mockError:      nil,
			expectedCode:   http.StatusBadRequest,
			expectingToken: false,
		},
		{
			name:           "Invalid Credentials",
			requestBody:    `{"email": "test@test.com", "password": "wrong"}`,
			mockTokens:     domain.TokenPair{},
			mockError:      errors.New("invalid email or password"),
			expectedCode:   http.StatusUnauthorized,
			expectingToken: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockAuthService{
				signInTokens: tc.mockTokens,
				signInErr:    tc.mockError,
			}
			handler := NewAuthHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sign-in", bytes.NewBufferString(tc.requestBody))
			recorder := httptest.NewRecorder()

			handler.SignIn(recorder, req)

			if recorder.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, recorder.Code)
			}

			if tc.expectingToken {
				var responseTokens domain.TokenPair
				err := json.NewDecoder(recorder.Body).Decode(&responseTokens)
				if err != nil {
					t.Fatalf("failed to decode response JSON: %v", err)
				}
				if responseTokens.AccessToken != tc.mockTokens.AccessToken {
					t.Errorf("expected access token %s, got %s", tc.mockTokens.AccessToken, responseTokens.AccessToken)
				}
			}
		})
	}
}

// === 4. Тесты для Refresh ===

func TestAuthHandler_Refresh(t *testing.T) {
	expectedTokens := domain.TokenPair{
		AccessToken:  "new.access.token",
		RefreshToken: "new.refresh.token",
	}

	tests := []struct {
		name           string
		requestBody    string
		mockTokens     domain.TokenPair
		mockError      error
		expectedCode   int
	}{
		{
			name:           "Success",
			requestBody:    `{"refresh_token": "old.refresh.token"}`,
			mockTokens:     expectedTokens,
			mockError:      nil,
			expectedCode:   http.StatusOK,
		},
		{
			name:           "Empty Token",
			requestBody:    `{"refresh_token": ""}`,
			mockTokens:     domain.TokenPair{},
			mockError:      nil,
			expectedCode:   http.StatusBadRequest,
		},
		{
			name:           "Expired or Invalid Token",
			requestBody:    `{"refresh_token": "expired.token"}`,
			mockTokens:     domain.TokenPair{},
			mockError:      errors.New("refresh token expired"),
			expectedCode:   http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockAuthService{
				refreshTokens: tc.mockTokens,
				refreshErr:    tc.mockError,
			}
			handler := NewAuthHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(tc.requestBody))
			recorder := httptest.NewRecorder()

			handler.Refresh(recorder, req)

			if recorder.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, recorder.Code)
			}
		})
	}
}
