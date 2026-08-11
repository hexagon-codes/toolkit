package validator

import "testing"

func FuzzValidatorBuiltinsNeverPanic(f *testing.F) {
	f.Add("test@example.com", "required,email")
	f.Add("guest", "oneof=admin,user,guest")
	f.Add("", "required")
	f.Fuzz(func(t *testing.T, value, tag string) {
		validator := NewValidator()
		_ = validator.Var(value, tag)
		_ = Email(value)
		_ = URL(value)
		_ = IDCard(value)
		_ = Password(value)
	})
}
