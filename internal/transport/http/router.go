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
	authMW *middleware.AuthMiddleware,
	groupAccess middleware.GroupAccessChecker,
) *http.ServeMux {
	mux := http.NewServeMux()
	requireGroupAccess := middleware.RequireGroupAccess(groupAccess)

	mux.HandleFunc("POST /api/v1/auth/sign-up", authHandler.SignUp)
	mux.HandleFunc("POST /api/v1/auth/sign-in", authHandler.SignIn)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)

	// Для общих операций с группами достаточно авторизации.
	mux.HandleFunc("POST /api/v1/groups", authMW.RequireAuth(groupHandler.CreateGroup))
	mux.HandleFunc("GET /api/v1/groups", authMW.RequireAuth(groupHandler.GetUserGroups))
	mux.HandleFunc("POST /api/v1/groups/join", authMW.RequireAuth(groupHandler.JoinGroup))

	// Доступ к конкретной группе требует авторизации и членства в ней.
	mux.HandleFunc("GET /api/v1/groups/{id}", authMW.RequireAuth(requireGroupAccess(groupHandler.GetGroupDetails)))
	mux.HandleFunc("POST /api/v1/groups/{id}/invites", authMW.RequireAuth(requireGroupAccess(groupHandler.GenerateInvite)))

	return mux
}
