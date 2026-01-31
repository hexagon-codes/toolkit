package env

import (
	"os"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	os.Setenv("TEST_VAR", "value")
	defer os.Unsetenv("TEST_VAR")

	if Get("TEST_VAR") != "value" {
		t.Error("Get should return value")
	}

	if Get("NON_EXISTENT") != "" {
		t.Error("Get should return empty for non-existent")
	}
}

func TestGetDefault(t *testing.T) {
	os.Setenv("TEST_VAR", "value")
	defer os.Unsetenv("TEST_VAR")

	if GetDefault("TEST_VAR", "default") != "value" {
		t.Error("GetDefault should return value when set")
	}

	if GetDefault("NON_EXISTENT", "default") != "default" {
		t.Error("GetDefault should return default when not set")
	}
}

func TestLookup(t *testing.T) {
	os.Setenv("TEST_VAR", "value")
	defer os.Unsetenv("TEST_VAR")

	val, ok := Lookup("TEST_VAR")
	if !ok || val != "value" {
		t.Error("Lookup should find existing var")
	}

	_, ok = Lookup("NON_EXISTENT")
	if ok {
		t.Error("Lookup should return false for non-existent")
	}
}

func TestMustGet(t *testing.T) {
	os.Setenv("TEST_VAR", "value")
	defer os.Unsetenv("TEST_VAR")

	val := MustGet("TEST_VAR")
	if val != "value" {
		t.Error("MustGet should return value")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet should panic for non-existent")
		}
	}()
	MustGet("NON_EXISTENT")
}

func TestGetInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	if GetInt("TEST_INT") != 42 {
		t.Error("GetInt should return int value")
	}

	if GetInt("NON_EXISTENT") != 0 {
		t.Error("GetInt should return 0 for non-existent")
	}

	os.Setenv("TEST_INVALID", "not_a_number")
	defer os.Unsetenv("TEST_INVALID")

	if GetInt("TEST_INVALID") != 0 {
		t.Error("GetInt should return 0 for invalid")
	}
}

func TestGetIntDefault(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	if GetIntDefault("TEST_INT", 99) != 42 {
		t.Error("GetIntDefault should return value when set")
	}

	if GetIntDefault("NON_EXISTENT", 99) != 99 {
		t.Error("GetIntDefault should return default when not set")
	}

	os.Setenv("TEST_INVALID", "not_a_number")
	defer os.Unsetenv("TEST_INVALID")

	if GetIntDefault("TEST_INVALID", 99) != 99 {
		t.Error("GetIntDefault should return default for invalid")
	}
}

func TestGetInt64(t *testing.T) {
	os.Setenv("TEST_INT64", "9223372036854775807")
	defer os.Unsetenv("TEST_INT64")

	if GetInt64("TEST_INT64") != 9223372036854775807 {
		t.Error("GetInt64 should return int64 value")
	}

	if GetInt64("NON_EXISTENT") != 0 {
		t.Error("GetInt64 should return 0 for non-existent")
	}
}

func TestGetInt64Default(t *testing.T) {
	if GetInt64Default("NON_EXISTENT", 123) != 123 {
		t.Error("GetInt64Default should return default")
	}

	os.Setenv("TEST_INVALID", "not_a_number")
	defer os.Unsetenv("TEST_INVALID")

	if GetInt64Default("TEST_INVALID", 123) != 123 {
		t.Error("GetInt64Default should return default for invalid")
	}
}

func TestGetFloat64(t *testing.T) {
	os.Setenv("TEST_FLOAT", "3.14")
	defer os.Unsetenv("TEST_FLOAT")

	if GetFloat64("TEST_FLOAT") != 3.14 {
		t.Error("GetFloat64 should return float value")
	}

	if GetFloat64("NON_EXISTENT") != 0 {
		t.Error("GetFloat64 should return 0 for non-existent")
	}
}

func TestGetFloat64Default(t *testing.T) {
	if GetFloat64Default("NON_EXISTENT", 1.23) != 1.23 {
		t.Error("GetFloat64Default should return default")
	}

	os.Setenv("TEST_INVALID", "not_a_number")
	defer os.Unsetenv("TEST_INVALID")

	if GetFloat64Default("TEST_INVALID", 1.23) != 1.23 {
		t.Error("GetFloat64Default should return default for invalid")
	}
}

func TestGetBool(t *testing.T) {
	trueValues := []string{"true", "TRUE", "1", "yes", "YES", "on", "ON"}
	falseValues := []string{"false", "FALSE", "0", "no", "NO", "off", "OFF", "invalid"}

	for _, v := range trueValues {
		os.Setenv("TEST_BOOL", v)
		if !GetBool("TEST_BOOL") {
			t.Errorf("GetBool should return true for %s", v)
		}
	}

	for _, v := range falseValues {
		os.Setenv("TEST_BOOL", v)
		if GetBool("TEST_BOOL") {
			t.Errorf("GetBool should return false for %s", v)
		}
	}

	os.Unsetenv("TEST_BOOL")
}

func TestGetBoolDefault(t *testing.T) {
	// Not set
	if GetBoolDefault("NON_EXISTENT", true) != true {
		t.Error("GetBoolDefault should return default when not set")
	}

	// Set to false
	os.Setenv("TEST_BOOL", "false")
	defer os.Unsetenv("TEST_BOOL")

	if GetBoolDefault("TEST_BOOL", true) != false {
		t.Error("GetBoolDefault should return parsed value")
	}
}

