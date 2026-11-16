package postgres

import (
	"context"
	"fmt"
	"splitpay/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IdempotencyRepository struct {
	db *pgxpool.Pool
}

func NewIdempotencyRepository(db *pgxpool.Pool) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) TryCreate(ctx context.Context,
	userID, key uuid.UUID, requestHash string) (bool, error) {
	query := `
		INSERT INTO idempotency_keys (user_id, key, request_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key) DO NOTHING
	`

	q := querierFrom(ctx, r.db)
	result, err := q.Exec(ctx, query, userID, key.String(), requestHash)
	if err != nil {
		return false, fmt.Errorf("insert idempotency key: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *IdempotencyRepository) Get(ctx context.Context,
	userID, key uuid.UUID) (domain.CachedResponse, error) {
	query := `
		SELECT request_hash, response_code, response_body
		FROM idempotency_keys WHERE user_id = $1 AND key = $2
	`

	q := querierFrom(ctx, r.db)
	var result domain.CachedResponse
	var code *int
	var body []byte
	err := q.QueryRow(ctx, query, userID, key.String()).Scan(
		&result.RequestHash,
		&code,
		&body)
	if err != nil {
		return domain.CachedResponse{}, fmt.Errorf("get idempotency key: %w", err)
	}
	if code == nil || *code < 200 || *code >= 300 {
		return domain.CachedResponse{}, fmt.Errorf("Сохранён незавершенный результат идемпотентности")
	}
	result.ResponseCode = *code
	result.ResponseBody = body
	return result, nil
}

func (r *IdempotencyRepository) SaveResponse(ctx context.Context,
	userID, key uuid.UUID, response domain.CachedResponse) error {
	query := `UPDATE idempotency_keys 
		SET response_code = $4, response_body = $5
		WHERE user_id = $1 
		  AND key = $2 
		  AND request_hash = $3 
		  AND response_code IS NULL
		`
	q := querierFrom(ctx, r.db)
	tag, err := q.Exec(ctx, query, userID, key.String(),
		response.RequestHash, response.ResponseCode,
		response.ResponseBody)
	if err != nil {
		return fmt.Errorf("update idempotency key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("Не удалось завершить запись идемпотентности")
	}
	return nil
}
