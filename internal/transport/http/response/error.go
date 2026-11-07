// Package response формирует общие HTTP-ответы для обработчиков и middleware.
package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"splitpay/internal/domain"
)

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// WriteError преобразует ошибку в безопасный JSON-ответ по контракту API.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	body := errorBody{Code: "internal", Message: "Внутренняя ошибка сервера"}
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.Is(err, domain.ErrInvalidBody), errors.As(err, &maxBytesError):
		status, body.Code, body.Message = http.StatusBadRequest, "invalid_body", domain.ErrInvalidBody.Error()
	case errors.Is(err, domain.ErrValidationFailed):
		status, body.Code, body.Message = http.StatusBadRequest, "validation_failed", domain.ErrValidationFailed.Error()
		var validationError *domain.ValidationError
		if errors.As(err, &validationError) && validationError != nil {
			body.Details = validationError.Fields
		}
	case errors.Is(err, domain.ErrInviteExpired):
		status, body.Code, body.Message = http.StatusBadRequest, "invite_expired", domain.ErrInviteExpired.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		status, body.Code, body.Message = http.StatusUnauthorized, "unauthorized", domain.ErrUnauthorized.Error()
	case errors.Is(err, domain.ErrInvalidCredentials):
		status, body.Code, body.Message = http.StatusUnauthorized, "invalid_credentials", domain.ErrInvalidCredentials.Error()
	case errors.Is(err, domain.ErrInvalidRefreshToken):
		status, body.Code, body.Message = http.StatusUnauthorized, "invalid_refresh_token", domain.ErrInvalidRefreshToken.Error()
	case errors.Is(err, domain.ErrForbidden):
		status, body.Code, body.Message = http.StatusForbidden, "forbidden", domain.ErrForbidden.Error()
	case errors.Is(err, domain.ErrNotFound):
		status, body.Code, body.Message = http.StatusNotFound, "not_found", domain.ErrNotFound.Error()
	case errors.Is(err, domain.ErrEmailTaken):
		status, body.Code, body.Message = http.StatusConflict, "email_taken", domain.ErrEmailTaken.Error()
	case errors.Is(err, domain.ErrIdempotencyKeyMismatch):
		status, body.Code, body.Message = http.StatusUnprocessableEntity, "idempotency_key_mismatch", domain.ErrIdempotencyKeyMismatch.Error()
	case errors.Is(err, domain.ErrTooManyParticipants):
		status, body.Code, body.Message = http.StatusUnprocessableEntity, "too_many_participants", domain.ErrTooManyParticipants.Error()
	case errors.Is(err, domain.ErrRateLimited):
		status, body.Code, body.Message = http.StatusTooManyRequests, "rate_limited", domain.ErrRateLimited.Error()
		var rateLimitError *domain.RateLimitError
		seconds := int64(1)
		if errors.As(err, &rateLimitError) && rateLimitError != nil {
			seconds = int64(rateLimitError.RetryAfter / time.Second)
			if rateLimitError.RetryAfter%time.Second > 0 {
				seconds++
			}
			if seconds < 1 {
				seconds = 1
			}
		}
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	default:
		slog.ErrorContext(r.Context(), "Ошибка обработки HTTP-запроса", "error", err, "method", r.Method, "path", r.URL.Path)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if encodeErr := json.NewEncoder(w).Encode(errorResponse{Error: body}); encodeErr != nil {
		slog.ErrorContext(r.Context(), "Не удалось записать ответ с ошибкой", "error", encodeErr)
	}
}
