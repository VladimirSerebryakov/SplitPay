package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splitpay/internal/config"
	"splitpay/internal/repository/postgres"
	"splitpay/internal/service"
	"splitpay/internal/transport/http/v1"
	"splitpay/internal/transport/http/middleware"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}
	fmt.Println("Successfully connected to PostgreSQL!")

	authRepo := postgres.NewAuthPostgres(pool)
	groupRepo := postgres.NewGroupRepository(pool)

	authService := service.NewAuthService(authRepo, cfg.JWTSecret)
	groupService := service.NewGroupService(groupRepo)

	authHandler := v1.NewAuthHandler(authService)
	groupHandler := v1.NewGroupHandler(groupService)

	authMW := middleware.NewAuthMiddleware(cfg.JWTSecret)
	requireGroupAccess := middleware.RequireGroupAccess(groupRepo)


	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/sign-up", authHandler.SignUp)
	mux.HandleFunc("POST /api/v1/auth/sign-in", authHandler.SignIn)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)

	// Защищенные роуты (Только авторизованные пользователи)
	mux.HandleFunc("POST /api/v1/groups", authMW.RequireAuth(groupHandler.CreateGroup))
	mux.HandleFunc("GET /api/v1/groups", authMW.RequireAuth(groupHandler.GetUserGroups))
	mux.HandleFunc("POST /api/v1/groups/join", authMW.RequireAuth(groupHandler.JoinGroup))

	// Защищенные роуты (Авторизация + Доступ к конкретной группе)
	mux.HandleFunc("GET /api/v1/groups/{id}", authMW.RequireAuth(requireGroupAccess(groupHandler.GetGroupDetails)))
	mux.HandleFunc("POST /api/v1/groups/{id}/invites", authMW.RequireAuth(requireGroupAccess(groupHandler.GenerateInvite)))


	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		fmt.Printf("🚀 Server is starting on port %s...\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nGracefully shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server exited cleanly")
}
