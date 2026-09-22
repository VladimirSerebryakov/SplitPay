package postgres

import (
	"context"
	"errors"
	"fmt"

	"splitpay/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthPostgres struct {
	pool *pgxpool.Pool
}

func NewAuthPostgres(pool *pgxpool.Pool) *AuthPostgres {
	return &AuthPostgres{pool: pool}
}

func (r *AuthPostgres) CreateUser(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, created_at`

	err := r.pool.QueryRow(ctx, query, user.Email, user.PasswordHash).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errors.New("user with this email already exists")
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *AuthPostgres) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, created_at
		FROM users
		WHERE email = $1`

	var user domain.User
	err := r.pool.QueryRow(ctx, query, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid email or password")
		}
		// Оборачиваем техническую ошибку
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &user, nil
}

func (r *AuthPostgres) CreateRefreshSession(ctx context.Context, session *domain.RefreshSession) error {
	query := `
		INSERT INTO refresh_sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id`

	err := r.pool.QueryRow(ctx, query, session.UserID, session.TokenHash, session.ExpiresAt).Scan(&session.ID)
	if err != nil {
		return fmt.Errorf("failed to create refresh session: %w", err)
	}
	return nil
}

func (r *AuthPostgres) GetRefreshSession(ctx context.Context, token string) (*domain.RefreshSession, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at
		FROM refresh_sessions
		WHERE token_hash = $1`

	var session domain.RefreshSession
	err := r.pool.QueryRow(ctx, query, token).Scan(&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("session not found")
		}
		return nil, fmt.Errorf("failed to get refresh session: %w", err)
	}

	return &session, nil
}

func (r *AuthPostgres) DeleteRefreshSession(ctx context.Context, token string) error {
	query := `DELETE FROM refresh_sessions WHERE token_hash = $1`

	_, err := r.pool.Exec(ctx, query, token)
	if err != nil {
		return fmt.Errorf("failed to delete refresh session: %w", err)
	}
	return nil
}
