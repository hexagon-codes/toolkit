package errorx

import (
	"errors"
	"strings"
	"testing"
)

func TestTry(t *testing.T) {
	// Normal execution
	err := Try(func() {
		// no panic
	})
	if err != nil {
		t.Error("Try should return nil for normal execution")
	}

	// Panic with error
	err = Try(func() {
		panic(errors.New("test error"))
	})
	if err == nil || err.Error() != "test error" {
		t.Error("Try should catch panic error")
	}

	// Panic with non-error
	err = Try(func() {
		panic("string panic")
	})
	if err == nil || !strings.Contains(err.Error(), "string panic") {
		t.Error("Try should catch non-error panic")
	}
}

func TestTryWithValue(t *testing.T) {
	// Normal execution
	val, err := TryWithValue(func() int {
		return 42
	})
	if err != nil || val != 42 {
		t.Error("TryWithValue should return value for normal execution")
	}

	// Panic
	val, err = TryWithValue(func() int {
		panic("test")
	})
	if err == nil {
		t.Error("TryWithValue should catch panic")
	}
}

func TestTryWithError(t *testing.T) {
	// Success
	val, err := TryWithError(func() (int, error) {
		return 42, nil
	})
	if err != nil || val != 42 {
		t.Error("TryWithError should return value")
	}

	// Error return
	val, err = TryWithError(func() (int, error) {
		return 0, errors.New("error")
	})
	if err == nil || err.Error() != "error" {
		t.Error("TryWithError should return error")
	}

	// Panic
	_, err = TryWithError(func() (int, error) {
		panic("panic")
	})
	if err == nil {
		t.Error("TryWithError should catch panic")
	}
}

func TestMust(t *testing.T) {
	// Success
	val := Must(42, nil)
	if val != 42 {
		t.Error("Must should return value")
	}

	// Panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic on error")
		}
	}()
	Must(0, errors.New("error"))
}

func TestMustOK(t *testing.T) {
	// Success
	val := MustOK(42, true)
	if val != 42 {
		t.Error("MustOK should return value")
	}

	// Panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustOK should panic when not ok")
		}
	}()
	MustOK(0, false)
}

func TestMust0(t *testing.T) {
	// Success
	Must0(nil) // Should not panic

	// Panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must0 should panic on error")
		}
	}()
	Must0(errors.New("error"))
}

func TestMust2(t *testing.T) {
	v1, v2 := Must2(1, "a", nil)
	if v1 != 1 || v2 != "a" {
		t.Error("Must2 should return values")
	}
}

func TestMust3(t *testing.T) {
	v1, v2, v3 := Must3(1, "a", true, nil)
	if v1 != 1 || v2 != "a" || v3 != true {
		t.Error("Must3 should return values")
	}
}

func TestWrap(t *testing.T) {
	// Wrap error
	err := errors.New("original")
	wrapped := Wrap(err, "context")

	if !strings.Contains(wrapped.Error(), "context") {
		t.Error("Wrap should add context")
	}

	if !errors.Is(wrapped, err) {
		t.Error("Wrapped error should be unwrappable")
	}

	// Wrap nil
	if Wrap(nil, "context") != nil {
		t.Error("Wrap nil should return nil")
	}
}

func TestWrapf(t *testing.T) {
	err := errors.New("original")
	wrapped := Wrapf(err, "context %d", 42)

	if !strings.Contains(wrapped.Error(), "context 42") {
		t.Error("Wrapf should add formatted context")
	}

	// Wrap nil
	if Wrapf(nil, "context %d", 42) != nil {
		t.Error("Wrapf nil should return nil")
	}
}

func TestUnwrap(t *testing.T) {
	inner := errors.New("inner")
	outer := Wrap(inner, "outer")

	unwrapped := Unwrap(outer)
	if unwrapped != inner {
		t.Error("Unwrap should return inner error")
	}
}

func TestIs(t *testing.T) {
	target := errors.New("target")
	wrapped := Wrap(target, "context")

	if !Is(wrapped, target) {
		t.Error("Is should find target error")
	}
}

type testCustomError struct {
	code int
}

func (e *testCustomError) Error() string {
	return "custom error"
}

func TestAs(t *testing.T) {
	ce := &testCustomError{code: 42}
	wrapped := Wrap(ce, "context")

	found, ok := As[*testCustomError](wrapped)
	if !ok || found.code != 42 {
		t.Error("As should find and convert error")
	}
}

func TestNew(t *testing.T) {
	err := New("test")
	if err.Error() != "test" {
		t.Error("New should create error")
	}
}

func TestNewf(t *testing.T) {
	err := Newf("test %d", 42)
	if err.Error() != "test 42" {
		t.Error("Newf should create formatted error")
	}
}

