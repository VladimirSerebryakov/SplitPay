package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL       string
	Port              string
	JWTSecret         string
	RedisAddr         string
	RateLimitRequests int64
	RateLimitWindow   time.Duration
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Значение по умолчанию
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		return nil, fmt.Errorf("Необходимо указать REDIS_ADDR")
	}

	limit := int64(30)
	if val := os.Getenv("RATE_LIMIT_REQUESTS"); val != "" {
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("RATE_LIMIT_REQUESTS должен быть натуральным числом")
		}
		limit = parsed
	}

	window := time.Minute
	if val := os.Getenv("RATE_LIMIT_WINDOW"); val != "" {
		parsed, err := time.ParseDuration(val)
		if err != nil || parsed < time.Millisecond {
			return nil, fmt.Errorf("RATE_LIMIT_WINDOW должен быть длительностью не менее 1ms")
		}
		window = parsed
	}

	return &Config{
		DatabaseURL:       dbURL,
		Port:              port,
		JWTSecret:         jwtSecret,
		RedisAddr:         redisAddr,
		RateLimitRequests: limit,
		RateLimitWindow:   window,
	}, nil
}
