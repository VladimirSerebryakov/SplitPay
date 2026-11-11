package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"splitpay/internal/domain"
	"splitpay/internal/token"
)

type authRepositoryStub struct {
	user                        *domain.User
	createUserErr, getUserErr   error
	revokeUserID                uuid.UUID
	revokeErr, createSessionErr error
	createdUser                 *domain.User
	createdSession              *domain.RefreshSession
	revokedHash                 string
	revokeCtx, createCtx        context.Context
}

func (r *authRepositoryStub) CreateUser(_ context.Context, user *domain.User) error {
	r.createdUser = user
	return r.createUserErr
}
func (r *authRepositoryStub) GetUserByEmail(context.Context, string) (*domain.User, error) {
	return r.user, r.getUserErr
}
func (r *authRepositoryStub) CreateRefreshSession(ctx context.Context, session *domain.RefreshSession) error {
	r.createCtx, r.createdSession = ctx, session
	return r.createSessionErr
}
func (r *authRepositoryStub) RevokeRefreshSession(ctx context.Context, hash string) (uuid.UUID, error) {
	r.revokeCtx, r.revokedHash = ctx, hash
	return r.revokeUserID, r.revokeErr
}

type testTxKey struct{}

// Заглушка проверяет взаимодействие с сервисом; настоящего rollback здесь нет.
type txManagerStub struct {
	called                 bool
	ctx                    context.Context
	callbackErr, finishErr error
}

func (m *txManagerStub) WithTx(ctx context.Context, fn func(context.Context) error) error {
	m.called = true
	m.ctx = context.WithValue(ctx, testTxKey{}, true)
	m.callbackErr = fn(m.ctx)
	if m.callbackErr != nil {
		return m.callbackErr
	}
	return m.finishErr
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
			repo := &authRepositoryStub{createUserErr: tc.repoErr}
			_, err := NewAuthService(repo, "secret", &txManagerStub{}).SignUp(context.Background(), domain.SignUpInput{Name: "Иван", Email: "a@example.com", Password: "password123"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Неверная ошибка: %v", err)
			}
		})
	}
}

func TestSignUpSuccess(t *testing.T) {
	repo := &authRepositoryStub{}
	input := domain.SignUpInput{Name: "Иван", Email: "ivan@example.com", Password: "password123"}
	user, err := NewAuthService(repo, "secret", &txManagerStub{}).SignUp(context.Background(), input)
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
		repo authRepositoryStub
		want error
	}{
		{"нет пользователя", authRepositoryStub{getUserErr: fmt.Errorf("поиск: %w", domain.ErrNotFound)}, domain.ErrInvalidCredentials},
		{"неверный пароль", authRepositoryStub{user: &domain.User{PasswordHash: string(hash)}}, domain.ErrInvalidCredentials},
		{"повреждённый хеш", authRepositoryStub{user: &domain.User{PasswordHash: "broken"}}, bcrypt.ErrHashTooShort},
		{"сбой БД", authRepositoryStub{getUserErr: storageErr}, storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAuthService(&tc.repo, "secret", &txManagerStub{}).SignIn(context.Background(), domain.SignInInput{Email: "a@example.com", Password: "wrong"})
			if !errors.Is(err, tc.want) || tc.repo.createdSession != nil {
				t.Fatalf("Неверная ошибка или создана сессия: %v", err)
			}
		})
	}
}

func TestSignInSession(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"успех", nil},
		{"ошибка сохранения сессии", errors.New("БД недоступна")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &domain.User{ID: uuid.New(), PasswordHash: string(hash)}
			repo := &authRepositoryStub{user: user, createSessionErr: tc.err}
			before := time.Now()
			pair, err := NewAuthService(repo, "secret", &txManagerStub{}).SignIn(context.Background(), domain.SignInInput{Email: "a@example.com", Password: "password123"})
			if tc.err != nil {
				if !errors.Is(err, tc.err) || pair != (domain.TokenPair{}) {
					t.Fatalf("Ожидались ошибка сохранения и пустая пара токенов: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			checkIssuedTokens(t, pair, repo.createdSession, user.ID, before)
		})
	}
}

