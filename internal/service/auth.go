package service

import (
	"context"
	"errors"
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
		return err
	}

	user := &domain.User{
		Email:        input.Email,
		PasswordHash: string(hash),
	}

	return s.repo.CreateUser(ctx, user)
}

func (s *AuthService) SignIn(ctx context.Context, input domain.SignInInput) (domain.TokenPair, error) {
	user, err := s.repo.GetUserByEmail(ctx, input.Email)
	if err != nil {
		return domain.TokenPair{}, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password))
	if err != nil {
		return domain.TokenPair{}, errors.New("invalid email or password")
	}

	accessToken, err := s.generateAccessToken(user.ID)
	if err != nil {
		return domain.TokenPair{}, err
	}

	refreshToken := uuid.New().String()
	session := &domain.RefreshSession{
		UserID:    user.ID,
		TokenHash: refreshToken,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}

	if err := s.repo.CreateRefreshSession(ctx, session); err != nil {
		return domain.TokenPair{}, err
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
		return domain.TokenPair{}, errors.New("invalid refresh token")
	}

	if session.ExpiresAt.Before(time.Now()) {
		_ = s.repo.DeleteRefreshSession(ctx, refreshToken)
		return domain.TokenPair{}, errors.New("refresh token expired")
	}

	if err := s.repo.DeleteRefreshSession(ctx, refreshToken); err != nil {
		return domain.TokenPair{}, err
	}

	accessToken, err := s.generateAccessToken(session.UserID)
	if err != nil {
		return domain.TokenPair{}, err
	}

	newRefreshToken := uuid.New().String()
	newSession := &domain.RefreshSession{
		UserID:    session.UserID,
		TokenHash: newRefreshToken,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}

	if err := s.repo.CreateRefreshSession(ctx, newSession); err != nil {
		return domain.TokenPair{}, err
	}

	return domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}
