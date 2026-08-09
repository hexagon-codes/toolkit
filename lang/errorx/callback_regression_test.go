package errorx

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type nilCloseProbe struct{}

func (*nilCloseProbe) Close() error {
	return nil
}

func TestNilCallbacksAndCloserDoNotPanic(t *testing.T) {
	tests := []struct {
		name string
		run  func()
	}{
		{
			name: "Walk callback",
			run:  func() { Walk(errors.New("root"), nil) },
		},
		{
			name: "RecoverWithHandler callback",
			run: func() {
				defer RecoverWithHandler(nil)
				panic("boom")
			},
		},
		{
			name: "IgnoreClose nil interface",
			run:  func() { IgnoreClose(nil) },
		},
		{
			name: "IgnoreClose typed nil",
			run:  func() { IgnoreClose((*nilCloseProbe)(nil)) },
		},
		{
			name: "Map callback",
			run: func() {
				Map(Ok(1), (func(int) string)(nil))
			},
		},
		{
			name: "FlatMap callback",
			run: func() {
				FlatMap(Ok(1), (func(int) Result[string])(nil))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Try(tt.run); err != nil {
				t.Fatalf("nil callback or closer panicked: %v", err)
			}
		})
	}
}

func TestSafeGoReportsNilOperationExplicitly(t *testing.T) {
	panicResult := SafeGo(nil)

	select {
	case err := <-panicResult:
		if err == nil || !strings.Contains(err.Error(), "operation must not be nil") {
			t.Fatalf("SafeGo nil operation error = %v, want explicit validation error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SafeGo did not report the nil operation")
	}
}

func TestUnwrapOrElseRejectsNilCallbackWithoutLosingOriginalError(t *testing.T) {
	original := errors.New("failed")
	value, err := Err[int](original).UnwrapOrElse(nil)
	if value != 0 {
		t.Fatalf("UnwrapOrElse(nil) value = %d, want zero", value)
	}
	if !errors.Is(err, ErrNilCallback) || !errors.Is(err, original) {
		t.Fatalf("UnwrapOrElse(nil) error = %v, want callback and original errors", err)
	}
}
