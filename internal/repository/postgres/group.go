package postgres

import (
	"context"
	"errors"
	"fmt"

	"splitpay/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupRepository struct {
	db *pgxpool.Pool
}

func NewGroupRepository(db *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{db: db}
}

func (r *GroupRepository) Create(ctx context.Context, group *domain.Group) error {
	query := `
		INSERT INTO groups (id, name) VALUES ($1, $2)
		RETURNING created_at
	`
	q := querierFrom(ctx, r.db)

	err := q.QueryRow(ctx, query, group.ID, group.Name).Scan(&group.CreatedAt)
	if err != nil {
		return fmt.Errorf("Запись группы: %w", err)
	}
	return nil
}

func (r *GroupRepository) GetGroupByID(ctx context.Context, groupID uuid.UUID) (*domain.Group, error) {
	var group domain.Group
	query := `
		SELECT id, name, created_at FROM groups WHERE id = $1
	`
	q := querierFrom(ctx, r.db)

	err := q.QueryRow(ctx, query, groupID).Scan(&group.ID, &group.Name, &group.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("Чтение группы: %w", err)
	}
	return &group, nil
}

func (r *GroupRepository) GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupSummary, error) {
	query := `
		SELECT g.id, g.name, gm.role,
		       (SELECT COUNT(*) FROM group_members members WHERE members.group_id = g.id),
		       COALESCE(b.net, 0)
		FROM groups g
		JOIN group_members gm ON gm.group_id = g.id AND gm.user_id = $1
		LEFT JOIN balances b ON b.group_id = g.id AND b.user_id = gm.user_id
		ORDER BY g.created_at DESC, g.id DESC
	`
	q := querierFrom(ctx, r.db)

	rows, err := q.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("Чтение списка групп: %w", err)
	}
	defer rows.Close()
	groups := make([]domain.GroupSummary, 0)
	for rows.Next() {
		var group domain.GroupSummary
		if err := rows.Scan(&group.ID, &group.Name, &group.MyRole, &group.MembersCount, &group.MyBalance); err != nil {
			return nil, fmt.Errorf("Чтение строки списка групп: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Чтение списка групп: %w", err)
	}
	return groups, nil
}

func (r *GroupRepository) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]domain.GroupMemberInfo, error) {
	query := `
		SELECT gm.user_id, u.name, gm.role, gm.joined_at
		FROM group_members gm
		JOIN users u ON u.id = gm.user_id
		WHERE gm.group_id = $1
		ORDER BY (gm.role = 'owner') DESC, gm.joined_at, gm.user_id
	`
	q := querierFrom(ctx, r.db)

	rows, err := q.Query(ctx, query, groupID)
	if err != nil {
		return nil, fmt.Errorf("Чтение участников группы: %w", err)
	}
	defer rows.Close()
	members := make([]domain.GroupMemberInfo, 0)
	for rows.Next() {
		var member domain.GroupMemberInfo
		if err := rows.Scan(&member.UserID, &member.Name, &member.Role, &member.JoinedAt); err != nil {
			return nil, fmt.Errorf("Чтение участника группы: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Чтение участников группы: %w", err)
	}
	return members, nil
}

func (r *GroupRepository) IsUserInGroup(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	var member bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM group_members gm WHERE gm.group_id = g.id AND gm.user_id = $2
		)
		FROM groups g WHERE g.id = $1
	`
	q := querierFrom(ctx, r.db)

	err := q.QueryRow(ctx, query, groupID, userID).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("Проверка членства в группе: %w", err)
	}
	return member, nil
}

func (r *GroupRepository) GetMemberRole(ctx context.Context, groupID, userID uuid.UUID) (string, error) {
	var role string
	query := `
		SELECT COALESCE(gm.role, '') FROM groups g
		LEFT JOIN group_members gm ON gm.group_id = g.id AND gm.user_id = $2
		WHERE g.id = $1
	`
	q := querierFrom(ctx, r.db)

	err := q.QueryRow(ctx, query, groupID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("Чтение роли участника: %w", err)
	}
	return role, nil
}

func (r *GroupRepository) AddMember(ctx context.Context, groupID, userID uuid.UUID, role string) (bool, error) {
	query := `
		INSERT INTO group_members (group_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (group_id, user_id) DO NOTHING
	`
	q := querierFrom(ctx, r.db)

	result, err := q.Exec(ctx, query, groupID, userID, role)
	if err != nil {
		return false, fmt.Errorf("Запись участника группы: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *GroupRepository) UpsertInvite(ctx context.Context, invite *domain.GroupInvite) error {
	query := `
		INSERT INTO group_invites (group_id, token_hash, created_by, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (group_id) DO UPDATE
		SET token_hash = EXCLUDED.token_hash, created_by = EXCLUDED.created_by,
		    expires_at = EXCLUDED.expires_at, created_at = now()
		RETURNING created_at, expires_at
	`
	q := querierFrom(ctx, r.db)

	err := q.QueryRow(ctx, query, invite.GroupID, invite.TokenHash, invite.CreatedBy, invite.ExpiresAt).Scan(&invite.CreatedAt, &invite.ExpiresAt)
	if err != nil {
		return fmt.Errorf("Запись приглашения: %w", err)
	}
	return nil
}

func (r *GroupRepository) GetInvite(ctx context.Context, tokenHash string) (*domain.GroupInvite, error) {
	var invite domain.GroupInvite
	// До конца транзакции вступления ссылка защищена от замены и отзыва.
	query := `
		SELECT group_id, token_hash, created_by, expires_at, created_at
		FROM group_invites WHERE token_hash = $1 FOR SHARE
	`
	q := querierFrom(ctx, r.db)

	err := q.QueryRow(ctx, query, tokenHash).Scan(&invite.GroupID, &invite.TokenHash, &invite.CreatedBy, &invite.ExpiresAt, &invite.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("Чтение приглашения: %w", err)
	}
	return &invite, nil
}

func (r *GroupRepository) DeleteInvite(ctx context.Context, groupID uuid.UUID) error {
	query := `DELETE FROM group_invites WHERE group_id = $1`
	q := querierFrom(ctx, r.db)

	_, err := q.Exec(ctx, query, groupID)
	if err != nil {
		return fmt.Errorf("Удаление приглашения: %w", err)
	}
	return nil
}
