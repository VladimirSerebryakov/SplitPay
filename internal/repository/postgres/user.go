package postgres

import (
	"context"
	"errors"
	"splitpay/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserPostgres struct {
	pool *pgxpool.Pool
}

func NewUserPostgres(pool *pgxpool.Pool) *UserPostgres {
	return &UserPostgres{pool}
}

func (u *UserPostgres) GetProfileById(ctx context.Context, userID uuid.UUID) (domain.UserProfile, error) {
	query := `
		SELECT u.id, u.email, u.name, u.created_at,
		EXISTS (SELECT 1 FROM telegram_links WHERE telegram_links.user_id = u.id) AS telegram_linked
		FROM users u WHERE u.id = $1`

	var userProfile domain.UserProfile
	err := u.pool.QueryRow(ctx, query, userID).Scan(&userProfile.ID, &userProfile.Email, &userProfile.Name, &userProfile.CreatedAt, &userProfile.TelegramLinked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserProfile{}, domain.ErrNotFound
		}
		return domain.UserProfile{}, err
	}
	return userProfile, nil
}

func (u *UserPostgres) UpdateName(ctx context.Context, userID uuid.UUID, name string) (domain.UserProfile, error) {
	query := `UPDATE users AS u SET name = $1 WHERE id = $2 
		RETURNING u.email, u.created_at, 
		EXISTS (SELECT 1 FROM telegram_links tl 
		WHERE tl.user_id = u.id) AS telegram_linked`

	var userProfile domain.UserProfile

	err := u.pool.QueryRow(ctx, query, name, userID).Scan(&userProfile.Email, &userProfile.CreatedAt, &userProfile.TelegramLinked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserProfile{}, domain.ErrNotFound
		}
		return domain.UserProfile{}, err
	}
	userProfile.Name = name
	userProfile.ID = userID

	return userProfile, nil
}
