package service

import (
	"context"
	"fmt"
	"time"

	"splitpay/internal/domain"
	"splitpay/internal/token"

	"github.com/google/uuid"
)

type GroupRepository interface {
	Create(ctx context.Context, group *domain.Group) error
	GetGroupByID(ctx context.Context, groupID uuid.UUID) (*domain.Group, error)
	GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupSummary, error)
	GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]domain.GroupMemberInfo, error)
	GetMemberRole(ctx context.Context, groupID, userID uuid.UUID) (string, error)
	AddMember(ctx context.Context, groupID, userID uuid.UUID, role string) (bool, error)
	UpsertInvite(ctx context.Context, invite *domain.GroupInvite) error
	GetInvite(ctx context.Context, tokenHash string) (*domain.GroupInvite, error)
	DeleteInvite(ctx context.Context, groupID uuid.UUID) error
}

type GroupService struct {
	repo GroupRepository
	tx   TxManager
}

func NewGroupService(repo GroupRepository, tx TxManager) *GroupService {
	return &GroupService{repo: repo, tx: tx}
}

func (s *GroupService) CreateGroup(ctx context.Context, name string, userID uuid.UUID) (domain.GroupDetails, error) {
	group := domain.Group{ID: uuid.New(), Name: name}
	var result domain.GroupDetails
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.Create(txCtx, &group); err != nil {
			return fmt.Errorf("Создание группы: %w", err)
		}
		if _, err := s.repo.AddMember(txCtx, group.ID, userID, domain.GroupRoleOwner); err != nil {
			return fmt.Errorf("Добавление владельца группы: %w", err)
		}
		var err error
		result, err = s.GetGroupDetails(txCtx, group.ID)
		return err
	})
	if err != nil {
		return domain.GroupDetails{}, err
	}
	return result, nil
}

func (s *GroupService) GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupSummary, error) {
	groups, err := s.repo.GetUserGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("Получение групп пользователя: %w", err)
	}
	return groups, nil
}

func (s *GroupService) GetGroupDetails(ctx context.Context, groupID uuid.UUID) (domain.GroupDetails, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return domain.GroupDetails{}, fmt.Errorf("Получение группы: %w", err)
	}
	members, err := s.repo.GetGroupMembers(ctx, groupID)
	if err != nil {
		return domain.GroupDetails{}, fmt.Errorf("Получение участников группы: %w", err)
	}
	return domain.GroupDetails{Group: *group, Members: members}, nil
}

func (s *GroupService) requireOwner(ctx context.Context, groupID, userID uuid.UUID) error {
	role, err := s.repo.GetMemberRole(ctx, groupID, userID)
	if err != nil {
		return fmt.Errorf("Проверка владельца группы: %w", err)
	}
	if role != domain.GroupRoleOwner {
		return domain.ErrForbidden
	}
	return nil
}

func (s *GroupService) GenerateInvite(ctx context.Context, groupID, userID uuid.UUID) (domain.IssuedInvite, error) {
	plain, hash, err := token.New()
	if err != nil {
		return domain.IssuedInvite{}, fmt.Errorf("Создание токена приглашения: %w", err)
	}
	invite := domain.GroupInvite{
		GroupID: groupID, TokenHash: hash, CreatedBy: userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := s.requireOwner(txCtx, groupID, userID); err != nil {
			return err
		}
		if err := s.repo.UpsertInvite(txCtx, &invite); err != nil {
			return fmt.Errorf("Сохранение приглашения: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.IssuedInvite{}, err
	}
	return domain.IssuedInvite{Token: plain, ExpiresAt: invite.ExpiresAt}, nil
}

func (s *GroupService) RevokeInvite(ctx context.Context, groupID, userID uuid.UUID) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if err := s.requireOwner(txCtx, groupID, userID); err != nil {
			return err
		}
		if err := s.repo.DeleteInvite(txCtx, groupID); err != nil {
			return fmt.Errorf("Отзыв приглашения: %w", err)
		}
		return nil
	})
}

func (s *GroupService) JoinByInvite(ctx context.Context, plain string, userID uuid.UUID) (domain.GroupDetails, error) {
	hash := token.Hash(plain)
	var result domain.GroupDetails
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		invite, err := s.repo.GetInvite(txCtx, hash)
		if err != nil {
			return fmt.Errorf("Получение приглашения: %w", err)
		}
		if !time.Now().Before(invite.ExpiresAt) {
			return domain.ErrInviteExpired
		}
		if _, err := s.repo.AddMember(txCtx, invite.GroupID, userID, domain.GroupRoleMember); err != nil {
			return fmt.Errorf("Вступление в группу: %w", err)
		}
		result, err = s.GetGroupDetails(txCtx, invite.GroupID)
		return err
	})
	if err != nil {
		return domain.GroupDetails{}, err
	}
	return result, nil
}
