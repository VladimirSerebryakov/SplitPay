package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"splitpay/internal/domain"
)

type GroupRepository struct {
	db *pgxpool.Pool
}

func NewGroupRepository(db *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{db: db}
}

func (r *GroupRepository) Create(ctx context.Context, group *domain.Group) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queryGroup := `
		INSERT INTO groups (id, name, currency, owner_id)
		VALUES ($1, $2, $3, $4)
	`
	_, err = tx.Exec(ctx, queryGroup, group.ID, group.Name, group.Currency, group.OwnerID)
	if err != nil {
		return fmt.Errorf("failed to insert group: %w", err)
	}

	queryMember := `
		INSERT INTO group_members (group_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`
	_, err = tx.Exec(ctx, queryMember, group.ID, group.OwnerID)
	if err != nil {
		return fmt.Errorf("failed to insert group owner as member: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *GroupRepository) GetGroupByID(ctx context.Context, groupID uuid.UUID) (*domain.Group, error) {
	query := `
		SELECT id, name, currency, owner_id
		FROM groups
		WHERE id = $1
	`
	var g domain.Group
	err := r.db.QueryRow(ctx, query, groupID).Scan(&g.ID, &g.Name, &g.Currency, &g.OwnerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("group not found")
		}
		return nil, fmt.Errorf("failed to get group by id: %w", err)
	}
	return &g, nil
}

func (r *GroupRepository) GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.Group, error) {
	query := `
		SELECT g.id, g.name, g.currency, g.owner_id
		FROM groups g
		JOIN group_members gm ON g.id = gm.group_id
		WHERE gm.user_id = $1
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}
	defer rows.Close()

	var groups []domain.Group
	for rows.Next() {
		var g domain.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Currency, &g.OwnerID); err != nil {
			return nil, fmt.Errorf("failed to scan group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (r *GroupRepository) GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]domain.GroupMemberInfo, error) {
	query := `
		SELECT u.id, u.email, gm.role, gm.joined_at
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		WHERE gm.group_id = $1
	`
	rows, err := r.db.Query(ctx, query, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group members: %w", err)
	}
	defer rows.Close()

	var members []domain.GroupMemberInfo
	for rows.Next() {
		var m domain.GroupMemberInfo
		if err := rows.Scan(&m.ID, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

func (r *GroupRepository) IsUserInGroup(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM group_members
			WHERE group_id = $1 AND user_id = $2
		)
	`
	var exists bool
	err := r.db.QueryRow(ctx, query, groupID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user membership: %w", err)
	}
	return exists, nil
}

func (r *GroupRepository) AddUserToGroup(ctx context.Context, groupID, userID uuid.UUID) error {
	query := `
		INSERT INTO group_members (group_id, user_id, role)
		VALUES ($1, $2, 'member')
		ON CONFLICT (group_id, user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, groupID, userID)
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w", err)
	}
	return nil
}

func (r *GroupRepository) UpsertInvite(ctx context.Context, groupID uuid.UUID, token string, expiresAt time.Time) error {
	query := `
		INSERT INTO group_invites (group_id, token, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (group_id) DO UPDATE
		SET token = EXCLUDED.token, expires_at = EXCLUDED.expires_at, created_at = now()
	`
	_, err := r.db.Exec(ctx, query, groupID, token, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to upsert invite: %w", err)
	}
	return nil
}

func (r *GroupRepository) GetInvite(ctx context.Context, token string) (*domain.GroupInvite, error) {
	query := `
		SELECT group_id, token, expires_at, created_at
		FROM group_invites
		WHERE token = $1
	`
	var inv domain.GroupInvite
	err := r.db.QueryRow(ctx, query, token).Scan(&inv.GroupID, &inv.Token, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}
	return &inv, nil
}
