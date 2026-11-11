package middleware

import (
	"context"
	"net/http"
	"splitpay/internal/domain"
	"splitpay/internal/transport/http/response"
	"time"

	"github.com/google/uuid"
)

type RateLimitCounter interface {
	Increment(ctx context.Context, userID uuid.UUID, window time.Duration) (int64, time.Duration, error)
}

type RateLimitMiddleware struct {
	counter RateLimitCounter
	limit   int64
	window  time.Duration
}

func NewRateLimitMiddleware(counter RateLimitCounter, limit int64, window time.Duration) *RateLimitMiddleware {
	return &RateLimitMiddleware{counter: counter, limit: limit, window: window}
}

func (m *RateLimitMiddleware) RateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr, err := GetUserID(r.Context())
		if err != nil {
			response.WriteError(w, r, domain.ErrUnauthorized)
			return
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			response.WriteError(w, r, domain.ErrUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)

		cnt, remaining, err := m.counter.Increment(ctx, userID, m.window)
		cancel()

		if err != nil {
			response.WriteError(w, r, err)
			return
		}

		if cnt > m.limit {
			response.WriteError(w, r, &domain.RateLimitError{RetryAfter: remaining})
			return
		}
		next.ServeHTTP(w, r)
	}
}
