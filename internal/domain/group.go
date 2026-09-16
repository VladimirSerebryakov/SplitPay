package domain

import (
	"time"

	"github.com/google/uuid"
)

type Group struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Currency string    `json:"currency"`
	OwnerID  uuid.UUID `json:"owner_id"`
}

type GroupMember struct {
	GroupID  uuid.UUID `json:"group_id"`
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"` // 'owner' или 'member'
	JoinedAt time.Time `json:"joined_at"`
}

type GroupInvite struct {
	GroupID   uuid.UUID `json:"group_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateGroupInput struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

type GroupResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Currency string    `json:"currency"`
	OwnerID  uuid.UUID `json:"owner_id"`
}

type GroupMemberInfo struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type GroupDetailsResponse struct {
	ID       uuid.UUID         `json:"id"`
	Name     string            `json:"name"`
	Currency string            `json:"currency"`
	OwnerID  uuid.UUID         `json:"owner_id"`
	Members  []GroupMemberInfo `json:"members"`
}

type InviteResponse struct {
	InviteLink string    `json:"invite_link"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type JoinGroupInput struct {
	Token string `json:"token" validate:"required"`
}
