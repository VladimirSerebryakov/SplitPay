package service

import (
	"context"
	"errors"
	"fmt"
	"splitpay/internal/domain"

	"github.com/google/uuid"
)

type userRepository interface {
	GetProfileById(ctx context.Context, userID uuid.UUID) (domain.UserProfile, error)
	UpdateName(ctx context.Context, userID uuid.UUID, name string) (domain.UserProfile, error)
}
type UserService struct {
	repo userRepository
}

func NewUserService(repo userRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetMe(ctx context.Context, userID uuid.UUID) (domain.UserProfile, error) {
	user, err := s.repo.GetProfileById(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.UserProfile{}, domain.ErrUnauthorized
		}
		return domain.UserProfile{}, fmt.Errorf("repo.GetUserProfileById: %w", err)
	}
	return user, nil
}

func (s *UserService) UpdateMe(ctx context.Context, userID uuid.UUID, name string) (domain.UserProfile, error) {
	user, err := s.repo.UpdateName(ctx, userID, name)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.UserProfile{}, domain.ErrUnauthorized
		}
		return domain.UserProfile{}, fmt.Errorf("repo.UpdateName: %w", err)
	}
	return user, nil
}
