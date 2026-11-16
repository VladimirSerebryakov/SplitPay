package v1

import (
	"time"

	"splitpay/internal/domain"

	"github.com/google/uuid"
)

type createGroupRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type joinGroupRequest struct {
	Token string `json:"token" validate:"required"`
}

type groupMemberResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type groupDetailsResponse struct {
	ID        uuid.UUID             `json:"id"`
	Name      string                `json:"name"`
	CreatedAt time.Time             `json:"created_at"`
	Members   []groupMemberResponse `json:"members"`
}

type groupSummaryResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	MyRole       string    `json:"my_role"`
	MembersCount int64     `json:"members_count"`
	MyBalance    int64     `json:"my_balance"`
}

type groupListResponse struct {
	Items []groupSummaryResponse `json:"items"`
}

type inviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func groupDetailsDTO(group domain.GroupDetails) groupDetailsResponse {
	members := make([]groupMemberResponse, 0, len(group.Members))
	for _, member := range group.Members {
		members = append(members, groupMemberResponse{
			UserID: member.UserID, Name: member.Name, Role: member.Role, JoinedAt: member.JoinedAt,
		})
	}
	return groupDetailsResponse{ID: group.ID, Name: group.Name, CreatedAt: group.CreatedAt, Members: members}
}
