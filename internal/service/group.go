package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"splitpay/internal/domain"
)

type GroupRepository interface {
	Create(ctx context.Context, group *domain.Group) error
	GetGroupByID(ctx context.Context, groupID uuid.UUID) (*domain.Group, error)
	GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.Group, error)
	GetGroupMembers(ctx context.Context, groupID uuid.UUID) ([]domain.GroupMemberInfo, error)
	IsUserInGroup(ctx context.Context, groupID, userID uuid.UUID) (bool, error)
	AddUserToGroup(ctx context.Context, groupID, userID uuid.UUID) error
	UpsertInvite(ctx context.Context, groupID uuid.UUID, token string, expiresAt time.Time) error
	GetInvite(ctx context.Context, token string) (*domain.GroupInvite, error)
}

type GroupService struct {
	repo GroupRepository
}

func NewGroupService(repo GroupRepository) *GroupService {
	return &GroupService{repo: repo}
}

func (s *GroupService) CreateGroup(ctx context.Context, input domain.CreateGroupInput, userID uuid.UUID) (domain.GroupResponse, error) {
	group := &domain.Group{
		ID:       uuid.New(),
		Name:     input.Name,
		Currency: "RUB",
		OwnerID:  userID,
	}

	if err := s.repo.Create(ctx, group); err != nil {
		return domain.GroupResponse{}, fmt.Errorf("repo.Create: %w", err)
	}

	return domain.GroupResponse{
		ID:       group.ID,
		Name:     group.Name,
		Currency: group.Currency,
		OwnerID:  group.OwnerID,
	}, nil
}

func (s *GroupService) GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupResponse, error) {
	groups, err := s.repo.GetUserGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("repo.GetUserGroups: %w", err)
	}

	response := make([]domain.GroupResponse, 0, len(groups))
	for _, g := range groups {
		response = append(response, domain.GroupResponse{
			ID:       g.ID,
			Name:     g.Name,
			Currency: g.Currency,
			OwnerID:  g.OwnerID,
		})
	}

	return response, nil
}

func (s *GroupService) GetGroupDetails(ctx context.Context, groupID uuid.UUID) (domain.GroupDetailsResponse, error) {
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return domain.GroupDetailsResponse{}, fmt.Errorf("repo.GetGroupByID: %w", err)
	}

	members, err := s.repo.GetGroupMembers(ctx, groupID)
	if err != nil {
		return domain.GroupDetailsResponse{}, fmt.Errorf("repo.GetGroupMembers: %w", err)
	}

	return domain.GroupDetailsResponse{
		ID:       group.ID,
		Name:     group.Name,
		Currency: group.Currency,
		OwnerID:  group.OwnerID,
		Members:  members,
	}, nil
}

func (s *GroupService) GenerateInvite(ctx context.Context, groupID, userID uuid.UUID) (domain.InviteResponse, error) {
	isInGroup, err := s.repo.IsUserInGroup(ctx, groupID, userID)
	if err != nil {
		return domain.InviteResponse{}, fmt.Errorf("repo.IsUserInGroup: %w", err)
	}
	if !isInGroup {
		return domain.InviteResponse{}, domain.ErrForbidden
	}

	token := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)

	if err := s.repo.UpsertInvite(ctx, groupID, token, expiresAt); err != nil {
		return domain.InviteResponse{}, fmt.Errorf("repo.UpsertInvite: %w", err)
	}

	return domain.InviteResponse{
		InviteLink: token,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *GroupService) JoinByInvite(ctx context.Context, token string, userID uuid.UUID) error {
	invite, err := s.repo.GetInvite(ctx, token)
	if err != nil {
		return fmt.Errorf("repo.GetInvite: %w", err)
	}

	if time.Now().After(invite.ExpiresAt) {
		return domain.ErrInviteExpired
	}

	if err := s.repo.AddUserToGroup(ctx, invite.GroupID, userID); err != nil {
		return fmt.Errorf("repo.AddUserToGroup: %w", err)
	}

	return nil
}
