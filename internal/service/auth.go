package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"splitpay/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour
)

type AuthRepository interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateRefreshSession(ctx context.Context, session *domain.RefreshSession) error
	GetRefreshSession(ctx context.Context, token string) (*domain.RefreshSession, error)
	DeleteRefreshSession(ctx context.Context, token string) error
}

type AuthService struct {
	repo      AuthRepository
	jwtSecret string
}

func NewAuthService(repo AuthRepository, jwtSecret string) *AuthService {
	return &AuthService{
		repo:      repo,
		jwtSecret: jwtSecret,
	}
}

func (s *AuthService) SignUp(ctx context.Context, input domain.SignUpInput) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user := &domain.User{
		Email:        input.Email,
		PasswordHash: string(hash),
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			return domain.ErrEmailTaken
		}
		return fmt.Errorf("repo.CreateUser: %w", err)
	}
	return nil
}

func (s *AuthService) SignIn(ctx context.Context, input domain.SignInInput) (domain.TokenPair, error) {
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.TokenPair{}, domain.ErrInvalidCredentials
		}
		return domain.TokenPair{}, fmt.Errorf("repo.GetUserByEmail: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password))
	if err != nil {
		return domain.TokenPair{}, domain.ErrInvalidCredentials
	}

	accessToken, err := s.generateAccessToken(user.ID)
	if err != nil {
		return domain.TokenPair{}, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken := uuid.New().String()
	session := &domain.RefreshSession{
		UserID:    user.ID,
		TokenHash: refreshToken,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}

	if err := s.repo.CreateRefreshSession(ctx, session); err != nil {
		return domain.TokenPair{}, fmt.Errorf("repo.CreateRefreshSession: %w", err)
	}

	return domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) generateAccessToken(userID uuid.UUID) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID.String(),
		"exp": time.Now().Add(accessTokenTTL).Unix(),
		"iat": time.Now().Unix(),
	})
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (domain.TokenPair, error) {
	session, err := s.repo.GetRefreshSession(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.TokenPair{}, domain.ErrInvalidRefreshToken
		}
		return domain.TokenPair{}, fmt.Errorf("repo.GetRefreshSession: %w", err)
	}

	if session.ExpiresAt.Before(time.Now()) {
		if err := s.repo.DeleteRefreshSession(ctx, refreshToken); err != nil {
			if !errors.Is(err, domain.ErrNotFound) {
				return domain.TokenPair{}, fmt.Errorf("repo.DeleteRefreshSession: %w", err)
			}
		}
		return domain.TokenPair{}, domain.ErrInvalidRefreshToken
	}

	if err := s.repo.DeleteRefreshSession(ctx, refreshToken); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.TokenPair{}, domain.ErrInvalidRefreshToken
		}
		return domain.TokenPair{}, fmt.Errorf("repo.DeleteRefreshSession: %w", err)
	}

	accessToken, err := s.generateAccessToken(session.UserID)
	if err != nil {
		return domain.TokenPair{}, fmt.Errorf("failed to generate access token: %w", err)
	}

	newRefreshToken := uuid.New().String()
	newSession := &domain.RefreshSession{
		UserID:    session.UserID,
		TokenHash: newRefreshToken,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}

	if err := s.repo.CreateRefreshSession(ctx, newSession); err != nil {
		return domain.TokenPair{}, fmt.Errorf("repo.CreateRefreshSession: %w", err)
	}

	return domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}
