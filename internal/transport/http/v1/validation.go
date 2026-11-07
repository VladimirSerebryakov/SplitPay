package v1

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"splitpay/internal/domain"

	"github.com/go-playground/validator/v10"
)

func newValidator() *validator.Validate {
	validate := validator.New()
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return validate
}

func validationError(err error) error {
	var validationErrs validator.ValidationErrors

	if errors.As(err, &validationErrs) {
		errs := make(map[string]string)
		for _, f := range validationErrs {
			errs[f.Field()] = getErrorMsg(f)
		}
		return &domain.ValidationError{Fields: errs}
	}
	return err
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