func TestRefreshSuccess(t *testing.T) {
	repo := &authRepositoryStub{revokeUserID: uuid.New()}
	tx := &txManagerStub{}
	before := time.Now()
	pair, err := NewAuthService(repo, "secret", tx).RefreshTokens(context.Background(), "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if !tx.called || repo.revokeCtx != tx.ctx || repo.createCtx != tx.ctx {
		t.Fatal("Оба запроса должны получить контекст из WithTx")
	}
	if repo.revokedHash != token.Hash("old-refresh") || pair.RefreshToken == "old-refresh" {
		t.Fatal("Нужно отозвать старый токен по хешу и выдать новый")
	}
	checkIssuedTokens(t, pair, repo.createdSession, repo.revokeUserID, before)
}

func TestRefreshErrorTranslation(t *testing.T) {
	storageErr := errors.New("БД недоступна")
	for _, tc := range []struct {
		name      string
		repo      authRepositoryStub
		finishErr error
		want      error
		created   bool
	}{
		{"нет действующей сессии", authRepositoryStub{revokeErr: fmt.Errorf("отзыв: %w", domain.ErrNotFound)}, nil, domain.ErrInvalidRefreshToken, false},
		{"сбой отзыва", authRepositoryStub{revokeErr: storageErr}, nil, storageErr, false},
		{"сбой создания", authRepositoryStub{revokeUserID: uuid.New(), createSessionErr: storageErr}, nil, storageErr, true},
		{"сбой фиксации транзакции", authRepositoryStub{revokeUserID: uuid.New()}, storageErr, storageErr, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &txManagerStub{finishErr: tc.finishErr}
			pair, err := NewAuthService(&tc.repo, "secret", tx).RefreshTokens(context.Background(), "test-token")
			if !errors.Is(err, tc.want) || pair != (domain.TokenPair{}) {
				t.Fatalf("Ожидались ошибка и пустая пара токенов: %v", err)
			}
			if !tx.called || (tc.repo.createdSession != nil) != tc.created {
				t.Fatal("Неверный вызов транзакции или создания сессии")
			}
			if tc.finishErr == nil && !errors.Is(tx.callbackErr, tc.want) {
				t.Fatal("Ошибка должна выйти из callback, чтобы менеджер выполнил rollback")
			}
			if tc.finishErr != nil && tx.callbackErr != nil {
				t.Fatal("Ошибка фиксации проверяется после успешного callback")
			}
		})
	}
}

func TestLogout(t *testing.T) {
	storageErr := errors.New("БД недоступна")
	for _, tc := range []struct {
		name          string
		repoErr, want error
	}{
		{"успех", nil, nil},
		{"сессия уже недействительна или неизвестна", fmt.Errorf("отзыв: %w", domain.ErrNotFound), nil},
		{"сбой БД", storageErr, storageErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &authRepositoryStub{revokeErr: tc.repoErr}
			err := NewAuthService(repo, "secret", &txManagerStub{}).Logout(context.Background(), "refresh-token")
			if !errors.Is(err, tc.want) {
				t.Fatalf("Неверная ошибка: %v", err)
			}
			if repo.revokedHash != token.Hash("refresh-token") {
				t.Fatal("Репозиторий должен получить хеш токена")
			}
		})
	}
}

// Общие проверки выдачи токенов при входе и ротации.
func checkIssuedTokens(t *testing.T, pair domain.TokenPair, session *domain.RefreshSession, userID uuid.UUID, before time.Time) {
	t.Helper()
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.TokenType != "Bearer" || pair.ExpiresIn != 900 {
		t.Fatal("Ожидались непустые токены, тип Bearer и expires_in = 900")
	}
	if session == nil || session.UserID != userID || session.TokenHash != token.Hash(pair.RefreshToken) {
		t.Fatal("Сессия должна содержать пользователя и хеш выданного refresh-токена")
	}
	const lifetime = 30 * 24 * time.Hour
	if session.ExpiresAt.Before(before.Add(lifetime)) || session.ExpiresAt.After(time.Now().Add(lifetime)) {
		t.Fatal("Refresh-сессия должна действовать 30 дней")
	}
}
