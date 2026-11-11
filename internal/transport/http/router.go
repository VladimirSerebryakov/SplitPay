// Package http связывает HTTP-маршруты с обработчиками и middleware.
package http

import (
	"net/http"

	"splitpay/internal/transport/http/middleware"
	"splitpay/internal/transport/http/v1"
)

// NewRouter регистрирует маршруты текущего API и проверки доступа к ним.
func NewRouter(
	authHandler *v1.AuthHandler,
	groupHandler *v1.GroupHandler,
	userHandler *v1.UserHandler,
	authMW *middleware.AuthMiddleware,
	rateLimiterMW *middleware.RateLimitMiddleware,
	groupAccess middleware.GroupAccessChecker,
) *http.ServeMux {
	mux := http.NewServeMux()
	requireGroupAccess := middleware.RequireGroupAccess(groupAccess)

	mux.HandleFunc("POST /api/v1/auth/sign-up", authHandler.SignUp)
	mux.HandleFunc("POST /api/v1/auth/sign-in", authHandler.SignIn)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)

	protected := func(next http.HandlerFunc) http.HandlerFunc { return authMW.RequireAuth(rateLimiterMW.RateLimit(next)) }

	mux.HandleFunc("GET /api/v1/me", protected(userHandler.GetMe))
	mux.HandleFunc("PATCH /api/v1/me", protected(userHandler.UpdateMe))

	// Для общих операций с группами достаточно авторизации.
	mux.HandleFunc("POST /api/v1/groups", protected(groupHandler.CreateGroup))
	mux.HandleFunc("GET /api/v1/groups", protected(groupHandler.GetUserGroups))
	mux.HandleFunc("POST /api/v1/groups/join", protected(groupHandler.JoinGroup))

	// Доступ к конкретной группе требует авторизации и членства в ней.
	mux.HandleFunc("GET /api/v1/groups/{groupId}", protected(requireGroupAccess(groupHandler.GetGroupDetails)))
	mux.HandleFunc("POST /api/v1/groups/{groupId}/invites", protected(requireGroupAccess(groupHandler.GenerateInvite)))

	return mux
}