func TestJoin(t *testing.T) {
	e1 := errors.New("error1")
	e2 := errors.New("error2")

	joined := Join(e1, e2)
	if !errors.Is(joined, e1) || !errors.Is(joined, e2) {
		t.Error("Join should combine errors")
	}
}

func TestIgnore(t *testing.T) {
	val := Ignore(42, errors.New("ignored"))
	if val != 42 {
		t.Error("Ignore should return value")
	}
}

func TestCoalesce(t *testing.T) {
	e1 := errors.New("first")
	e2 := errors.New("second")

	if Coalesce(nil, nil) != nil {
		t.Error("Coalesce should return nil for all nil")
	}

	if Coalesce(nil, e1, e2) != e1 {
		t.Error("Coalesce should return first non-nil error")
	}
}

func TestWithStack(t *testing.T) {
	err := WithStack(errors.New("test"))

	se, ok := err.(*StackError)
	if !ok {
		t.Fatal("WithStack should return StackError")
	}

	if se.Error() != "test" {
		t.Error("StackError should preserve message")
	}

	stack := se.Stack()
	if !strings.Contains(stack, "TestWithStack") {
		t.Error("Stack should contain caller")
	}

	// Test nil
	if WithStack(nil) != nil {
		t.Error("WithStack nil should return nil")
	}
}

func TestStackTrace(t *testing.T) {
	err := WithStack(errors.New("test"))
	stack := StackTrace(err)

	if !strings.Contains(stack, "TestStackTrace") {
		t.Error("StackTrace should return stack")
	}

	// Non-stack error
	if StackTrace(errors.New("plain")) != "" {
		t.Error("StackTrace should return empty for plain error")
	}
}

func TestSafe(t *testing.T) {
	// Normal
	err := Safe(func() error {
		return nil
	})
	if err != nil {
		t.Error("Safe should return nil")
	}

	// Error
	err = Safe(func() error {
		return errors.New("error")
	})
	if err == nil {
		t.Error("Safe should return error")
	}

	// Panic
	err = Safe(func() error {
		panic("panic")
	})
	if err == nil {
		t.Error("Safe should catch panic")
	}
}

func TestSafeGo(t *testing.T) {
	done := make(chan error, 1)

	SafeGo(func() {
		panic("test panic")
	}, func(err error) {
		done <- err
	})

	err := <-done
	if err == nil || !strings.Contains(err.Error(), "test panic") {
		t.Error("SafeGo should catch panic and call handler")
	}
}

func TestResult(t *testing.T) {
	// Ok result
	ok := Ok(42)
	if !ok.IsOk() || ok.IsErr() {
		t.Error("Ok should be ok")
	}
	if ok.Value() != 42 {
		t.Error("Value should return 42")
	}

	// Err result
	errResult := Err[int](errors.New("error"))
	if errResult.IsOk() || !errResult.IsErr() {
		t.Error("Err should be err")
	}

	// FromError
	r := FromError(42, nil)
	if !r.IsOk() {
		t.Error("FromError with nil should be ok")
	}

	// Unwrap
	val, err := ok.Unwrap()
	if val != 42 || err != nil {
		t.Error("Unwrap failed")
	}

	// UnwrapOr
	if errResult.UnwrapOr(99) != 99 {
		t.Error("UnwrapOr should return default on error")
	}

	// UnwrapOrElse
	if errResult.UnwrapOrElse(func(e error) int { return 88 }) != 88 {
		t.Error("UnwrapOrElse should call function on error")
	}

	// Must on ok
	if ok.Must() != 42 {
		t.Error("Must should return value on ok")
	}
}

func TestResultMustPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic on error result")
		}
	}()

	errResult := Err[int](errors.New("error"))
	errResult.Must()
}

func TestMap(t *testing.T) {
	ok := Ok(42)
	mapped := Map(ok, func(v int) string {
		return "value"
	})
	if !mapped.IsOk() || mapped.Value() != "value" {
		t.Error("Map should transform value")
	}

	errResult := Err[int](errors.New("error"))
	mapped2 := Map(errResult, func(v int) string {
		return "value"
	})
	if !mapped2.IsErr() {
		t.Error("Map should preserve error")
	}
}

func TestFlatMap(t *testing.T) {
	ok := Ok(42)
	flatMapped := FlatMap(ok, func(v int) Result[string] {
		return Ok("value")
	})
	if !flatMapped.IsOk() || flatMapped.Value() != "value" {
		t.Error("FlatMap should transform value")
	}

	flatMappedErr := FlatMap(ok, func(v int) Result[string] {
		return Err[string](errors.New("error"))
	})
	if !flatMappedErr.IsErr() {
		t.Error("FlatMap should return error result")
	}
}
