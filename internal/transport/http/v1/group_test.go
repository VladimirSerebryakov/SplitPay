package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"splitpay/internal/domain"
	"splitpay/internal/transport/http/middleware"
)

// mockGroupService — расширенный ручной мок для имитации бизнес-логики
type mockGroupService struct {
	CreateGroupFunc     func(ctx context.Context, input domain.CreateGroupInput, userID uuid.UUID) (domain.GroupResponse, error)
	GetUserGroupsFunc   func(ctx context.Context, userID uuid.UUID) ([]domain.GroupResponse, error)
	GetGroupDetailsFunc func(ctx context.Context, groupID uuid.UUID) (domain.GroupDetailsResponse, error)
	GenerateInviteFunc  func(ctx context.Context, groupID, userID uuid.UUID) (domain.InviteResponse, error)
	JoinByInviteFunc    func(ctx context.Context, token string, userID uuid.UUID) error
}

func (m *mockGroupService) CreateGroup(ctx context.Context, input domain.CreateGroupInput, userID uuid.UUID) (domain.GroupResponse, error) {
	if m.CreateGroupFunc != nil {
		return m.CreateGroupFunc(ctx, input, userID)
	}
	return domain.GroupResponse{}, nil
}

func (m *mockGroupService) GetUserGroups(ctx context.Context, userID uuid.UUID) ([]domain.GroupResponse, error) {
	if m.GetUserGroupsFunc != nil {
		return m.GetUserGroupsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockGroupService) GetGroupDetails(ctx context.Context, groupID uuid.UUID) (domain.GroupDetailsResponse, error) {
	if m.GetGroupDetailsFunc != nil {
		return m.GetGroupDetailsFunc(ctx, groupID)
	}
	return domain.GroupDetailsResponse{}, nil
}

func (m *mockGroupService) GenerateInvite(ctx context.Context, groupID, userID uuid.UUID) (domain.InviteResponse, error) {
	if m.GenerateInviteFunc != nil {
		return m.GenerateInviteFunc(ctx, groupID, userID)
	}
	return domain.InviteResponse{}, nil
}

func (m *mockGroupService) JoinByInvite(ctx context.Context, token string, userID uuid.UUID) error {
	if m.JoinByInviteFunc != nil {
		return m.JoinByInviteFunc(ctx, token, userID)
	}
	return nil
}

// === Тесты ===

func TestGroupHandler_CreateGroup(t *testing.T) {
	testUserID := uuid.New()

	tests := []struct {
		name           string
		inputBody      interface{}
		mockBehavior   func(m *mockGroupService)
		expectedStatus int
	}{
		{
			name:      "Success",
			inputBody: domain.CreateGroupInput{Name: "Trip to Berlin"},
			mockBehavior: func(m *mockGroupService) {
				m.CreateGroupFunc = func(ctx context.Context, input domain.CreateGroupInput, userID uuid.UUID) (domain.GroupResponse, error) {
					return domain.GroupResponse{
						ID:       uuid.New(),
						Name:     "Trip to Berlin",
						Currency: "RUB",
						OwnerID:  userID,
					}, nil
				}
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:      "Invalid JSON",
			inputBody: "not-a-json",
			mockBehavior: func(m *mockGroupService) {
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "Service Error",
			inputBody: domain.CreateGroupInput{Name: "Trip to Berlin"},
			mockBehavior: func(m *mockGroupService) {
				m.CreateGroupFunc = func(ctx context.Context, input domain.CreateGroupInput, userID uuid.UUID) (domain.GroupResponse, error) {
					return domain.GroupResponse{}, errors.New("database connection lost")
				}
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockGroupService{}
			tt.mockBehavior(mockService)
			handler := NewGroupHandler(mockService)

			var reqBody []byte
			if s, ok := tt.inputBody.(string); ok {
				reqBody = []byte(s)
			} else {
				reqBody, _ = json.Marshal(tt.inputBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewBuffer(reqBody))
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.CreateGroup(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestGroupHandler_GetUserGroups(t *testing.T) {
	testUserID := uuid.New()

	tests := []struct {
		name           string
		mockBehavior   func(m *mockGroupService)
		expectedStatus int
	}{
		{
			name: "Success",
			mockBehavior: func(m *mockGroupService) {
				m.GetUserGroupsFunc = func(ctx context.Context, userID uuid.UUID) ([]domain.GroupResponse, error) {
					return []domain.GroupResponse{
						{ID: uuid.New(), Name: "Group 1", Currency: "RUB", OwnerID: userID},
					}, nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Service Error",
			mockBehavior: func(m *mockGroupService) {
				m.GetUserGroupsFunc = func(ctx context.Context, userID uuid.UUID) ([]domain.GroupResponse, error) {
					return nil, errors.New("db error")
				}
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockGroupService{}
			tt.mockBehavior(mockService)
			handler := NewGroupHandler(mockService)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.GetUserGroups(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestGroupHandler_GetGroupDetails(t *testing.T) {
	testUserID := uuid.New()
	validGroupID := uuid.New()

	tests := []struct {
		name           string
		pathID         string
		mockBehavior   func(m *mockGroupService)
		expectedStatus int
	}{
		{
			name:   "Success",
			pathID: validGroupID.String(),
			mockBehavior: func(m *mockGroupService) {
				m.GetGroupDetailsFunc = func(ctx context.Context, groupID uuid.UUID) (domain.GroupDetailsResponse, error) {
					return domain.GroupDetailsResponse{
						ID: groupID, Name: "Detailed Group", Currency: "RUB", OwnerID: testUserID,
					}, nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Invalid UUID Format",
			pathID: "invalid-uuid",
			mockBehavior: func(m *mockGroupService) {
				// Метод не должен вызываться
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Not Found / Service Error",
			pathID: validGroupID.String(),
			mockBehavior: func(m *mockGroupService) {
				m.GetGroupDetailsFunc = func(ctx context.Context, groupID uuid.UUID) (domain.GroupDetailsResponse, error) {
					return domain.GroupDetailsResponse{}, domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockGroupService{}
			tt.mockBehavior(mockService)
			handler := NewGroupHandler(mockService)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+tt.pathID, nil)
			// Go 1.22+ позволяет внедрять PathValue напрямую в реквест для тестов
			req.SetPathValue("id", tt.pathID)

			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.GetGroupDetails(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestGroupHandler_GenerateInvite(t *testing.T) {
	testUserID := uuid.New()
	validGroupID := uuid.New()

	tests := []struct {
		name           string
		pathID         string
		mockBehavior   func(m *mockGroupService)
		expectedStatus int
	}{
		{
			name:   "Success",
			pathID: validGroupID.String(),
			mockBehavior: func(m *mockGroupService) {
				m.GenerateInviteFunc = func(ctx context.Context, groupID, userID uuid.UUID) (domain.InviteResponse, error) {
					return domain.InviteResponse{
						InviteLink: "invite-token-123",
						ExpiresAt:  time.Now().Add(24 * time.Hour),
					}, nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "User Not In Group Error",
			pathID: validGroupID.String(),
			mockBehavior: func(m *mockGroupService) {
				m.GenerateInviteFunc = func(ctx context.Context, groupID, userID uuid.UUID) (domain.InviteResponse, error) {
					return domain.InviteResponse{}, domain.ErrForbidden
				}
			},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockGroupService{}
			tt.mockBehavior(mockService)
			handler := NewGroupHandler(mockService)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+tt.pathID+"/invites", nil)
			req.SetPathValue("id", tt.pathID)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.GenerateInvite(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestGroupHandler_JoinGroup(t *testing.T) {
	testUserID := uuid.New()

	tests := []struct {
		name           string
		inputBody      interface{}
		mockBehavior   func(m *mockGroupService)
		expectedStatus int
	}{
		{
			name:      "Success",
			inputBody: domain.JoinGroupInput{Token: "valid-token"},
			mockBehavior: func(m *mockGroupService) {
				m.JoinByInviteFunc = func(ctx context.Context, token string, userID uuid.UUID) error {
					return nil
				}
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "Expired Token",
			inputBody: domain.JoinGroupInput{Token: "expired-token"},
			mockBehavior: func(m *mockGroupService) {
				m.JoinByInviteFunc = func(ctx context.Context, token string, userID uuid.UUID) error {
					return domain.ErrInviteExpired
				}
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "Token Not Found",
			inputBody: domain.JoinGroupInput{Token: "unknown-token"},
			mockBehavior: func(m *mockGroupService) {
				m.JoinByInviteFunc = func(ctx context.Context, token string, userID uuid.UUID) error {
					return domain.ErrNotFound
				}
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockGroupService{}
			tt.mockBehavior(mockService)
			handler := NewGroupHandler(mockService)

			var reqBody []byte
			if s, ok := tt.inputBody.(string); ok {
				reqBody = []byte(s)
			} else {
				reqBody, _ = json.Marshal(tt.inputBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/join", bytes.NewBuffer(reqBody))
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.JoinGroup(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}
