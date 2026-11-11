package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"splitpay/internal/domain"
	"splitpay/internal/transport/http/middleware"
	"splitpay/internal/transport/http/response"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type GroupService interface {
	CreateGroup(ctx context.Context, input domain.CreateGroupInput, userID uuid.UUID) (domain.GroupResponse, error)
	GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupResponse, error)
	GetGroupDetails(ctx context.Context, groupID uuid.UUID) (domain.GroupDetailsResponse, error)
	GenerateInvite(ctx context.Context, groupID, userID uuid.UUID) (domain.InviteResponse, error)
	JoinByInvite(ctx context.Context, token string, userID uuid.UUID) error
}

type GroupHandler struct {
	service  GroupService
	validate *validator.Validate
}

func NewGroupHandler(service GroupService) *GroupHandler {
	return &GroupHandler{service: service, validate: newValidator()}
}

// POST /api/v1/groups
func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userIDStr, err := middleware.GetUserID(r.Context())
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	var input domain.CreateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	defer r.Body.Close()

	input.Name = strings.TrimSpace(input.Name)

	if err := h.validate.Struct(input); err != nil {
		response.WriteError(w, r, validationError(err))
		return
	}

	resp, err := h.service.CreateGroup(r.Context(), input, userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// GET /api/v1/groups
func (h *GroupHandler) GetUserGroups(w http.ResponseWriter, r *http.Request) {
	userIDStr, err := middleware.GetUserID(r.Context())
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	groups, err := h.service.GetUserGroups(r.Context(), userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

// GET /api/v1/groups/{groupId}
func (h *GroupHandler) GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	groupID, err := middleware.GetGroupID(r.Context())
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	details, err := h.service.GetGroupDetails(r.Context(), groupID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(details)
}

// POST /api/v1/groups/{groupId}/invites
func (h *GroupHandler) GenerateInvite(w http.ResponseWriter, r *http.Request) {
	userIDStr, err := middleware.GetUserID(r.Context())
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	groupID, err := middleware.GetGroupID(r.Context())
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	inviteResp, err := h.service.GenerateInvite(r.Context(), groupID, userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inviteResp)
}

// POST /api/v1/groups/join
func (h *GroupHandler) JoinGroup(w http.ResponseWriter, r *http.Request) {
	userIDStr, err := middleware.GetUserID(r.Context())
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		response.WriteError(w, r, domain.ErrUnauthorized)
		return
	}

	var input domain.JoinGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	defer r.Body.Close()

	if err := h.validate.Struct(input); err != nil {
		response.WriteError(w, r, validationError(err))
		return
	}

	if err := h.service.JoinByInvite(r.Context(), input.Token, userID); err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "successfully joined the group"}`))
}
