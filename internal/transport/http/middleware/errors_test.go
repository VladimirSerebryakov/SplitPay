package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"splitpay/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func checkMiddlewareError(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var result struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rr.Code != status || result.Error.Code != code || result.Error.Message == "" || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Неверный ответ middleware: %d %s", rr.Code, rr.Body.String())
	}
}

func TestAuthErrorResponses(t *testing.T) {
	userID := uuid.NewString()
	sign := func(secret string, claims jwt.MapClaims) string {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return "Bearer " + token
	}
	claims := jwt.MapClaims{"sub": userID, "exp": time.Now().Add(time.Hour).Unix()}
	valid := sign("test-secret", claims)
	for _, tc := range []struct {
		name, header string
		allowed      bool
	}{
		{"нет заголовка", "", false},
		{"неверный заголовок", "Basic test", false},
		{"неверный JWT", "Bearer broken", false},
		{"неверная подпись", sign("another-secret", claims), false},
		{"истёкший JWT", sign("test-secret", jwt.MapClaims{"sub": userID, "exp": time.Now().Add(-time.Hour).Unix()}), false},
		{"нет sub", sign("test-secret", jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}), false},
		{"неверный тип sub", sign("test-secret", jwt.MapClaims{"sub": 123, "exp": time.Now().Add(time.Hour).Unix()}), false},
		{"доступ разрешён", valid, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			next := NewAuthMiddleware("test-secret").RequireAuth(func(w http.ResponseWriter, r *http.Request) {
				called = true
				id, err := GetUserID(r.Context())
				if err != nil || id != userID {
					t.Fatal("Не передан user_id")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
			req.Header.Set("Authorization", tc.header)
			rr := httptest.NewRecorder()
			next(rr, req)
			if called != tc.allowed {
				t.Fatalf("Вызов обработчика: %t", called)
			}
			if !tc.allowed {
				checkMiddlewareError(t, rr, 401, "unauthorized")
			} else if rr.Code != 204 {
				t.Fatal("Не выполнен обработчик")
			}
		})
	}
}

type groupCheckerFunc func(context.Context, uuid.UUID, uuid.UUID) (bool, error)

func (f groupCheckerFunc) IsUserInGroup(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	return f(ctx, groupID, userID)
}

func TestGroupAccessErrorResponses(t *testing.T) {
	groupID, userID := uuid.NewString(), uuid.NewString()
	for _, tc := range []struct {
		name, userID, groupID string
		member                bool
		err                   error
		status                int
		code                  string
		checked               bool
	}{
		{"нет пользователя", "", groupID, false, nil, 401, "unauthorized", false},
		{"неверный пользователь", "wrong", groupID, false, nil, 401, "unauthorized", false},
		{"неверная группа", userID, "wrong", false, nil, 400, "validation_failed", false},
		{"группа не найдена", userID, groupID, false, fmt.Errorf("проверка: %w", domain.ErrNotFound), 404, "not_found", true},
		{"нет членства", userID, groupID, false, nil, 403, "forbidden", true},
		{"сбой хранилища", userID, groupID, false, fmt.Errorf("проверка: %w", errors.New("БД недоступна")), 500, "internal", true},
		{"доступ разрешён", userID, groupID, true, nil, 204, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checked, called := false, false
			checker := groupCheckerFunc(func(_ context.Context, g, u uuid.UUID) (bool, error) {
				checked = true
				if g.String() != groupID || u.String() != userID {
					t.Fatal("Неверные идентификаторы при проверке")
				}
				return tc.member, tc.err
			})
			next := RequireGroupAccess(checker)(func(w http.ResponseWriter, r *http.Request) {
				called = true
				id, err := GetGroupID(r.Context())
				if err != nil || id.String() != groupID {
					t.Fatalf("Не передан UUID группы: %v, %v", id, err)
				}
				w.WriteHeader(204)
			})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+tc.groupID, nil)
			req.SetPathValue("groupId", tc.groupID)
			if tc.userID != "" {
				req = req.WithContext(context.WithValue(req.Context(), UserIDKey, tc.userID))
			}
			rr := httptest.NewRecorder()
			next(rr, req)
			if checked != tc.checked || called != (tc.status == 204) {
				t.Fatalf("Нарушен порядок проверок: checked=%t called=%t", checked, called)
			}
			if tc.status != 204 {
				checkMiddlewareError(t, rr, tc.status, tc.code)
			} else if rr.Code != 204 {
				t.Fatal("Не выполнен обработчик")
			}
		})
	}
}

func TestGetGroupIDMissingContext(t *testing.T) {
	id, err := GetGroupID(context.Background())
	if id != uuid.Nil || err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Ожидалась внутренняя ошибка отсутствующего context: %v, %v", id, err)
	}
}
