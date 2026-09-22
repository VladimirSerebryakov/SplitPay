package postgres

import (
	"context"
	"errors"

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
		return err
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
		return nil, err
	}
	return &user, nil
}

func (r *AuthPostgres) CreateRefreshSession(ctx context.Context, session *domain.RefreshSession) error {
	query := `
		INSERT INTO refresh_sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id`

	return r.pool.QueryRow(ctx, query, session.UserID, session.TokenHash, session.ExpiresAt).Scan(&session.ID)
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
		return nil, err
	}

	return &session, nil
}

func (r *AuthPostgres) DeleteRefreshSession(ctx context.Context, token string) error {
	query := `DELETE FROM refresh_sessions WHERE token_hash = $1`

	_, err := r.pool.Exec(ctx, query, token)
	return err
}
