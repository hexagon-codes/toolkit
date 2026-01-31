package conv

import (
	"encoding/binary"
	"math"
	"testing"
)

// 实现 iFloat32 接口的自定义类型
type customFloat32 float32

func (c customFloat32) Float32() float32 {
	return float32(c)
}

// 实现 iFloat64 接口的自定义类型
type customFloat64 float64

func (c customFloat64) Float64() float64 {
	return float64(c)
}

func TestFloat32(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float32
	}{
		{"nil", nil, 0},
		{"float32", float32(3.14), 3.14},
		{"float64", 3.14, 3.14},
		{"string", "3.14159", 3.14159},
		{"int", 42, 42.0},
		{"zero", 0, 0.0},
		{"negative", -3.14, -3.14},
		{"custom type", customFloat32(2.5), 2.5}, // 测试 iFloat32 接口
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float32(tt.input)
			// 浮点数比较使用小误差
			if math.Abs(float64(result-tt.expected)) > 0.0001 {
				t.Errorf("Float32(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFloat32_Binary(t *testing.T) {
	// 测试二进制解码
	var f float32 = 3.14
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(f))

	result := Float32(b)
	if math.Abs(float64(result-f)) > 0.0001 {
		t.Errorf("Float32(binary) = %v, want %v", result, f)
	}

	// 测试不足4字节
	shortBytes := []byte{1, 2}
	result = Float32(shortBytes)
	if result != 0 {
		t.Errorf("Float32(short bytes) = %v, want 0", result)
	}
}

func TestFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
	}{
		{"nil", nil, 0},
		{"float64", 3.14159, 3.14159},
		{"float32", float32(3.14), 3.140000104904175}, // float32 精度损失
		{"string", "3.14159265359", 3.14159265359},
		{"int", 42, 42.0},
		{"zero", 0, 0.0},
		{"negative", -3.14159, -3.14159},
		{"scientific", "1.23e-4", 0.000123},
		{"custom type", customFloat64(9.87), 9.87}, // 测试 iFloat64 接口
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float64(tt.input)
			if math.Abs(result-tt.expected) > 0.0000001 {
				t.Errorf("Float64(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFloat64_Binary(t *testing.T) {
	// 测试二进制解码
	var f float64 = 3.141592653589793
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(f))

	result := Float64(b)
	if math.Abs(result-f) > 0.0000001 {
		t.Errorf("Float64(binary) = %v, want %v", result, f)
	}

	// 测试不足8字节
	shortBytes := []byte{1, 2, 3}
	result = Float64(shortBytes)
	if result != 0 {
		t.Errorf("Float64(short bytes) = %v, want 0", result)
	}
}

func BenchmarkFloat32(b *testing.B) {
	benchmarks := []struct {
		name  string
		input any
	}{
		{"string", "3.14"},
		{"float32", float32(3.14)},
		{"float64", 3.14},
		{"int", 42},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = Float32(bm.input)
			}
		})
	}
}

func BenchmarkFloat64(b *testing.B) {
	benchmarks := []struct {
		name  string
		input any
	}{
		{"string", "3.14159"},
		{"float64", 3.14159},
		{"int", 42},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = Float64(bm.input)
			}
		})
	}
}
