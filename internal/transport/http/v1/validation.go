package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationErrorResponse struct {
	Errors map[string]string `json:"errors"`
}

func HandleValidationErrors(w http.ResponseWriter, err error) {
	var validationErrs validator.ValidationErrors

	if errors.As(err, &validationErrs) {
		errs := make(map[string]string)
		for _, f := range validationErrs {
			field := strings.ToLower(f.Field())
			errs[field] = getErrorMsg(f)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ValidationErrorResponse{Errors: errs})
		return
	}
	http.Error(w, "invalid request body", http.StatusBadRequest)
}

func getErrorMsg(f validator.FieldError) string {
	switch f.Tag() {
	case "required":
		return "Это поле обязательно для заполнения"
	case "email":
		return "Неверный формат email адреса"
	case "min":
		return fmt.Sprintf("Минимальная длина %s символов", f.Param())
	case "max":
		return fmt.Sprintf("Максимальная длина %s символов", f.Param())
	default:
		return "Неверное значение"
	}
}
