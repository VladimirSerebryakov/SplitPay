package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"splitpay/internal/domain"
	repo "splitpay/internal/repository/postgres"
	"splitpay/internal/service"
	"splitpay/internal/token"
	httptransport "splitpay/internal/transport/http"
	"splitpay/internal/transport/http/middleware"
	"splitpay/internal/transport/http/v1"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createGroupTestUser(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, 'test-hash')`, id, id.String()+"@example.com", name)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGroupServiceIntegration(t *testing.T) {
	// Helper поднимает отдельный PostgreSQL 16 и применяет настоящие миграции.
	pool := setupUserTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	storage := repo.NewGroupRepository(pool)
	manager := repo.NewManager(pool)
	groups := service.NewGroupService(storage, manager)
	owner := createGroupTestUser(t, pool, "Владелец")
	member := createGroupTestUser(t, pool, "Участник")
	outsider := createGroupTestUser(t, pool, "Посторонний")

	group, err := groups.CreateGroup(ctx, "Поездка", owner)
	if err != nil {
		t.Fatal(err)
	}
	if group.ID == uuid.Nil || group.CreatedAt.IsZero() || len(group.Members) != 1 || group.Members[0].UserID != owner || group.Members[0].Name != "Владелец" || group.Members[0].Role != "owner" {
		t.Fatalf("Неверная группа: %+v", group)
	}

	t.Run("проверка доступа и отсутствующей группы", func(t *testing.T) {
		for _, tc := range []struct {
			id   uuid.UUID
			want bool
		}{{owner, true}, {outsider, false}} {
			got, err := storage.IsUserInGroup(ctx, group.ID, tc.id)
			if err != nil || got != tc.want {
				t.Fatalf("Членство=%t, ошибка=%v", got, err)
			}
		}
		if _, err := storage.IsUserInGroup(ctx, uuid.New(), owner); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Ожидалась отсутствующая группа: %v", err)
		}
	})

	t.Run("rollback группы при ошибке добавления owner", func(t *testing.T) {
		if _, err := groups.CreateGroup(ctx, "Откат", uuid.New()); err == nil {
			t.Fatal("Ожидалась ошибка внешнего ключа")
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM groups WHERE name='Откат'`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("Группа осталась после rollback: %d, %v", count, err)
		}
	})

	t.Run("переиспользование внешней транзакции", func(t *testing.T) {
		rollback := errors.New("откат внешней операции")
		var id uuid.UUID
		err := manager.WithTx(ctx, func(txCtx context.Context) error {
			created, err := groups.CreateGroup(txCtx, "Внешняя транзакция", owner)
			if err != nil {
				return err
			}
			id = created.ID
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		if _, err := storage.GetGroupByID(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Вложенная операция закоммитилась отдельно: %v", err)
		}
	})

	issued, err := groups.GenerateInvite(ctx, group.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("безопасное приглашение", func(t *testing.T) {
		raw, err := base64.RawURLEncoding.DecodeString(issued.Token)
		if err != nil || len(raw) != 32 {
			t.Fatal("Токен должен содержать 32 случайных байта")
		}
		stored, err := storage.GetInvite(ctx, token.Hash(issued.Token))
		if err != nil {
			t.Fatal(err)
		}
		if stored.TokenHash == issued.Token || stored.CreatedBy != owner || stored.GroupID != group.ID || !stored.ExpiresAt.Equal(issued.ExpiresAt) {
			t.Fatal("Неверные данные приглашения")
		}
		if remaining := time.Until(issued.ExpiresAt); remaining < 23*time.Hour+59*time.Minute || remaining > 24*time.Hour {
			t.Fatalf("Неверный TTL: %s", remaining)
		}
	})

	t.Run("вступление и параллельные повторы", func(t *testing.T) {
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for range 8 {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := groups.JoinByInvite(ctx, issued.Token, member); errs <- err }()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		details, err := groups.GetGroupDetails(ctx, group.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(details.Members) != 2 {
			t.Fatalf("Участников: %d", len(details.Members))
		}
		if _, err := groups.JoinByInvite(ctx, issued.Token, owner); err != nil {
			t.Fatal(err)
		}
		role, err := storage.GetMemberRole(ctx, group.ID, owner)
		if err != nil || role != "owner" {
			t.Fatalf("Повторное вступление изменило роль owner: %s, %v", role, err)
		}
	})

	t.Run("приглашения доступны только owner", func(t *testing.T) {
		for _, userID := range []uuid.UUID{member, outsider} {
			if _, err := groups.GenerateInvite(ctx, group.ID, userID); !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("Выпуск не владельцем: %v", err)
			}
			if err := groups.RevokeInvite(ctx, group.ID, userID); !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("Отзыв не владельцем: %v", err)
			}
		}
		if _, err := groups.GenerateInvite(ctx, uuid.New(), owner); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
		if err := groups.RevokeInvite(ctx, uuid.New(), owner); !errors.Is(err, domain.ErrNotFound) {
			t.Fatal(err)
		}
	})

	t.Run("истёкшая и неизвестная ссылка", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE group_invites SET expires_at=now()-interval '1 second' WHERE group_id=$1`, group.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := groups.JoinByInvite(ctx, issued.Token, outsider); !errors.Is(err, domain.ErrInviteExpired) {
			t.Fatalf("Истёкшая ссылка: %v", err)
		}
		if _, err := groups.JoinByInvite(ctx, "unknown", outsider); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Неизвестная ссылка: %v", err)
		}
		if joined, err := storage.IsUserInGroup(ctx, group.ID, outsider); err != nil || joined {
			t.Fatal("Посторонний добавлен по неверной ссылке")
		}
	})

	replacement, err := groups.GenerateInvite(ctx, group.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Token == issued.Token {
		t.Fatal("Не заменён токен")
	}
	if _, err := groups.JoinByInvite(ctx, issued.Token, outsider); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Старая ссылка действует: %v", err)
	}

	for _, operation := range []string{"отзыв", "замена"} {
		t.Run("вступление удерживает ссылку/"+operation, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			txCtx := context.WithValue(ctx, repo.TxKey{}, tx)
			if _, err := storage.GetInvite(txCtx, token.Hash(replacement.Token)); err != nil {
				t.Fatal(err)
			}
			deadline, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()
			if operation == "отзыв" {
				err = groups.RevokeInvite(deadline, group.ID, owner)
			} else {
				_, err = groups.GenerateInvite(deadline, group.ID, owner)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Операция не дождалась транзакции вступления: %v", err)
			}
		})
	}

	t.Run("отзыв и повторный отзыв", func(t *testing.T) {
		for range 2 {
			if err := groups.RevokeInvite(ctx, group.ID, owner); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := groups.JoinByInvite(ctx, replacement.Token, outsider); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Отозванная ссылка действует: %v", err)
		}
	})

	t.Run("список с ролью количеством и балансом", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE groups SET created_at='2026-01-01' WHERE id=$1`, group.ID); err != nil {
			t.Fatal(err)
		}
		newer, err := groups.CreateGroup(ctx, "Новая группа", owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO balances (group_id,user_id,net) VALUES ($1,$2,-123)`, group.ID, member); err != nil {
			t.Fatal(err)
		}
		list, err := groups.GetUserGroups(ctx, owner)
		if err != nil || len(list) != 2 || list[0].ID != newer.ID || list[1].ID != group.ID || list[0].MyBalance != 0 || list[1].MembersCount != 2 || list[0].MyRole != "owner" {
			t.Fatalf("Неверный список владельца: %+v, %v", list, err)
		}
		list, err = groups.GetUserGroups(ctx, member)
		if err != nil || len(list) != 1 || list[0].MyRole != "member" || list[0].MyBalance != -123 || list[0].MembersCount != 2 {
			t.Fatalf("Неверный список участника: %+v, %v", list, err)
		}
		list, err = groups.GetUserGroups(ctx, outsider)
		if err != nil || len(list) != 0 {
			t.Fatalf("Посторонний видит группы: %+v, %v", list, err)
		}
	})
}

// Redis заменён заглушками; сервисы и PostgreSQL используются настоящие.
type groupRateCounter struct{}

func (groupRateCounter) Increment(context.Context, uuid.UUID, time.Duration) (int64, time.Duration, error) {
	return 1, time.Minute, nil
}

// Промах кэша заставляет каждый повтор проверять сохранённый результат в БД.
type groupIdempotencyCache struct{}

func (groupIdempotencyCache) CheckIdempotency(context.Context, uuid.UUID, uuid.UUID, string) (*domain.CachedResponse, error) {
	return nil, nil
}

func (groupIdempotencyCache) WriteIdempotency(context.Context, uuid.UUID, uuid.UUID, string, domain.CachedResponse) error {
	return nil
}

func TestGroupRoutesIntegration(t *testing.T) {
	pool := setupUserTestDB(t)
	storage := repo.NewGroupRepository(pool)
	manager := repo.NewManager(pool)
	groups := service.NewGroupService(storage, manager)
	idempotency := service.NewIdempotencyService(repo.NewIdempotencyRepository(pool), manager)
	owner := createGroupTestUser(t, pool, "Владелец")
	member := createGroupTestUser(t, pool, "Участник")
	outsider := createGroupTestUser(t, pool, "Посторонний")
	const secret = "test-group-routes"
	router := httptransport.NewRouter(
		v1.NewAuthHandler(nil), v1.NewGroupHandler(groups, idempotency), v1.NewUserHandler(nil),
		middleware.NewAuthMiddleware(secret), middleware.NewRateLimitMiddleware(groupRateCounter{}, 30, time.Minute),
		middleware.NewIdempotencyMiddleware(groupIdempotencyCache{}), storage,
	)
	call := func(method, path, body string, userID uuid.UUID, want int, key ...string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if len(key) > 0 {
			req.Header.Set("Idempotency-Key", key[0])
		}
		if userID != uuid.Nil {
			signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": userID.String(), "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+signed)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Fatalf("%s %s: ожидался %d, получено %d: %s", method, path, want, rr.Code, rr.Body.String())
		}
		if want == 204 {
			if rr.Body.Len() != 0 {
				t.Fatal("Ответ 204 содержит тело")
			}
		} else if want != 404 || strings.HasPrefix(rr.Body.String(), "{") {
			if rr.Header().Get("Content-Type") != "application/json" {
				t.Fatal("Нет JSON Content-Type")
			}
		}
		return rr
	}
	created := call("POST", "/api/v1/groups", `{"name":"Поездка"}`, owner, 201)
	var group struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		Members   []struct {
			UserID uuid.UUID `json:"user_id"`
			Name   string    `json:"name"`
			Role   string    `json:"role"`
		} `json:"members"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &group); err != nil {
		t.Fatal(err)
	}
	if group.ID == uuid.Nil || group.CreatedAt.IsZero() || len(group.Members) != 1 || group.Members[0].UserID != owner || group.Members[0].Name != "Владелец" || group.Members[0].Role != "owner" {
		t.Fatal("Неверный ответ создания")
	}
	path := "/api/v1/groups/" + group.ID.String()
	call("GET", path, "", owner, 200)
	call("GET", path, "", outsider, 403)
	call("GET", "/api/v1/groups/"+uuid.NewString(), "", owner, 404)
	call("GET", "/api/v1/groups/wrong", "", owner, 400)
	call("GET", path, "", uuid.Nil, 401)
	inviteResponse := call("POST", path+"/invite", "", owner, 201)
	var invite struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(inviteResponse.Body.Bytes(), &invite); err != nil || invite.Token == "" {
		t.Fatal("Нет токена приглашения")
	}
	body := `{"token":"` + invite.Token + `"}`
	joined := call("POST", "/api/v1/groups/join", body, member, 200)
	if strings.Contains(joined.Body.String(), "email") || strings.Contains(joined.Body.String(), "owner_id") || strings.Contains(joined.Body.String(), "currency") {
		t.Fatal("В ответе лишние поля")
	}
	call("POST", "/api/v1/groups/join", body, member, 200)
	call("POST", path+"/invite", "", member, 403)
	call("DELETE", path+"/invite", "", member, 403)
	call("POST", path+"/invites", "", owner, 404)
	call("DELETE", path+"/invite", "", owner, 204)
	call("DELETE", path+"/invite", "", owner, 204)
	call("POST", "/api/v1/groups/join", body, outsider, 404)
	list := call("GET", "/api/v1/groups", "", member, 200)
	var response struct {
		Items []struct {
			ID           uuid.UUID `json:"id"`
			MyRole       string    `json:"my_role"`
			MembersCount int64     `json:"members_count"`
			MyBalance    int64     `json:"my_balance"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil || len(response.Items) != 1 || response.Items[0].MyRole != "member" || response.Items[0].MembersCount != 2 || response.Items[0].MyBalance != 0 {
		t.Fatalf("Неверный ответ списка: %s", list.Body.String())
	}

	// Идемпотентное создание проверяется через PostgreSQL при каждом запросе.
	{
		key := uuid.NewString()
		body := `{"name":"Идемпотентная группа"}`
		first := call("POST", "/api/v1/groups", body, owner, 201, key)
		var initial struct {
			ID      uuid.UUID         `json:"id"`
			Members []json.RawMessage `json:"members"`
		}
		if err := json.Unmarshal(first.Body.Bytes(), &initial); err != nil {
			t.Fatal(err)
		}
		// После изменения группы повтор должен вернуть первоначальный состав.
		issued, err := groups.GenerateInvite(context.Background(), initial.ID, owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := groups.JoinByInvite(context.Background(), issued.Token, member); err != nil {
			t.Fatal(err)
		}
		repeated := call("POST", "/api/v1/groups", body, owner, 201, key)
		var got struct {
			ID      uuid.UUID         `json:"id"`
			Members []json.RawMessage `json:"members"`
		}
		if err := json.Unmarshal(repeated.Body.Bytes(), &got); err != nil || got.ID != initial.ID || len(got.Members) != 1 {
			t.Fatalf("Повтор не вернул первоначальную группу: %s, %v", repeated.Body.String(), err)
		}
		for _, changed := range []string{`{"name":"Другая"}`, `{"name":""}`, `{`} {
			mismatch := call("POST", "/api/v1/groups", changed, owner, 422, key)
			if !strings.Contains(mismatch.Body.String(), `"code":"idempotency_key_mismatch"`) {
				t.Fatalf("Неверная ошибка повтора: %s", mismatch.Body.String())
			}
		}
		// Область ключа ограничена пользователем.
		other := call("POST", "/api/v1/groups", body, outsider, 201, key)
		if err := json.Unmarshal(other.Body.Bytes(), &got); err != nil || got.ID == initial.ID {
			t.Fatalf("Пользователи разделили один результат: %s, %v", other.Body.String(), err)
		}
	}
}
