package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	GroupRoleOwner  = "owner"
	GroupRoleMember = "member"
)

type Group struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

type GroupSummary struct {
	ID           uuid.UUID
	Name         string
	MyRole       string
	MembersCount int64
	MyBalance    int64
}

type GroupMemberInfo struct {
	UserID   uuid.UUID
	Name     string
	Role     string
	JoinedAt time.Time
}

type GroupDetails struct {
	Group
	Members []GroupMemberInfo
}

type GroupInvite struct {
	GroupID   uuid.UUID
	TokenHash string
	CreatedBy uuid.UUID
	ExpiresAt time.Time
	CreatedAt time.Time
}

// IssuedInvite содержит открытый токен только для ответа на его выпуск.
type IssuedInvite struct {
	Token     string
	ExpiresAt time.Time
}
