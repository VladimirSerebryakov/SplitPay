package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"splitpay/internal/domain"
	"splitpay/internal/transport/http/middleware"

	"github.com/google/uuid"
)

type mockGroupService struct {
	create  func(context.Context, string, uuid.UUID) (domain.GroupDetails, error)
	list    func(context.Context, uuid.UUID) ([]domain.GroupSummary, error)
	details func(context.Context, uuid.UUID) (domain.GroupDetails, error)
	invite  func(context.Context, uuid.UUID, uuid.UUID) (domain.IssuedInvite, error)
	revoke  func(context.Context, uuid.UUID, uuid.UUID) error
	join    func(context.Context, string, uuid.UUID) (domain.GroupDetails, error)
}

func (m *mockGroupService) CreateGroup(ctx context.Context, name string, id uuid.UUID) (domain.GroupDetails, error) {
	return m.create(ctx, name, id)
}
func (m *mockGroupService) GetUserGroups(ctx context.Context, id uuid.UUID) ([]domain.GroupSummary, error) {
	return m.list(ctx, id)
}
func (m *mockGroupService) GetGroupDetails(ctx context.Context, id uuid.UUID) (domain.GroupDetails, error) {
	return m.details(ctx, id)
}
func (m *mockGroupService) GenerateInvite(ctx context.Context, groupID, userID uuid.UUID) (domain.IssuedInvite, error) {
	return m.invite(ctx, groupID, userID)
}
func (m *mockGroupService) RevokeInvite(ctx context.Context, groupID, userID uuid.UUID) error {
	return m.revoke(ctx, groupID, userID)
}
func (m *mockGroupService) JoinByInvite(ctx context.Context, plain string, id uuid.UUID) (domain.GroupDetails, error) {
	return m.join(ctx, plain, id)
}

type groupAccessStub struct{}

