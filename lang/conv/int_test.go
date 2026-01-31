package conv

import (
	"testing"
)

func TestInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int32
	}{
		{"nil", nil, 0},
		{"int32", int32(123), 123},
		{"int", 456, 456},
		{"string", "789", 789},
		{"float", 3.14, 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"large int64", int64(2147483647), 2147483647}, // max int32
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int32(tt.input)
			if result != tt.expected {
				t.Errorf("Int32(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUint32(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected uint32
	}{
		{"nil", nil, 0},
		{"uint32", uint32(123), 123},
		{"uint", uint(456), 456},
		{"int", 789, 789},
		{"string", "100", 100},
		{"float", 3.99, 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Uint32(tt.input)
			if result != tt.expected {
				t.Errorf("Uint32(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"nil", nil, 0},
		{"string number", "123", 123},
		{"string invalid", "invalid", 0},
		{"int", 456, 456},
		{"int32", int32(789), 789},
		{"int64", int64(1234), 1234},
		{"float32", float32(3.14), 3},
		{"float64", 3.99, 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"uint", uint(100), 100},
		{"negative", -123, -123},
		{"zero", 0, 0},
		{"[]byte", []byte("456"), 456},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int(tt.input)
			if result != tt.expected {
				t.Errorf("Int(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int64
	}{
		{"nil", nil, 0},
		{"string", "9223372036854775807", 9223372036854775807},
		{"int", int(123), 123},
		{"int8", int8(50), 50},
		{"int16", int16(5000), 5000},
		{"int32", int32(500000), 500000},
		{"int64", int64(123), 123},
		{"uint", uint(456), 456},
		{"uint8", uint8(10), 10},
		{"uint16", uint16(1000), 1000},
		{"uint32", uint32(100000), 100000},
		{"uint64", uint64(6000000), 6000000},
		{"float32", float32(3.14), 3},
		{"float64", 3.14, 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"[]byte", []byte("789"), 789},
		{"negative", int64(-123), -123},
		{"custom type", struct{ v int }{v: 42}, 0}, // default case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int64(tt.input)
			if result != tt.expected {
				t.Errorf("Int64(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUint64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected uint64
	}{
		{"nil", nil, 0},
		{"uint64", uint64(123), 123},
		{"uint", uint(456), 456},
		{"uint8", uint8(10), 10},
		{"uint16", uint16(1000), 1000},
		{"uint32", uint32(100000), 100000},
		{"int", int(789), 789},
		{"int8", int8(50), 50},
		{"int16", int16(5000), 5000},
		{"int32", int32(500000), 500000},
		{"int64", int64(6000000), 6000000},
		{"float32", float32(3.14), 3},
		{"float64", 99.99, 99},
		{"bool true", true, 1},
		{"bool false", false, 0},
		{"string", "789", 789},
		{"[]byte", []byte("456"), 456},
		{"invalid string", "abc", 0},
		{"custom type", struct{ v int }{v: 42}, 0}, // default case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Uint64(tt.input)
			if result != tt.expected {
				t.Errorf("Uint64(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUint(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected uint
	}{
		{"nil", nil, 0},
		{"string", "123", 123},
		{"uint", uint(456), 456},
		{"int", 789, 789},
		{"float64", 3.14, 3},
		{"bool true", true, 1},
		{"bool false", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Uint(tt.input)
			if result != tt.expected {
				t.Errorf("Uint(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		{"nil", nil, false},
		{"bool true", true, true},
		{"bool false", false, false},
		{"int 1", 1, true},
		{"int 0", 0, false},
		{"int negative", -1, true},
		{"int8 1", int8(1), true},
		{"int8 0", int8(0), false},
		{"int16 1", int16(1), true},
		{"int32 0", int32(0), false},
		{"int64 1", int64(1), true},
		{"uint 1", uint(1), true},
		{"uint 0", uint(0), false},
		{"uint8 1", uint8(1), true},
		{"uint16 0", uint16(0), false},
		{"uint32 1", uint32(1), true},
		{"uint64 0", uint64(0), false},
		{"float32 1.0", float32(1.0), true},
		{"float32 0.0", float32(0.0), false},
		{"float64 1.0", 1.0, true},
		{"float64 0.0", 0.0, false},
		{"string true", "true", true},
		{"string false", "false", false},
		{"string 1", "1", true},
		{"string 0", "0", false},
		{"string invalid", "invalid", false},
		{"[]byte true", []byte("true"), true},
		{"[]byte false", []byte("false"), false},
		{"custom type", struct{ v int }{v: 42}, false}, // default case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Bool(tt.input)
			if result != tt.expected {
				t.Errorf("Bool(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func BenchmarkInt(b *testing.B) {
	benchmarks := []struct {
		name  string
		input any
	}{
		{"string", "123"},
		{"int", 456},
		{"float64", 3.14},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = Int(bm.input)
			}
		})
	}
}

func BenchmarkBool(b *testing.B) {
	benchmarks := []struct {
		name  string
		input any
	}{
		{"bool", true},
		{"int", 1},
		{"string", "true"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = Bool(bm.input)
			}
		})
	}
}
