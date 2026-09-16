package middleware

import (
	"context"
	"net/http"

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
				http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
				return
			}

			userID, err := uuid.Parse(userIDStr)
			if err != nil {
				http.Error(w, "unauthorized: invalid user id format", http.StatusUnauthorized)
				return
			}

			groupIDStr := r.PathValue("id")
			groupID, err := uuid.Parse(groupIDStr)
			if err != nil {
				http.Error(w, "bad request: invalid group id format", http.StatusBadRequest)
				return
			}

			hasAccess, err := checker.IsUserInGroup(r.Context(), groupID, userID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			if !hasAccess {
				http.Error(w, "forbidden: you are not a member of this group", http.StatusForbidden)
				return
			}

			next(w, r)
		}
	}
}