func (groupAccessStub) IsUserInGroup(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

// Обычные тесты handler выполняют операцию; повторы проверяются отдельно.
type groupIdempotencyStub struct {
	result *domain.CachedResponse
	err    error
}

func (s groupIdempotencyStub) Execute(ctx context.Context, _ uuid.UUID,
	run func(context.Context) (domain.CachedResponse, error)) (domain.CachedResponse, error) {
	if s.err != nil {
		return domain.CachedResponse{}, s.err
	}
	if s.result != nil {
		return *s.result, nil
	}
	return run(ctx)
}

func TestGroupHandlers(t *testing.T) {
	internalErr := errors.New("секретная причина сбоя хранилища")
	for _, tc := range []struct {
		name, action, body string
		err                error
		status             int
		code, field        string
	}{
		{"создание", "create", `{"name":"  Поездка  "}`, nil, 201, "", ""},
		{"имя 100 символов", "create", `{"name":"` + strings.Repeat("я", 100) + `"}`, nil, 201, "", ""},
		{"имя 101 символ", "create", `{"name":"` + strings.Repeat("я", 101) + `"}`, nil, 400, "validation_failed", "name"},
		{"пустое имя", "create", `{"name":"  "}`, nil, 400, "validation_failed", "name"},
		{"неверный JSON", "create", `{`, nil, 400, "invalid_body", ""},
		{"ошибка создания", "create", `{"name":"Поездка"}`, internalErr, 500, "internal", ""},
		{"список", "list", "", nil, 200, "", ""},
		{"ошибка списка", "list", "", internalErr, 500, "internal", ""},
		{"детали", "details", "", nil, 200, "", ""},
		{"группа не найдена", "details", "", domain.ErrNotFound, 404, "not_found", ""},
		{"приглашение", "invite", "", nil, 201, "", ""},
		{"приглашение только owner", "invite", "", domain.ErrForbidden, 403, "forbidden", ""},
		{"ошибка приглашения", "invite", "", internalErr, 500, "internal", ""},
		{"отзыв", "revoke", "", nil, 204, "", ""},
		{"отзыв только owner", "revoke", "", domain.ErrForbidden, 403, "forbidden", ""},
		{"ошибка отзыва", "revoke", "", internalErr, 500, "internal", ""},
		{"вступление", "join", `{"token":"secret-token"}`, nil, 200, "", ""},
		{"ссылка истекла", "join", `{"token":"secret-token"}`, domain.ErrInviteExpired, 400, "invite_expired", ""},
		{"ссылка не найдена", "join", `{"token":"secret-token"}`, domain.ErrNotFound, 404, "not_found", ""},
		{"нет токена", "join", `{}`, nil, 400, "validation_failed", "token"},
		{"JSON вступления", "join", `{`, nil, 400, "invalid_body", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID, groupID := uuid.New(), uuid.New()
			created := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
			group := domain.GroupDetails{Group: domain.Group{ID: groupID, Name: "Поездка", CreatedAt: created}, Members: []domain.GroupMemberInfo{{UserID: userID, Name: "Иван", Role: "owner", JoinedAt: created}}}
			service := &mockGroupService{
				create: func(_ context.Context, name string, id uuid.UUID) (domain.GroupDetails, error) {
					if id != userID || name != strings.TrimSpace(name) {
						t.Fatal("Неверные данные создания")
					}
					return group, tc.err
				},
				list: func(context.Context, uuid.UUID) ([]domain.GroupSummary, error) {
					return []domain.GroupSummary{{ID: groupID, Name: "Поездка", MyRole: "owner", MembersCount: 2, MyBalance: -123}}, tc.err
				},
				details: func(_ context.Context, id uuid.UUID) (domain.GroupDetails, error) {
					if id != groupID {
						t.Fatal("Неверный UUID группы")
					}
					return group, tc.err
				},
				invite: func(_ context.Context, g, u uuid.UUID) (domain.IssuedInvite, error) {
					if g != groupID || u != userID {
						t.Fatal("Неверные UUID приглашения")
					}
					return domain.IssuedInvite{Token: "secret-token", ExpiresAt: created.Add(24 * time.Hour)}, tc.err
				},
				revoke: func(_ context.Context, g, u uuid.UUID) error {
					if g != groupID || u != userID {
						t.Fatal("Неверные UUID отзыва")
					}
					return tc.err
				},
				join: func(_ context.Context, plain string, id uuid.UUID) (domain.GroupDetails, error) {
					if plain != "secret-token" || id != userID {
						t.Fatal("Неверные данные вступления")
					}
					return group, tc.err
				},
			}
			h := NewGroupHandler(service, groupIdempotencyStub{})
			handlers := map[string]http.HandlerFunc{"create": h.CreateGroup, "list": h.GetUserGroups, "details": h.GetGroupDetails, "invite": h.GenerateInvite, "revoke": h.RevokeInvite, "join": h.JoinGroup}
			next := handlers[tc.action]
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(tc.body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID.String()))
			if tc.action == "details" || tc.action == "invite" || tc.action == "revoke" {
				req.SetPathValue("groupId", groupID.String())
				next = middleware.RequireGroupAccess(groupAccessStub{})(next)
			}
			rr := httptest.NewRecorder()
			next(rr, req)
			if tc.status >= 400 {
				checkErrorResponse(t, rr, tc.status, tc.code, tc.field)
				return
			}
			if rr.Code != tc.status {
				t.Fatalf("Статус %d, ожидался %d: %s", rr.Code, tc.status, rr.Body.String())
			}
			if tc.status == 204 {
				if rr.Body.Len() != 0 {
					t.Fatal("Ответ 204 содержит тело")
				}
				return
			}
			if rr.Header().Get("Content-Type") != "application/json" {
				t.Fatal("Нет JSON Content-Type")
			}
			switch tc.action {
			case "list":
				var got groupListResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if len(got.Items) != 1 || got.Items[0].ID != groupID || got.Items[0].MyRole != "owner" || got.Items[0].MembersCount != 2 || got.Items[0].MyBalance != -123 {
					t.Fatalf("Неверный список: %s", rr.Body.String())
				}
			case "invite":
				var got inviteResponse
				decoder := json.NewDecoder(rr.Body)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&got); err != nil {
					t.Fatal(err)
				}
				if got.Token != "secret-token" || !got.ExpiresAt.Equal(created.Add(24*time.Hour)) {
					t.Fatal("Неверное приглашение")
				}
			default:
				var got groupDetailsResponse
				decoder := json.NewDecoder(rr.Body)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&got); err != nil {
					t.Fatal(err)
				}
				if got.ID != groupID || got.Name != "Поездка" || !got.CreatedAt.Equal(created) || len(got.Members) != 1 {
					t.Fatal("Неверные детали группы")
				}
				member := got.Members[0]
				if member.UserID != userID || member.Name != "Иван" || member.Role != "owner" || !member.JoinedAt.Equal(created) {
					t.Fatal("Неверный участник")
				}
			}
		})
	}
}

