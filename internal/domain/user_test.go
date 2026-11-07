package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestSignUpInputValidate(t *testing.T) {
	for _, tc := range []struct {
		title, name, password, email, field string
	}{
		{"корректные данные", "Иван", "password123", "ivan@example.com", ""},
		{"пароль 7 байт", "Иван", strings.Repeat("a", 7), "ivan@example.com", "password"},
		{"пароль 8 байт", "Иван", strings.Repeat("a", 8), "ivan@example.com", ""},
		{"пароль 72 байта", "Иван", strings.Repeat("a", 72), "ivan@example.com", ""},
		{"пароль 73 байта", "Иван", strings.Repeat("a", 73), "ivan@example.com", "password"},
		{"4 русские буквы — 8 байт", "Иван", strings.Repeat("я", 4), "ivan@example.com", ""},
		{"37 русских букв — 74 байта", "Иван", strings.Repeat("я", 37), "ivan@example.com", "password"},
		{"пустое имя", "", "password123", "ivan@example.com", "name"},
		{"имя 50 символов", strings.Repeat("Я", 50), "password123", "ivan@example.com", ""},
		{"имя 51 символ", strings.Repeat("Я", 51), "password123", "ivan@example.com", "name"},
		{"неверный email", "Иван", "password123", "wrong", "email"},
		{"пустой email", "Иван", "password123", "", "email"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			input := SignUpInput{Name: tc.name, Password: tc.password, Email: tc.email}
			err := input.Validate()
			if tc.field == "" {
				if err != nil {
					t.Fatalf("Ожидался nil, получена ошибка: %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.Is(err, ErrValidationFailed) || !errors.As(err, &validationErr) || validationErr == nil || validationErr.Fields[tc.field] == "" {
				t.Fatalf("Ожидалась ошибка поля %s, получено: %v", tc.field, err)
			}
		})
	}
}
