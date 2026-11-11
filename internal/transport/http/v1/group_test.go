package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

type groupAccessStub struct{}

func (groupAccessStub) IsUserInGroup(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
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
					if groupID != validGroupID {
						t.Fatalf("В сервис передан неверный UUID группы: %v", groupID)
					}
					return domain.GroupDetailsResponse{
						ID: groupID, Name: "Detailed Group", Currency: "RUB", OwnerID: testUserID,
					}, nil
				}
			},
			expectedStatus: http.StatusOK,
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
			req.SetPathValue("groupId", tt.pathID)

			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			middleware.RequireGroupAccess(groupAccessStub{})(handler.GetGroupDetails)(rr, req)

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
					if groupID != validGroupID || userID != testUserID {
						t.Fatalf("В сервис переданы неверные UUID: группа=%v, пользователь=%v", groupID, userID)
					}
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
			req.SetPathValue("groupId", tt.pathID)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, testUserID.String())
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			middleware.RequireGroupAccess(groupAccessStub{})(handler.GenerateInvite)(rr, req)

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
			expectedStatus: http.StatusNotFound,
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

func TestGroupHandlerErrorResponses(t *testing.T) {
	internalErr := errors.New("секретная причина сбоя хранилища")
	group := NewGroupHandler(&mockGroupService{
		CreateGroupFunc: func(context.Context, domain.CreateGroupInput, uuid.UUID) (domain.GroupResponse, error) {
			return domain.GroupResponse{}, internalErr
		},
		GetUserGroupsFunc: func(context.Context, uuid.UUID) ([]domain.GroupResponse, error) { return nil, internalErr },
		GetGroupDetailsFunc: func(context.Context, uuid.UUID) (domain.GroupDetailsResponse, error) {
			return domain.GroupDetailsResponse{}, fmt.Errorf("группа: %w", domain.ErrNotFound)
		},
		GenerateInviteFunc: func(context.Context, uuid.UUID, uuid.UUID) (domain.InviteResponse, error) {
			return domain.InviteResponse{}, fmt.Errorf("приглашение: %w", domain.ErrForbidden)
		},
		JoinByInviteFunc: func(context.Context, string, uuid.UUID) error {
			return fmt.Errorf("приглашение: %w", domain.ErrNotFound)
		},
	})
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		body    string
		groupID string
		status  int
		code    string
		field   string
	}{
		{"group JSON", group.CreateGroup, `{`, "", 400, "invalid_body", ""},
		{"group name", group.CreateGroup, `{"name":"  "}`, "", 400, "validation_failed", "name"},
		{"join JSON", group.JoinGroup, `{`, "", 400, "invalid_body", ""},
		{"join token", group.JoinGroup, `{}`, "", 400, "validation_failed", "token"},
		{"create failure", group.CreateGroup, `{"name":"Поездка"}`, "", 500, "internal", ""},
		{"list failure", group.GetUserGroups, "", "", 500, "internal", ""},
		{"group missing", group.GetGroupDetails, "", uuid.NewString(), 404, "not_found", ""},
		{"invite forbidden", group.GenerateInvite, "", uuid.NewString(), 403, "forbidden", ""},
		{"invite missing", group.JoinGroup, `{"token":"wrong"}`, "", 404, "not_found", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(tc.body))
			req.SetPathValue("groupId", tc.groupID)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.NewString()))
			rr := httptest.NewRecorder()
			next := tc.handler
			if tc.groupID != "" {
				next = middleware.RequireGroupAccess(groupAccessStub{})(next)
			}
			next(rr, req)
			checkErrorResponse(t, rr, tc.status, tc.code, tc.field)
		})
	}
}

func TestGroupHandlersRequireUser(t *testing.T) {
	handler := NewGroupHandler(&mockGroupService{})
	for _, invalidID := range []string{"", "wrong"} {
		for _, next := range []http.HandlerFunc{handler.CreateGroup, handler.GetUserGroups, handler.GenerateInvite, handler.JoinGroup} {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", nil)
			req.SetPathValue("groupId", uuid.NewString())
			if invalidID != "" {
				req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, invalidID))
			}
			rr := httptest.NewRecorder()
			next(rr, req)
			checkErrorResponse(t, rr, 401, "unauthorized", "")
		}
	}
}

func TestGroupHandlersRequireGroupContext(t *testing.T) {
	called := false
	handler := NewGroupHandler(&mockGroupService{
		GetGroupDetailsFunc: func(context.Context, uuid.UUID) (domain.GroupDetailsResponse, error) {
			called = true
			return domain.GroupDetailsResponse{}, nil
		},
		GenerateInviteFunc: func(context.Context, uuid.UUID, uuid.UUID) (domain.InviteResponse, error) {
			called = true
			return domain.InviteResponse{}, nil
		},
	})
	for _, tc := range []struct {
		name string
		next http.HandlerFunc
	}{
		{"детали группы", handler.GetGroupDetails},
		{"приглашение", handler.GenerateInvite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.NewString()))
			rr := httptest.NewRecorder()
			tc.next(rr, req)
			if called {
				t.Fatal("Сервис вызван без context группы")
			}
			checkErrorResponse(t, rr, 500, "internal", "")
		})
	}
}