func TestGroupListEmpty(t *testing.T) {
	h := NewGroupHandler(&mockGroupService{list: func(context.Context, uuid.UUID) ([]domain.GroupSummary, error) { return nil, nil }}, groupIdempotencyStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.NewString()))
	rr := httptest.NewRecorder()
	h.GetUserGroups(rr, req)
	if strings.TrimSpace(rr.Body.String()) != `{"items":[]}` {
		t.Fatalf("Неверный пустой список: %s", rr.Body.String())
	}
}

func TestGroupHandlersMissingContext(t *testing.T) {
	// Вызов сервиса здесь недопустим: у заглушки намеренно нет обработчиков.
	h := NewGroupHandler(&mockGroupService{}, groupIdempotencyStub{})
	for _, next := range []http.HandlerFunc{h.CreateGroup, h.GetUserGroups, h.GenerateInvite, h.RevokeInvite, h.JoinGroup} {
		for _, userID := range []string{"", "wrong"} {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", nil)
			if userID != "" {
				req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
			}
			rr := httptest.NewRecorder()
			next(rr, req)
			checkErrorResponse(t, rr, 401, "unauthorized", "")
		}
	}
	for _, next := range []http.HandlerFunc{h.GetGroupDetails, h.GenerateInvite, h.RevokeInvite} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.NewString()))
		rr := httptest.NewRecorder()
		next(rr, req)
		checkErrorResponse(t, rr, 500, "internal", "")
	}
}

func TestCreateGroupSavedResult(t *testing.T) {
	saved := domain.CachedResponse{
		ResponseCode: http.StatusCreated,
		ResponseBody: json.RawMessage(`{"id":"f57fbdfb-297f-4f88-ae17-fc946411cb53","name":"Поездка","created_at":"2026-11-16T12:00:00Z","members":[]}`),
	}
	for _, tc := range []struct {
		name   string
		body   string
		err    error
		status int
		code   string
	}{
		{"сохранённый ответ", `{"name":"Поездка"}`, nil, 201, ""},
		{"другое тело запроса", `{`, domain.ErrIdempotencyKeyMismatch, 422, "idempotency_key_mismatch"},
		{"ошибка транзакции", `{`, errors.New("секретная причина сбоя хранилища"), 500, "internal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Невалидное тело не должно разбираться до проверки сохранённого ключа.
			h := NewGroupHandler(&mockGroupService{}, groupIdempotencyStub{result: &saved, err: tc.err})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(tc.body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.NewString()))
			rr := httptest.NewRecorder()
			h.CreateGroup(rr, req)
			if tc.err != nil {
				checkErrorResponse(t, rr, tc.status, tc.code, "")
				return
			}
			if rr.Code != tc.status || rr.Header().Get("Content-Type") != "application/json" || rr.Body.String() != string(saved.ResponseBody) {
				t.Fatalf("Изменился сохранённый ответ: %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}