func TestGetDuration(t *testing.T) {
	os.Setenv("TEST_DURATION", "5s")
	defer os.Unsetenv("TEST_DURATION")

	if GetDuration("TEST_DURATION") != 5*time.Second {
		t.Error("GetDuration should return duration")
	}

	if GetDuration("NON_EXISTENT") != 0 {
		t.Error("GetDuration should return 0 for non-existent")
	}
}

func TestGetDurationDefault(t *testing.T) {
	if GetDurationDefault("NON_EXISTENT", time.Minute) != time.Minute {
		t.Error("GetDurationDefault should return default")
	}

	os.Setenv("TEST_INVALID", "not_a_duration")
	defer os.Unsetenv("TEST_INVALID")

	if GetDurationDefault("TEST_INVALID", time.Minute) != time.Minute {
		t.Error("GetDurationDefault should return default for invalid")
	}
}

func TestGetSlice(t *testing.T) {
	os.Setenv("TEST_SLICE", "a, b, c")
	defer os.Unsetenv("TEST_SLICE")

	slice := GetSlice("TEST_SLICE")
	if len(slice) != 3 || slice[0] != "a" || slice[1] != "b" || slice[2] != "c" {
		t.Error("GetSlice should parse comma-separated values")
	}

	if GetSlice("NON_EXISTENT") != nil {
		t.Error("GetSlice should return nil for non-existent")
	}

	// Empty parts should be filtered
	os.Setenv("TEST_SLICE", "a, , b")
	slice = GetSlice("TEST_SLICE")
	if len(slice) != 2 {
		t.Error("GetSlice should filter empty parts")
	}
}

func TestGetSliceDefault(t *testing.T) {
	defaultVal := []string{"default"}

	if len(GetSliceDefault("NON_EXISTENT", defaultVal)) != 1 {
		t.Error("GetSliceDefault should return default")
	}

	os.Setenv("TEST_SLICE", "a, b")
	defer os.Unsetenv("TEST_SLICE")

	if len(GetSliceDefault("TEST_SLICE", defaultVal)) != 2 {
		t.Error("GetSliceDefault should return parsed value")
	}

	// Empty value
	os.Setenv("TEST_EMPTY", ",  ,")
	defer os.Unsetenv("TEST_EMPTY")

	if len(GetSliceDefault("TEST_EMPTY", defaultVal)) != 1 {
		t.Error("GetSliceDefault should return default for empty")
	}
}

func TestSet(t *testing.T) {
	err := Set("TEST_SET", "value")
	if err != nil {
		t.Error("Set should not error")
	}

	if os.Getenv("TEST_SET") != "value" {
		t.Error("Set should set env var")
	}

	os.Unsetenv("TEST_SET")
}

func TestUnset(t *testing.T) {
	os.Setenv("TEST_UNSET", "value")

	err := Unset("TEST_UNSET")
	if err != nil {
		t.Error("Unset should not error")
	}

	if os.Getenv("TEST_UNSET") != "" {
		t.Error("Unset should remove env var")
	}
}

func TestExists(t *testing.T) {
	os.Setenv("TEST_EXISTS", "value")
	defer os.Unsetenv("TEST_EXISTS")

	if !Exists("TEST_EXISTS") {
		t.Error("Exists should return true")
	}

	if Exists("NON_EXISTENT") {
		t.Error("Exists should return false for non-existent")
	}
}

func TestIsProd(t *testing.T) {
	// Clean up
	defer func() {
		os.Unsetenv("GO_ENV")
		os.Unsetenv("ENV")
		os.Unsetenv("ENVIRONMENT")
	}()

	// Not set
	if IsProd() {
		t.Error("IsProd should return false when not set")
	}

	// Test various prod values
	prodValues := []string{"prod", "production", "PROD", "PRODUCTION"}
	envVars := []string{"GO_ENV", "ENV", "ENVIRONMENT"}

	for _, envVar := range envVars {
		for _, val := range prodValues {
			os.Setenv(envVar, val)
			if !IsProd() {
				t.Errorf("IsProd should return true for %s=%s", envVar, val)
			}
			os.Unsetenv(envVar)
		}
	}
}

func TestIsDev(t *testing.T) {
	defer func() {
		os.Unsetenv("GO_ENV")
	}()

	os.Setenv("GO_ENV", "development")
	if !IsDev() {
		t.Error("IsDev should return true for development")
	}

	os.Setenv("GO_ENV", "dev")
	if !IsDev() {
		t.Error("IsDev should return true for dev")
	}

	os.Setenv("GO_ENV", "local")
	if !IsDev() {
		t.Error("IsDev should return true for local")
	}

	os.Unsetenv("GO_ENV")
	if IsDev() {
		t.Error("IsDev should return false when not set")
	}
}

func TestIsTest(t *testing.T) {
	defer func() {
		os.Unsetenv("GO_ENV")
	}()

	os.Setenv("GO_ENV", "test")
	if !IsTest() {
		t.Error("IsTest should return true for test")
	}

	os.Setenv("GO_ENV", "testing")
	if !IsTest() {
		t.Error("IsTest should return true for testing")
	}

	os.Setenv("GO_ENV", "staging")
	if !IsTest() {
		t.Error("IsTest should return true for staging")
	}

	os.Unsetenv("GO_ENV")
	if IsTest() {
		t.Error("IsTest should return false when not set")
	}
}
