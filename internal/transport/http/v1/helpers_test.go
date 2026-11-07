package v1

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// checkErrorResponse проверяет общий формат ошибки в тестах HTTP-обработчиков.
func checkErrorResponse(t *testing.T, rr *httptest.ResponseRecorder, status int, code, field string) {
	t.Helper()
	var result struct {
		Error struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rr.Code != status || rr.Header().Get("Content-Type") != "application/json" || result.Error.Code != code || result.Error.Message == "" {
		t.Fatalf("Неверный ответ: %d %s", rr.Code, rr.Body.String())
	}
	if field != "" && result.Error.Details[field] == "" {
		t.Fatalf("Нет ошибки поля %s: %s", field, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "секретная причина") {
		t.Fatal("Раскрыта внутренняя причина")
	}
}
