package validator

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestOneOfAcceptsEveryDeclaredValue(t *testing.T) {
	v := NewValidator()
	for _, value := range []string{"admin", "user", "guest"} {
		t.Run(value, func(t *testing.T) {
			if err := v.Var(value, "oneof=admin,user,guest"); err != nil {
				t.Fatalf("declared value %q was rejected: %v", value, err)
			}
		})
	}
}

func TestRequiredRejectsTypedNilValues(t *testing.T) {
	v := NewValidator()
	var channel chan int
	var function func()
	for name, value := range map[string]any{"channel": channel, "function": function} {
		t.Run(name, func(t *testing.T) {
			if err := v.Var(value, "required"); err == nil {
				t.Fatal("required accepted a typed nil value")
			}
		})
	}
}

func TestNilAndPanickingRulesReturnValidationErrors(t *testing.T) {
	v := NewValidator()
	v.RegisterRule("nil-rule", nil)
	v.RegisterRule("panic-rule", func(any, string) bool {
		panic("sensitive panic payload")
	})

	for _, rule := range []string{"nil-rule", "panic-rule"} {
		t.Run(rule, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("validation panicked: %v", recovered)
				}
			}()
			err := v.Var("value", rule)
			if err == nil {
				t.Fatal("invalid rule returned nil")
			}
			if strings.Contains(err.Error(), "sensitive panic payload") {
				t.Fatalf("validation error leaked panic payload: %v", err)
			}
		})
	}
}

func TestValidationErrorsDoNotRetainRejectedValue(t *testing.T) {
	const secret = "top-secret-validator-value"
	err := NewValidator().Var(secret, "len=1")
	if err == nil {
		t.Fatal("invalid value unexpectedly passed validation")
	}
	if dump := fmt.Sprintf("%#v", err); strings.Contains(dump, secret) {
		t.Fatalf("validation error retained rejected value: %s", dump)
	}
}

func TestUnknownRuleFailsClosed(t *testing.T) {
	err := NewValidator().Var("value", "requried")
	if err == nil {
		t.Fatal("unknown validation rule was silently ignored")
	}
	if !errors.Is(err, ErrUnknownRule) {
		t.Fatalf("unknown rule error = %v, want ErrUnknownRule", err)
	}
}

func TestUnknownRuleFailsClosedForEmptyValue(t *testing.T) {
	err := NewValidator().Var("", "requried")
	if !errors.Is(err, ErrUnknownRule) {
		t.Fatalf("empty value unknown-rule error = %v, want ErrUnknownRule", err)
	}
}

func TestEmailRejectsDisplayNameSyntax(t *testing.T) {
	if Email("Alice <alice@example.com>") {
		t.Fatal("display-name mailbox was accepted as a bare email address")
	}
}

func TestIDCardRejectsImpossibleCalendarDate(t *testing.T) {
	prefix := "11010119990231001"
	weights := [...]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checkCodes := "10X98765432"
	sum := 0
	for index := range prefix {
		sum += int(prefix[index]-'0') * weights[index]
	}
	id := prefix + string(checkCodes[sum%11])
	if IDCard(id) {
		t.Fatalf("identity number with February 31 was accepted: %s", id)
	}
}

func TestPasswordCountsUnicodeCodePoints(t *testing.T) {
	if Password("你Aa123") {
		t.Fatal("six-code-point password passed the eight-character minimum")
	}
}

func TestLengthBoundsRejectInvalidRanges(t *testing.T) {
	if MinLength("", -1) {
		t.Fatal("negative minimum length was accepted")
	}
	if LengthBetween("abc", 4, 2) {
		t.Fatal("reversed length range was accepted")
	}
}

func TestValidatorRegistrationAndValidationAreConcurrentSafe(t *testing.T) {
	v := NewValidator()
	const workers = 8
	const iterations = 500
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				v.RegisterRule("dynamic", func(any, string) bool { return true })
				v.RegisterMessage("dynamic", "%s is invalid")
				v.SetTagName("validate")
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				_ = v.Var("value", "dynamic")
			}
		}()
	}
	close(start)
	wg.Wait()
}
