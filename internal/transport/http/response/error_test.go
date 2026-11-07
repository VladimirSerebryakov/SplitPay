package response_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"splitpay/internal/domain"
	"splitpay/internal/transport/http/response"
)

func TestWriteErrorContract(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidBody, 400, "invalid_body"},
		{&http.MaxBytesError{Limit: 10}, 400, "invalid_body"},
		{domain.ErrValidationFailed, 400, "validation_failed"},
		{domain.ErrInviteExpired, 400, "invite_expired"},
		{domain.ErrUnauthorized, 401, "unauthorized"},
		{domain.ErrInvalidCredentials, 401, "invalid_credentials"},
		{domain.ErrInvalidRefreshToken, 401, "invalid_refresh_token"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrEmailTaken, 409, "email_taken"},
		{domain.ErrIdempotencyKeyMismatch, 422, "idempotency_key_mismatch"},
		{domain.ErrTooManyParticipants, 422, "too_many_participants"},
		{domain.ErrRateLimited, 429, "rate_limited"},
		{domain.ErrAlreadyExists, 500, "internal"},
		{errors.New("ошибка подключения к БД"), 500, "internal"},
	}
	for _, tc := range tests {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/wrapped=%t/%T", tc.code, wrapped, tc.err), func(t *testing.T) {
				err := tc.err
				if wrapped {
					err = fmt.Errorf("внутренняя причина: %w", err)
				}
				rr := httptest.NewRecorder()
				response.WriteError(rr, httptest.NewRequest(http.MethodPost, "/api/v1/groups", nil), err)
				if rr.Code != tc.status || rr.Header().Get("Content-Type") != "application/json" {
					t.Fatalf("Неверный статус или Content-Type: %d %v", rr.Code, rr.Header())
				}
				var envelope map[string]map[string]json.RawMessage
				if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				body := envelope["error"]
				var code, message string
				if err := json.Unmarshal(body["code"], &code); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body["message"], &message); err != nil {
					t.Fatal(err)
				}
				if len(envelope) != 1 || len(body) != 2 || code != tc.code || message == "" {
					t.Fatalf("Неверный формат ошибки: %s", rr.Body.String())
				}
				if strings.Contains(rr.Body.String(), "внутренняя причина") || strings.Contains(rr.Body.String(), "подключения к БД") {
					t.Fatalf("В ответ попала внутренняя причина: %s", rr.Body.String())
				}
				wantRetry := ""
				if tc.status == 429 {
					wantRetry = "1"
				}
				if rr.Header().Get("Retry-After") != wantRetry {
					t.Fatal("Неверный Retry-After")
				}
			})
		}
	}
}

func TestWriteValidationDetails(t *testing.T) {
	fields := map[string]string{"refresh_token": "Это поле обязательно для заполнения"}
	err := fmt.Errorf("проверка: %w", &domain.ValidationError{Fields: fields})
	if !errors.Is(err, domain.ErrValidationFailed) {
		t.Fatal("Потеряна категория ошибки")
	}
	rr := httptest.NewRecorder()
	response.WriteError(rr, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil), err)
	var result struct {
		Error struct {
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rr.Code != 400 || !reflect.DeepEqual(result.Error.Details, fields) {
		t.Fatalf("Неверные details: %s", rr.Body.String())
	}
}

func TestRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		wait time.Duration
		want string
	}{
		{-time.Second, "1"}, {0, "1"}, {time.Nanosecond, "1"}, {time.Second, "1"},
		{2400 * time.Millisecond, "3"}, {3 * time.Second, "3"}, {time.Duration(1<<63 - 1), "9223372037"},
	} {
		t.Run(tc.wait.String(), func(t *testing.T) {
			err := fmt.Errorf("лимит: %w", &domain.RateLimitError{RetryAfter: tc.wait})
			if !errors.Is(err, domain.ErrRateLimited) {
				t.Fatal("Потеряна категория ошибки")
			}
			rr := httptest.NewRecorder()
			response.WriteError(rr, httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil), err)
			if rr.Code != 429 || rr.Header().Get("Retry-After") != tc.want {
				t.Fatalf("Неверный Retry-After: %v", rr.Header())
			}
		})
	}
}

func TestInternalErrorLogging(t *testing.T) {
	var logs bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh?token=query-secret", strings.NewReader(`{"refresh_token":"body-secret"}`))
	req.Header.Set("Authorization", "Bearer header-secret")
	rr := httptest.NewRecorder()
	response.WriteError(rr, req, errors.New("причина внутреннего сбоя"))
	if !strings.Contains(logs.String(), "причина внутреннего сбоя") {
		t.Fatal("Причина не записана в лог")
	}
	for _, secret := range []string{"body-secret", "header-secret", "query-secret"} {
		if strings.Contains(logs.String(), secret) || strings.Contains(rr.Body.String(), secret) {
			t.Fatalf("Раскрыт секрет %s", secret)
		}
	}
	if strings.Contains(rr.Body.String(), "причина внутреннего сбоя") {
		t.Fatal("Причина попала клиенту")
	}
}
