package domain

import (
	"errors"
	"time"
)

// Общие ошибки хранилища
var (
	ErrNotFound      = errors.New("Сущность не найдена")
	ErrAlreadyExists = errors.New("Запись уже существует")
)

// Бизнес-ошибки
var (
	ErrInvalidBody            = errors.New("Некорректное тело запроса")
	ErrValidationFailed       = errors.New("Некорректные данные запроса")
	ErrInvalidCredentials     = errors.New("Неверный email или пароль")
	ErrEmailTaken             = errors.New("Этот email уже занят")
	ErrUnauthorized           = errors.New("Требуется авторизация")
	ErrForbidden              = errors.New("Доступ запрещён")
	ErrInviteExpired          = errors.New("Срок действия приглашения истёк")
	ErrInvalidRefreshToken    = errors.New("Refresh-токен неверный, отозван или истёк")
	ErrIdempotencyKeyMismatch = errors.New("Ключ идемпотентности уже использован для другого запроса")
	ErrTooManyParticipants    = errors.New("Слишком много участников для точного алгоритма")
	ErrRateLimited            = errors.New("Превышен лимит запросов")
)

// ValidationError содержит сообщения для полей, которые необходимо исправить.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return ErrValidationFailed.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidationFailed }

// RateLimitError сообщает, сколько нужно подождать перед повторным запросом.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return ErrRateLimited.Error() }
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }
