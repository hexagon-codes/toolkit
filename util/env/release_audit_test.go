package env

import (
	"math"
	"reflect"
	"testing"
)

func TestDefaultsDistinguishMissingFromExplicitEmpty(t *testing.T) {
	const key = "TOOLKIT_ENV_EXPLICIT_EMPTY"
	t.Setenv(key, "")
	if got := GetDefault(key, "fallback"); got != "" {
		t.Fatalf("GetDefault() = %q, want explicit empty value", got)
	}
	if got := GetSliceDefault(key, []string{"fallback"}); len(got) != 0 {
		t.Fatalf("GetSliceDefault() = %v, want explicit empty slice", got)
	}
}

func TestEnvironmentClassificationUsesOnePrecedenceOrder(t *testing.T) {
	t.Setenv("GO_ENV", " production ")
	t.Setenv("ENV", "development")
	t.Setenv("ENVIRONMENT", "testing")

	if !IsProd() {
		t.Fatal("GO_ENV did not take precedence")
	}
	if IsDev() || IsTest() {
		t.Fatalf("conflicting lower-priority variables produced multiple active environments")
	}
}

func TestFloatGettersRejectNonFiniteValues(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			const key = "TOOLKIT_ENV_NONFINITE"
			t.Setenv(key, value)
			if got := GetFloat64(key); got != 0 || math.Signbit(got) {
				t.Fatalf("GetFloat64(%q) = %v, want 0", value, got)
			}
			if got := GetFloat64Default(key, 42); got != 42 {
				t.Fatalf("GetFloat64Default(%q) = %v, want 42", value, got)
			}
		})
	}
}

func TestSliceDefaultDoesNotAliasCallerDefault(t *testing.T) {
	const key = "TOOLKIT_ENV_MISSING_SLICE"
	if err := Unset(key); err != nil {
		t.Fatalf("unset environment variable: %v", err)
	}
	defaultValue := []string{"original"}
	got := GetSliceDefault(key, defaultValue)
	got[0] = "mutated"
	if !reflect.DeepEqual(defaultValue, []string{"original"}) {
		t.Fatalf("returned slice aliases caller default: %v", defaultValue)
	}
}
