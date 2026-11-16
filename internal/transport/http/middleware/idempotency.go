package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"splitpay/internal/domain"
	"splitpay/internal/service"
	"splitpay/internal/token"
	"splitpay/internal/transport/http/response"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MAXBYTESREADER = 5000

type IdempotencyChecker interface {
	CheckIdempotency(ctx context.Context, userID uuid.UUID, idempotencyKey uuid.UUID, requestHash string) (*domain.CachedResponse, error)
	WriteIdempotency(ctx context.Context, userID uuid.UUID, idempotencyKey uuid.UUID, requestHash string, response domain.CachedResponse) error
}

type IdempotencyMiddleware struct {
	checker IdempotencyChecker
}

func NewIdempotencyMiddleware(checker IdempotencyChecker) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{checker: checker}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode  int
	body        *bytes.Buffer
	wroteHeader bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           new(bytes.Buffer),
	}
}

func (r *responseRecorder) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		r.ResponseWriter.WriteHeader(code)
		return
	}

	if r.wroteHeader {
		return
	}

	r.wroteHeader = true

	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (im *IdempotencyMiddleware) Middleware(required bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		userIDstr, err := GetUserID(r.Context())
		if err != nil {
			response.WriteError(w, r, domain.ErrUnauthorized)
			return
		}

		userID, err := uuid.Parse(userIDstr)
		if err != nil {
			response.WriteError(w, r, domain.ErrUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, MAXBYTESREADER)
		defer r.Body.Close()

		idempotencyKeyStr := r.Header.Get("Idempotency-Key")
		if idempotencyKeyStr == "" {
			if required {
				response.WriteError(w, r, &domain.ValidationError{
					Fields: map[string]string{
						"Idempotency-Key": "Обязательный заголовок",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
			return
		}

		idempotencyKey, err := uuid.Parse(idempotencyKeyStr)
		if err != nil {
			response.WriteError(w, r, domain.ErrInvalidBody)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			response.WriteError(w, r, fmt.Errorf("не удалось прочитать тело запроса: %w", err))
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var data bytes.Buffer
		parts := [][]byte{
			[]byte(strings.ToUpper(r.Method)),
			[]byte(r.URL.Path),
			bodyBytes,
		}

		for _, part := range parts {
			fmt.Fprintf(&data, "%d:", len(part))
			data.Write(part)
		}

		requestHash := token.Hash(data.String())

		cacheCtx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
		resp, err := im.checker.CheckIdempotency(cacheCtx, userID, idempotencyKey, requestHash)
		cancel()
		if errors.Is(err, domain.ErrIdempotencyKeyMismatch) {
			response.WriteError(w, r, err)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "Не удалось прочитать кэш идемпотентности", "error", err)
			resp = nil
		}
		if resp != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.ResponseCode)

			if len(resp.ResponseBody) > 0 {
				if _, err := w.Write(resp.ResponseBody); err != nil {
					slog.ErrorContext(r.Context(),
						"Не удалось отправить сохранённый ответ",
						"error", err,
					)
				}
			}
			return
		}

		recorder := newResponseRecorder(w)

		ctx := service.WithIdempotency(r.Context(), service.IdempotencyData{
			Key:         idempotencyKey,
			RequestHash: requestHash,
		})

		next.ServeHTTP(recorder, r.WithContext(ctx))

		statusCode := recorder.statusCode
		rawBytes := recorder.body.Bytes()

		var responseBody json.RawMessage
		if len(rawBytes) > 0 {
			responseBody = rawBytes
		}
		respo := domain.CachedResponse{
			RequestHash:  requestHash,
			ResponseCode: statusCode,
			ResponseBody: responseBody,
		}
		if statusCode >= 200 && statusCode < 300 {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			err := im.checker.WriteIdempotency(cacheCtx, userID, idempotencyKey, requestHash, respo)
			cancel()
			if err != nil {
				slog.ErrorContext(r.Context(),
					"Не удалось сохранить ответ в кэш идемпотентности",
					"error", err,
				)
			}
		}

	}
}
