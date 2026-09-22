package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type RefreshSession struct {
	ID        uuid.UUID `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	TokenHash string    `json:"-" db:"token_hash"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

type Group struct {
	ID       uuid.UUID `json:"id" db:"id"`
	Name     string    `json:"name" db:"name"`
	Currency string    `json:"currency" db:"currency"`
	OwnerID  uuid.UUID `json:"owner_id" db:"owner_id"`
}

type GroupMember struct {
	GroupID  uuid.UUID `json:"group_id" db:"group_id"`
	UserID   uuid.UUID `json:"user_id" db:"user_id"`
	Role     string    `json:"role" db:"role"`
	JoinedAt time.Time `json:"joined_at" db:"joined_at"`
}

type Expense struct {
	ID          uuid.UUID `json:"id" db:"id"`
	GroupID     uuid.UUID `json:"group_id" db:"group_id"`
	PayerID     uuid.UUID `json:"payer_id" db:"payer_id"`
	Amount      int64     `json:"amount" db:"amount"`
	Description string    `json:"description" db:"description"`
	SplitType   string    `json:"split_type" db:"split_type"` // "equal", "exact"
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type ExpenseSplit struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ExpenseID uuid.UUID `json:"expense_id" db:"expense_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Amount    int64     `json:"amount" db:"amount"`
}
