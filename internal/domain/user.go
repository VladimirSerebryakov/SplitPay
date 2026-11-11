package domain

import (
	"net/mail"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserProfile struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	TelegramLinked bool      `json:"telegram_linked"`
}

type SignUpInput struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type SignInInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (input SignUpInput) Validate() error {
	password := []byte(input.Password)
	name := []rune(input.Name)
	email := []rune(input.Email)

	var validateErr *ValidationError

	if len(password) < 8 || len(password) > 72 {
		validateErr = &ValidationError{Fields: make(map[string]string)}
		validateErr.Fields["password"] = "Пароль должен содержать от 8 до 72 байт UTF-8"
	}
	if len(name) < 1 || len(name) > 50 {
		if validateErr == nil {
			validateErr = &ValidationError{Fields: make(map[string]string)}
		}
		validateErr.Fields["name"] = "Имя должно содержать от 1 до 50 символов"
	}

	if len(email) > 254 {
		if validateErr == nil {
			validateErr = &ValidationError{Fields: make(map[string]string)}
		}
		validateErr.Fields["email"] = "Email не должен превышать 254 символа"
	} else if _, err := mail.ParseAddress(input.Email); err != nil {
		if validateErr == nil {
			validateErr = &ValidationError{Fields: make(map[string]string)}
		}
		validateErr.Fields["email"] = "Неверный формат email адреса"
	}

	if validateErr != nil {
		return validateErr
	}
	return nil
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type RefreshSession struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}
