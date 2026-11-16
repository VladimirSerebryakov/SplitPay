package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"splitpay/internal/domain"
	repo "splitpay/internal/repository/postgres"
	"splitpay/internal/service"
)

func TestIdempotencyIntegration(t *testing.T) {
	pool := setupUserTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manager := repo.NewManager(pool)
	groups := service.NewGroupService(repo.NewGroupRepository(pool), manager)
	idempotency := service.NewIdempotencyService(repo.NewIdempotencyRepository(pool), manager)
	owner := createGroupTestUser(t, pool, "Владелец")
	key := uuid.New()
	keyCtx := service.WithIdempotency(ctx, service.IdempotencyData{Key: key, RequestHash: "original-hash"})
	run := func(txCtx context.Context) (domain.CachedResponse, error) {
		group, err := groups.CreateGroup(txCtx, "Идемпотентная", owner)
		if err != nil {
			return domain.CachedResponse{}, err
		}
		body, err := json.Marshal(map[string]uuid.UUID{"id": group.ID})
		return domain.CachedResponse{ResponseCode: 201, ResponseBody: body}, err
	}

	// Параллельное создание даёт исходную запись для последующих проверок.
	{
		type outcome struct {
			response domain.CachedResponse
			err      error
		}
		const requests = 4
		start := make(chan struct{})
		results := make(chan outcome, requests)
		for range requests {
			go func() {
				<-start
				response, err := idempotency.Execute(keyCtx, owner, run)
				results <- outcome{response, err}
			}()
		}
		close(start)
		var firstID uuid.UUID
		for range requests {
			got := <-results
			if got.err != nil {
				t.Error(got.err)
				continue
			}
			var body struct {
				ID uuid.UUID `json:"id"`
			}
			if err := json.Unmarshal(got.response.ResponseBody, &body); err != nil || body.ID == uuid.Nil {
				t.Errorf("Неверное тело ответа: %s, %v", got.response.ResponseBody, err)
				continue
			}
			if firstID == uuid.Nil {
				firstID = body.ID
			}
			if body.ID != firstID || got.response.ResponseCode != 201 || got.response.RequestHash != "original-hash" {
				t.Errorf("Запрос получил другой результат: %+v", got.response)
			}
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM groups`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("Создано групп: %d, ошибка: %v", count, err)
		}
	}

	t.Run("тот же ключ с другим хешем", func(t *testing.T) {
		changed := service.WithIdempotency(ctx, service.IdempotencyData{Key: key, RequestHash: "changed-hash"})
		_, err := idempotency.Execute(changed, owner, func(context.Context) (domain.CachedResponse, error) {
			t.Fatal("Операция повторно вызвана при несовпадении хеша")
			return domain.CachedResponse{}, nil
		})
		if !errors.Is(err, domain.ErrIdempotencyKeyMismatch) {
			t.Fatalf("Ожидалась ошибка несовпадения ключа: %v", err)
		}
	})

	t.Run("ошибка откатывает группу и ключ", func(t *testing.T) {
		rollback := errors.New("ошибка после создания группы")
		rollbackKey := uuid.New()
		rollbackCtx := service.WithIdempotency(ctx, service.IdempotencyData{Key: rollbackKey, RequestHash: "rollback-hash"})
		_, err := idempotency.Execute(rollbackCtx, owner, func(txCtx context.Context) (domain.CachedResponse, error) {
			if _, err := run(txCtx); err != nil {
				return domain.CachedResponse{}, err
			}
			return domain.CachedResponse{}, rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		var groupsCount, keysCount int
		err = pool.QueryRow(ctx, `SELECT
			(SELECT COUNT(*) FROM groups),
			(SELECT COUNT(*) FROM idempotency_keys WHERE user_id=$1 AND key=$2)`, owner, rollbackKey.String()).
			Scan(&groupsCount, &keysCount)
		if err != nil || groupsCount != 1 || keysCount != 0 {
			t.Fatalf("Остались данные после rollback: групп=%d, ключей=%d, ошибка=%v", groupsCount, keysCount, err)
		}
		if _, err := idempotency.Execute(rollbackCtx, owner, run); err != nil {
			t.Fatalf("После rollback ключ нельзя использовать повторно: %v", err)
		}
	})
}
