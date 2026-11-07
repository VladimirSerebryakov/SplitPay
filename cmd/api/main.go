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
	httptransport "splitpay/internal/transport/http"
	"splitpay/internal/transport/http/middleware"
	"splitpay/internal/transport/http/v1"

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
	router := httptransport.NewRouter(authHandler, groupHandler, authMW, groupRepo)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
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
