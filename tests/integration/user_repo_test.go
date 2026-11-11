package integration_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"splitpay/internal/domain"
	repo "splitpay/internal/repository/postgres"
)

func TestUserRepository(t *testing.T) {
	pool := setupUserTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	userRepo := repo.NewUserPostgres(pool)
	users := []domain.UserProfile{
		{ID: uuid.New(), Email: "ivan@example.com", Name: "Иван", CreatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)},
		{ID: uuid.New(), Email: "maria@example.com", Name: "Мария", CreatedAt: time.Date(2026, 11, 2, 13, 0, 0, 0, time.UTC), TelegramLinked: true},
	}
	for _, user := range users {
		_, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, created_at)
			VALUES ($1, $2, $3, $4, $5)`, user.ID, user.Email, user.Name, "test-hash", user.CreatedAt)
		if err != nil {
			t.Fatalf("Не удалось создать пользователя: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO telegram_links (user_id, chat_id) VALUES ($1, $2)`, users[1].ID, int64(123456789)); err != nil {
		t.Fatalf("Не удалось добавить Telegram-привязку: %v", err)
	}

	// Привязка второго пользователя не должна влиять на профиль первого.
	for _, user := range users {
		t.Run("получение профиля/"+user.Name, func(t *testing.T) {
			got, err := userRepo.GetProfileById(ctx, user.ID)
			if err != nil {
				t.Fatal(err)
			}
			checkUserProfile(t, got, user)
		})
	}

	for i, user := range users {
		t.Run("изменение имени/"+user.Name, func(t *testing.T) {
			updated := user
			updated.Name = "Новое имя " + user.Name
			got, err := userRepo.UpdateName(ctx, user.ID, updated.Name)
			if err != nil {
				t.Fatal(err)
			}
			checkUserProfile(t, got, updated)

			// Независимый SELECT проверяет сохранение и отсутствие изменений у соседа.
			for j, other := range users {
				want := other
				if j == i {
					want = updated
				}
				var email, name string
				var createdAt time.Time
				err := pool.QueryRow(ctx, `SELECT email, name, created_at FROM users WHERE id = $1`, want.ID).
					Scan(&email, &name, &createdAt)
				if err != nil {
					t.Fatal(err)
				}
				if email != want.Email || name != want.Name || !createdAt.Equal(want.CreatedAt) {
					t.Fatalf("Неверные сохранённые данные пользователя %s: %s, %s, %v", want.ID, email, name, createdAt)
				}
			}
			// Следующий сценарий учитывает уже сохранённое имя первого пользователя.
			users[i] = updated
		})
	}

	t.Run("неизвестный пользователь", func(t *testing.T) {
		missingID := uuid.New()
		if _, err := userRepo.GetProfileById(ctx, missingID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Получение отсутствующего профиля: ожидался ErrNotFound, получено %v", err)
		}
		if _, err := userRepo.UpdateName(ctx, missingID, "Новое имя"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Обновление отсутствующего профиля: ожидался ErrNotFound, получено %v", err)
		}
	})
}

func checkUserProfile(t *testing.T, got, want domain.UserProfile) {
	t.Helper()
	if got.ID != want.ID || got.Email != want.Email || got.Name != want.Name ||
		!got.CreatedAt.Equal(want.CreatedAt) || got.TelegramLinked != want.TelegramLinked {
		t.Fatalf("Неверный профиль: получено %+v, ожидалось %+v", got, want)
	}
}

// Отдельная БД с актуальными миграциями; старый setupTestDB тестов групп не используется.
func setupUserTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("Не удалось найти миграции: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("profile_test"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.WithOrderedInitScripts(migrations...),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("Не удалось запустить тестовый PostgreSQL: %v", err)
	}
	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Не удалось получить адрес тестовой БД: %v", err)
	}
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		t.Fatalf("Не удалось создать пул тестовой БД: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Тестовая БД недоступна: %v", err)
	}
	return pool
}
