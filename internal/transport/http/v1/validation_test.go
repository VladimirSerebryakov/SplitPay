package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"splitpay/internal/transport/http/response"
)

func TestValidationErrorConversion(t *testing.T) {
	validate := newValidator()
	err := validate.Struct(struct {
		UserID string `json:"user_id,omitempty" validate:"required"`
	}{})
	rr := httptest.NewRecorder()
	response.WriteError(rr, httptest.NewRequest(http.MethodPost, "/", nil), validationError(err))
	checkErrorResponse(t, rr, 400, "validation_failed", "user_id")
	err = validate.Struct(nil)
	if validationError(err) != err {
		t.Fatal("Внутренняя ошибка validator потеряна")
	}
	rr = httptest.NewRecorder()
	response.WriteError(rr, httptest.NewRequest(http.MethodPost, "/", nil), validationError(err))
	checkErrorResponse(t, rr, 500, "internal", "")
}
