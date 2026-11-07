package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"splitpay/internal/domain"
)

type authErrorRepository struct {
	user                                                       *domain.User
	session                                                    *domain.RefreshSession
	createUserErr, getUserErr, getSessionErr, deleteSessionErr error
	createdSession                                             bool
	createdUser                                                *domain.User
}

func (r *authErrorRepository) CreateUser(_ context.Context, user *domain.User) error {
	r.createdUser = user
	return r.createUserErr
}
func (r *authErrorRepository) GetUserByEmail(context.Context, string) (*domain.User, error) {
	return r.user, r.getUserErr
}
func (r *authErrorRepository) CreateRefreshSession(context.Context, *domain.RefreshSession) error {
	r.createdSession = true
	return nil
}
func (r *authErrorRepository) GetRefreshSession(context.Context, string) (*domain.RefreshSession, error) {
	return r.session, r.getSessionErr
}
func (r *authErrorRepository) DeleteRefreshSession(context.Context, string) error {
	return r.deleteSessionErr
}

func TestSignUpErrorTranslation(t *testing.T) {
	storageErr := errors.New("БД недоступна")
	for _, tc := range []struct {
		name          string
		repoErr, want error
	}{
		{"email занят", fmt.Errorf("запись: %w", domain.ErrAlreadyExists), domain.ErrEmailTaken},
		{"сбой БД", storageErr, storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &authErrorRepository{createUserErr: tc.repoErr}
			_, err := NewAuthService(repo, "secret").SignUp(context.Background(), domain.SignUpInput{Name: "Иван", Email: "a@example.com", Password: "password123"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Неверная ошибка: %v", err)
			}
		})
	}
}

func TestSignUpSuccess(t *testing.T) {
	repo := &authErrorRepository{}
	input := domain.SignUpInput{Name: "Иван", Email: "ivan@example.com", Password: "password123"}
	user, err := NewAuthService(repo, "secret").SignUp(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user != repo.createdUser || user.Name != input.Name || user.Email != input.Email {
		t.Fatal("Сервис должен сохранить и вернуть пользователя с исходным именем и email")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		t.Fatalf("В репозиторий должен передаваться bcrypt-хеш пароля: %v", err)
	}
}

func TestSignInErrorTranslation(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	storageErr := errors.New("БД недоступна")
	for _, tc := range []struct {
		name string
		repo authErrorRepository
		want error
	}{
		{"нет пользователя", authErrorRepository{getUserErr: fmt.Errorf("поиск: %w", domain.ErrNotFound)}, domain.ErrInvalidCredentials},
		{"неверный пароль", authErrorRepository{user: &domain.User{PasswordHash: string(hash)}}, domain.ErrInvalidCredentials},
		{"повреждённый хеш", authErrorRepository{user: &domain.User{PasswordHash: "broken"}}, bcrypt.ErrHashTooShort},
		{"сбой БД", authErrorRepository{getUserErr: storageErr}, storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAuthService(&tc.repo, "secret").SignIn(context.Background(), domain.SignInInput{Email: "a@example.com", Password: "wrong"})
			if !errors.Is(err, tc.want) || tc.repo.createdSession {
				t.Fatalf("Неверная ошибка или создана сессия: %v", err)
			}
		})
	}
}

func TestRefreshErrorTranslation(t *testing.T) {
	storageErr := errors.New("БД недоступна")
	for _, tc := range []struct {
		name string
		repo authErrorRepository
		want error
	}{
		{"нет токена", authErrorRepository{getSessionErr: fmt.Errorf("поиск: %w", domain.ErrNotFound)}, domain.ErrInvalidRefreshToken},
		{"истёкший токен", authErrorRepository{session: &domain.RefreshSession{ExpiresAt: time.Now().Add(-time.Hour)}}, domain.ErrInvalidRefreshToken},
		{"истёкший токен уже удалён", authErrorRepository{session: &domain.RefreshSession{ExpiresAt: time.Now().Add(-time.Hour)}, deleteSessionErr: domain.ErrNotFound}, domain.ErrInvalidRefreshToken},
		{"токен уже использован", authErrorRepository{session: &domain.RefreshSession{ExpiresAt: time.Now().Add(time.Hour)}, deleteSessionErr: domain.ErrNotFound}, domain.ErrInvalidRefreshToken},
		{"сбой поиска", authErrorRepository{getSessionErr: storageErr}, storageErr},
		{"сбой удаления", authErrorRepository{session: &domain.RefreshSession{ExpiresAt: time.Now().Add(time.Hour)}, deleteSessionErr: storageErr}, storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAuthService(&tc.repo, "secret").RefreshTokens(context.Background(), "test-token")
			if !errors.Is(err, tc.want) || tc.repo.createdSession {
				t.Fatalf("Неверная ошибка или создана сессия: %v", err)
			}
		})
	}
}
