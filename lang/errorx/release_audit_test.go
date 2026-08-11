package errorx

import (
	"errors"
	"testing"
)

type auditNilError struct{}

func (*auditNilError) Error() string { return "typed nil" }

func TestRecoveredErrorAlwaysRetainsStackAndCause(t *testing.T) {
	sentinel := errors.New("panic cause")
	err := Try(func() { panic(sentinel) })
	var panicErr *PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("Try panic error type = %T, want *PanicError", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Try panic error = %v, want it to unwrap %v", err, sentinel)
	}
	if panicErr.Stack() == "" {
		t.Fatal("PanicError did not retain a recovery stack")
	}
}

func TestErrorHelpersNormalizeTypedNil(t *testing.T) {
	var typedNil *auditNilError
	if err := Wrap(typedNil, "context"); err != nil {
		t.Fatalf("Wrap(typed nil) = %v, want nil", err)
	}
	coded := ErrInternal("failed").WithCause(typedNil)
	if coded.Unwrap() != nil {
		t.Fatalf("WithCause(typed nil) retained %T, want nil", coded.Unwrap())
	}
}
