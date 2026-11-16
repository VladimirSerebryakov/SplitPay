package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"splitpay/internal/domain"
	"splitpay/internal/transport/http/middleware"
	"splitpay/internal/transport/http/response"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type GroupService interface {
	CreateGroup(ctx context.Context, name string, userID uuid.UUID) (domain.GroupDetails, error)
	GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupSummary, error)
	GetGroupDetails(ctx context.Context, groupID uuid.UUID) (domain.GroupDetails, error)
	GenerateInvite(ctx context.Context, groupID, userID uuid.UUID) (domain.IssuedInvite, error)
	RevokeInvite(ctx context.Context, groupID, userID uuid.UUID) error
	JoinByInvite(ctx context.Context, token string, userID uuid.UUID) (domain.GroupDetails, error)
}

type IdempotencyService interface {
	Execute(ctx context.Context,
		userID uuid.UUID,
		run func(context.Context) (domain.CachedResponse, error)) (domain.CachedResponse, error)
}

type GroupHandler struct {
	service     GroupService
	idempotency IdempotencyService
	validate    *validator.Validate
}

func NewGroupHandler(service GroupService, idempotency IdempotencyService) *GroupHandler {
	return &GroupHandler{service: service, idempotency: idempotency, validate: newValidator()}
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
	defer r.Body.Close()
	result, err := h.idempotency.Execute(
		r.Context(),
		userID,
		func(txCtx context.Context) (domain.CachedResponse, error) {
			var input createGroupRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				return domain.CachedResponse{}, domain.ErrInvalidBody
			}
			input.Name = strings.TrimSpace(input.Name)
			if err := h.validate.Struct(input); err != nil {
				return domain.CachedResponse{}, validationError(err)
			}
			group, err := h.service.CreateGroup(txCtx, input.Name, userID)
			if err != nil {
				return domain.CachedResponse{}, err
			}

			body, err := json.Marshal(groupDetailsDTO(group))
			if err != nil {
				return domain.CachedResponse{},
					fmt.Errorf("формирование ответа создания группы: %w", err)
			}

			return domain.CachedResponse{
				ResponseCode: http.StatusCreated,
				ResponseBody: body,
			}, nil
		},
	)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.ResponseCode)

	if _, err := w.Write(result.ResponseBody); err != nil {
		slog.ErrorContext(r.Context(), "Не удалось отправить ответ", "error", err)
	}
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
	result := groupListResponse{Items: make([]groupSummaryResponse, 0, len(groups))}
	for _, group := range groups {
		result.Items = append(result.Items, groupSummaryResponse{
			ID: group.ID, Name: group.Name, MyRole: group.MyRole,
			MembersCount: group.MembersCount, MyBalance: group.MyBalance,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /api/v1/groups/{groupId}
func (h *GroupHandler) GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	groupID, err := middleware.GetGroupID(r.Context())
	if err != nil {
		response.WriteError(w, r, err)
		return
	}
	group, err := h.service.GetGroupDetails(r.Context(), groupID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groupDetailsDTO(group))
}

// POST /api/v1/groups/{groupId}/invite
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
	invite, err := h.service.GenerateInvite(r.Context(), groupID, userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(inviteResponse{Token: invite.Token, ExpiresAt: invite.ExpiresAt})
}

// DELETE /api/v1/groups/{groupId}/invite
func (h *GroupHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
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
	if err := h.service.RevokeInvite(r.Context(), groupID, userID); err != nil {
		response.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	defer r.Body.Close()
	var input joinGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, r, domain.ErrInvalidBody)
		return
	}
	if err := h.validate.Struct(input); err != nil {
		response.WriteError(w, r, validationError(err))
		return
	}
	group, err := h.service.JoinByInvite(r.Context(), input.Token, userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groupDetailsDTO(group))
}
