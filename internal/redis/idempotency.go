package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"splitpay/internal/domain"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

type IdempotencyStorage struct {
	client *goredis.Client
}

func NewIdempotencyStorage(client *goredis.Client) *IdempotencyStorage {
	return &IdempotencyStorage{client: client}
}

func (s *IdempotencyStorage) CheckIdempotency(ctx context.Context,
	userID uuid.UUID, idempotencyKey uuid.UUID,
	requestHash string) (*domain.CachedResponse, error) {
	key := "idem:" + userID.String() + ":" + idempotencyKey.String()
	data, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get idempotency data: %w", err)
	}

	var cached domain.CachedResponse
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("unmarshal idempotency data: %w", err)
	}

	if cached.RequestHash == "" || cached.ResponseCode < 200 || cached.ResponseCode > 299 {
		return nil, errors.New("invalid idempotency data")
	}

	if cached.RequestHash != requestHash {
		return nil, domain.ErrIdempotencyKeyMismatch
	}
	return &cached, nil
}

func (s *IdempotencyStorage) WriteIdempotency(ctx context.Context, userID uuid.UUID,
	idempotencyKey uuid.UUID, requestHash string,
	response domain.CachedResponse) error {
	data, err := json.Marshal(&response)
	if err != nil {
		return fmt.Errorf("marshal idempotency data: %w", err)
	}
	if data == nil || len(data) < 1 {
		return fmt.Errorf("invalid idempotency data")
	}
	key := "idem:" + userID.String() + ":" + idempotencyKey.String()
	if err := s.client.Set(ctx, key, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("set idempotency data: %w", err)
	}
	return nil
}
