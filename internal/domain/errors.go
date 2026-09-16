package domain

import "errors"

// Общие ошибки хранилища
var (
	ErrNotFound      = errors.New("record not found")
	ErrAlreadyExists = errors.New("record already exists")
)

// Бизнес-ошибки
var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrEmailTaken          = errors.New("user with this email already exists")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrInviteExpired       = errors.New("invite token has expired")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
)
