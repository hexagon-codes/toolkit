package conv

import (
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"[]byte", []byte("world"), "world"},
		{"int", 123, "123"},
		{"int8", int8(127), "127"},
		{"int16", int16(32767), "32767"},
		{"int32", int32(2147483647), "2147483647"},
		{"int64", int64(9223372036854775807), "9223372036854775807"},
		{"uint", uint(123), "123"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float32", float32(3.14), "3.14"},
		{"float64", 3.14159, "3.14159"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"negative int", -123, "-123"},
		{"zero", 0, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String(tt.input)
			if result != tt.expected {
				t.Errorf("String(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// 自定义类型测试
type customString struct {
	value string
}

func (c customString) String() string {
	return c.value
}

func TestString_CustomType(t *testing.T) {
	custom := customString{value: "custom"}
	result := String(custom)
	if result != "custom" {
		t.Errorf("String(customString) = %v, want %v", result, "custom")
	}
}

func BenchmarkString(b *testing.B) {
	benchmarks := []struct {
		name  string
		input any
	}{
		{"int", 123},
		{"float64", 3.14159},
		{"string", "hello"},
		{"bool", true},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = String(bm.input)
			}
		})
	}
}
