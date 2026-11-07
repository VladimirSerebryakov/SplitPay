package middleware

import (
	"context"
	"net/http"

	"splitpay/internal/domain"
	"splitpay/internal/transport/http/response"

	"github.com/google/uuid"
)

type GroupAccessChecker interface {
	IsUserInGroup(ctx context.Context, groupID, userID uuid.UUID) (bool, error)
}

func RequireGroupAccess(checker GroupAccessChecker) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
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

			groupIDStr := r.PathValue("id")
			groupID, err := uuid.Parse(groupIDStr)
			if err != nil {
				response.WriteError(w, r, &domain.ValidationError{Fields: map[string]string{
					"groupId": "Неверный формат UUID группы",
				}})
				return
			}

			hasAccess, err := checker.IsUserInGroup(r.Context(), groupID, userID)
			if err != nil {
				response.WriteError(w, r, err)
				return
			}

			if !hasAccess {
				response.WriteError(w, r, domain.ErrForbidden)
				return
			}

			next(w, r)
		}
	}
}
