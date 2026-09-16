package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"splitpay/internal/domain"
	repo "splitpay/internal/repository/postgres"
)

func setupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}

	// Накатываем структуру таблиц для теста
	schema := `
		CREATE TABLE users (
			id UUID PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);

		CREATE TABLE groups (
			id UUID PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			currency VARCHAR(10) NOT NULL,
			owner_id UUID REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE group_members (
			group_id UUID REFERENCES groups(id) ON DELETE CASCADE,
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			role VARCHAR(50) NOT NULL,
			joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			PRIMARY KEY (group_id, user_id)
		);
	`
	_, err = pool.Exec(ctx, schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	cleanup := func() {
		pool.Close()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	}

	return pool, cleanup
}

func TestGroupRepository_Create_Integration(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	groupRepo := repo.NewGroupRepository(pool)

	ownerID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)", ownerID, "owner@test.com", "hash")
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	group := &domain.Group{
		ID:       uuid.New(),
		Name:     "Test Integration Group",
		Currency: "RUB",
		OwnerID:  ownerID,
	}

	err = groupRepo.Create(ctx, group)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	var savedName string
	err = pool.QueryRow(ctx, "SELECT name FROM groups WHERE id = $1", group.ID).Scan(&savedName)
	if err != nil {
		t.Fatalf("Failed to query saved group: %v", err)
	}
	if savedName != group.Name {
		t.Errorf("Expected group name %s, got %s", group.Name, savedName)
	}

	var role string
	err = pool.QueryRow(ctx, "SELECT role FROM group_members WHERE group_id = $1 AND user_id = $2", group.ID, ownerID).Scan(&role)
	if err != nil {
		t.Fatalf("Failed to query group members: %v", err)
	}
	if role != "owner" {
		t.Errorf("Expected role 'owner', got %s", role)
	}
}

func TestGroupRepository_GetUserGroups_Integration(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	groupRepo := repo.NewGroupRepository(pool)

	// 1. Создаем тестового пользователя
	userID := uuid.New()
	_, err := pool.Exec(ctx, "INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)", userID, "multi@test.com", "hash")
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	// 2. Создаем две группы и добавляем пользователя в обе
	group1 := &domain.Group{ID: uuid.New(), Name: "Group A", Currency: "RUB", OwnerID: userID}
	group2 := &domain.Group{ID: uuid.New(), Name: "Group B", Currency: "RUB", OwnerID: userID}

	err = groupRepo.Create(ctx, group1)
	if err != nil {
		t.Fatalf("failed to create group1: %v", err)
	}
	err = groupRepo.Create(ctx, group2)
	if err != nil {
		t.Fatalf("failed to create group2: %v", err)
	}

	// 3. Тестируемое действие
	groups, err := groupRepo.GetUserGroups(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserGroups() returned error: %v", err)
	}

	// 4. Проверки
	if len(groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(groups))
	}

	// Проверяем, что вернулись нужные имена
	names := make(map[string]bool)
	for _, g := range groups {
		names[g.Name] = true
	}
	if !names["Group A"] || !names["Group B"] {
		t.Errorf("Expected groups 'Group A' and 'Group B' in result, got: %v", groups)
	}
}

func TestGroupRepository_InviteFlow_Integration(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	groupRepo := repo.NewGroupRepository(pool)

	// ВАЖНО: Добавим таблицу group_invites в схему БД для этого теста,
	// так как в setupTestDB из предыдущего сообщения мы ее не создавали.
	// В реальном проекте эта таблица должна создаваться в рамках основной миграции.
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS group_invites (
			group_id UUID PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
			token VARCHAR(255) UNIQUE NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
	`)
	if err != nil {
		t.Fatalf("failed to create group_invites table: %v", err)
	}

	// 1. Подготовка: Владелец и Группа
	ownerID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)", ownerID, "invite_owner@test.com", "hash")
	if err != nil {
		t.Fatalf("failed to insert owner: %v", err)
	}

	groupID := uuid.New()
	group := &domain.Group{ID: groupID, Name: "Invite Test Group", Currency: "RUB", OwnerID: ownerID}
	if err := groupRepo.Create(ctx, group); err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	// 2. Подготовка: Новый пользователь (будущий участник)
	memberID := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)", memberID, "new_member@test.com", "hash")
	if err != nil {
		t.Fatalf("failed to insert member: %v", err)
	}

	// 3. ТЕСТ: UpsertInvite
	token := "secure-invite-token-123"
	expiresAt := time.Now().Add(24 * time.Hour)
	err = groupRepo.UpsertInvite(ctx, groupID, token, expiresAt)
	if err != nil {
		t.Fatalf("UpsertInvite() returned error: %v", err)
	}

	// Проверяем, что инвайт сохранился
	var savedGroupID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT group_id FROM group_invites WHERE token = $1", token).Scan(&savedGroupID)
	if err != nil {
		t.Fatalf("Failed to find inserted invite in DB: %v", err)
	}
	if savedGroupID != groupID {
		t.Errorf("Expected invite for group %s, got %s", groupID, savedGroupID)
	}

	// 4. ТЕСТ: GetInvite
	invite, err := groupRepo.GetInvite(ctx, token)
	if err != nil {
		t.Fatalf("GetInvite() returned error: %v", err)
	}
	if invite.GroupID != groupID {
		t.Errorf("Expected invite group %s, got %s", groupID, invite.GroupID)
	}
	if invite.Token != token {
		t.Errorf("Expected token %s, got %s", token, invite.Token)
	}

	// 5. ТЕСТ: AddUserToGroup
	err = groupRepo.AddUserToGroup(ctx, groupID, memberID)
	if err != nil {
		t.Fatalf("AddUserToGroup() returned error: %v", err)
	}

	// Проверяем, что пользователь реально добавлен
	var role string
	err = pool.QueryRow(ctx, "SELECT role FROM group_members WHERE group_id = $1 AND user_id = $2", groupID, memberID).Scan(&role)
	if err != nil {
		t.Fatalf("Failed to query new group member: %v", err)
	}
	if role != "member" {
		t.Errorf("Expected role 'member', got %s", role)
	}

	// 6. ТЕСТ: IsUserInGroup
	inGroup, err := groupRepo.IsUserInGroup(ctx, groupID, memberID)
	if err != nil {
		t.Fatalf("IsUserInGroup() returned error: %v", err)
	}
	if !inGroup {
		t.Error("IsUserInGroup() returned false, expected true for added member")
	}
}
