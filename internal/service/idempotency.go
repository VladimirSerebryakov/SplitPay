package service

import (
	"context"
	"splitpay/internal/domain"

	"github.com/google/uuid"
)

type IdempotencyData struct {
	Key         uuid.UUID
	RequestHash string
}

type idempotencyContextKey struct{}

func WithIdempotency(ctx context.Context, data IdempotencyData) context.Context {
	return context.WithValue(ctx, idempotencyContextKey{}, data)
}

func IdempotencyFromContext(ctx context.Context) (IdempotencyData, bool) {
	data, ok := ctx.Value(idempotencyContextKey{}).(IdempotencyData)
	return data, ok
}

type IdempotencyRepository interface {
	TryCreate(
		ctx context.Context,
		userID, key uuid.UUID,
		requestHash string,
	) (bool, error)

	Get(
		ctx context.Context,
		userID, key uuid.UUID,
	) (domain.CachedResponse, error)

	SaveResponse(
		ctx context.Context,
		userID, key uuid.UUID,
		response domain.CachedResponse,
	) error
}

type IdempotencyService struct {
	repo IdempotencyRepository
	tx   TxManager
}

func NewIdempotencyService(
	repo IdempotencyRepository,
	tx TxManager,
) *IdempotencyService {
	return &IdempotencyService{repo: repo, tx: tx}
}

func (s *IdempotencyService) Execute(ctx context.Context,
	userID uuid.UUID,
	run func(context.Context) (domain.CachedResponse, error)) (domain.CachedResponse, error) {
	data, enable := IdempotencyFromContext(ctx)
	var result domain.CachedResponse
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if enable {
			created, err := s.repo.TryCreate(txCtx, userID, data.Key, data.RequestHash)
			if err != nil {
				return err
			}
			if !created {
				saved, err := s.repo.Get(txCtx, userID, data.Key)
				if err != nil {
					return err
				}
				if saved.RequestHash != data.RequestHash {
					return domain.ErrIdempotencyKeyMismatch
				}
				result = saved
				return nil
			}
		}
		response, err := run(txCtx)
		if err != nil {
			return err
		}
		if enable {
			response.RequestHash = data.RequestHash
			if err := s.repo.SaveResponse(txCtx, userID, data.Key, response); err != nil {
				return err
			}
		}
		result = response
		return nil
	})
	if err != nil {
		return domain.CachedResponse{}, err
	}
	return result, nil
}
