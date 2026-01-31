package mathx

import (
	"math"
	"testing"
)

// TestMin 测试 Min 函数
func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		values   []int
		expected int
	}{
		{"单个值", []int{5}, 5},
		{"多个值", []int{3, 1, 4, 1, 5}, 1},
		{"负数", []int{-3, -1, -4}, -4},
		{"混合", []int{-1, 0, 1}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Min(tt.values...)
			if result != tt.expected {
				t.Errorf("Min(%v) = %d, want %d",
					tt.values, result, tt.expected)
			}
		})
	}

	// 测试空参数
	t.Run("空参数", func(t *testing.T) {
		result := Min[int]()
		if result != 0 {
			t.Errorf("Min() = %d, want 0", result)
		}
	})
}

// TestMinFloat 测试 Min 函数（浮点数）
func TestMinFloat(t *testing.T) {
	result := Min(3.14, 2.71, 1.41)
	if result != 1.41 {
		t.Errorf("Min(3.14, 2.71, 1.41) = %f, want 1.41", result)
	}
}

// TestMinString 测试 Min 函数（字符串）
func TestMinString(t *testing.T) {
	result := Min("c", "a", "b")
	if result != "a" {
		t.Errorf("Min(c, a, b) = %s, want a", result)
	}
}

// TestMax 测试 Max 函数
func TestMax(t *testing.T) {
	tests := []struct {
		name     string
		values   []int
		expected int
	}{
		{"单个值", []int{5}, 5},
		{"多个值", []int{3, 1, 4, 1, 5}, 5},
		{"负数", []int{-3, -1, -4}, -1},
		{"混合", []int{-1, 0, 1}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Max(tt.values...)
			if result != tt.expected {
				t.Errorf("Max(%v) = %d, want %d",
					tt.values, result, tt.expected)
			}
		})
	}

	// 测试空参数
	t.Run("空参数", func(t *testing.T) {
		result := Max[int]()
		if result != 0 {
			t.Errorf("Max() = %d, want 0", result)
		}
	})
}

// TestMinMax 测试 MinMax 函数
func TestMinMax(t *testing.T) {
	t.Run("正常情况", func(t *testing.T) {
		min, max := MinMax(3, 1, 4, 1, 5)
		if min != 1 || max != 5 {
			t.Errorf("MinMax(3, 1, 4, 1, 5) = (%d, %d), want (1, 5)", min, max)
		}
	})

	t.Run("单个值", func(t *testing.T) {
		min, max := MinMax(5)
		if min != 5 || max != 5 {
			t.Errorf("MinMax(5) = (%d, %d), want (5, 5)", min, max)
		}
	})

	t.Run("空参数", func(t *testing.T) {
		min, max := MinMax[int]()
		if min != 0 || max != 0 {
			t.Errorf("MinMax() = (%d, %d), want (0, 0)", min, max)
		}
	})

	t.Run("负数", func(t *testing.T) {
		min, max := MinMax(-5, -1, -10)
		if min != -10 || max != -1 {
			t.Errorf("MinMax(-5, -1, -10) = (%d, %d), want (-10, -1)", min, max)
		}
	})
}

// TestClamp 测试 Clamp 函数
func TestClamp(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		min      int
		max      int
		expected int
	}{
		{"在范围内", 5, 0, 10, 5},
		{"超过上限", 15, 0, 10, 10},
		{"低于下限", -5, 0, 10, 0},
		{"等于下限", 0, 0, 10, 0},
		{"等于上限", 10, 0, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Clamp(tt.value, tt.min, tt.max)
			if result != tt.expected {
				t.Errorf("Clamp(%d, %d, %d) = %d, want %d",
					tt.value, tt.min, tt.max, result, tt.expected)
			}
		})
	}
}

// TestAbs 测试 Abs 函数
func TestAbs(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		expected int
	}{
		{"正数", 5, 5},
		{"负数", -5, 5},
		{"零", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Abs(tt.value)
			if result != tt.expected {
				t.Errorf("Abs(%d) = %d, want %d",
					tt.value, result, tt.expected)
			}
		})
	}
}

// TestAbsFloat 测试 Abs 函数（浮点数）
func TestAbsFloat(t *testing.T) {
	result := Abs(-3.14)
	if result != 3.14 {
		t.Errorf("Abs(-3.14) = %f, want 3.14", result)
	}
}

// TestAbsDiff 测试 AbsDiff 函数
func TestAbsDiff(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"a > b", 5, 3, 2},
		{"a < b", 3, 5, 2},
		{"a == b", 3, 3, 0},
		{"负数", -3, 2, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AbsDiff(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("AbsDiff(%d, %d) = %d, want %d",
					tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestRound 测试 Round 函数
func TestRound(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected float64
	}{
		{"向上", 3.5, 4.0},
		{"向下", 3.4, 3.0},
		{"负数向上", -3.4, -3.0},
		{"负数向下", -3.5, -4.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Round(tt.value)
			if result != tt.expected {
				t.Errorf("Round(%f) = %f, want %f",
					tt.value, result, tt.expected)
			}
		})
	}
}

// TestRoundTo 测试 RoundTo 函数
func TestRoundTo(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		decimals int
		expected float64
	}{
		{"保留2位", 3.14159, 2, 3.14},
		{"保留0位", 3.14159, 0, 3.0},
		{"保留1位", 123.456, 1, 123.5},
		{"保留3位", 1.2345, 3, 1.235},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RoundTo(tt.value, tt.decimals)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("RoundTo(%f, %d) = %f, want %f",
					tt.value, tt.decimals, result, tt.expected)
			}
		})
	}
}

// TestCeil 测试 Ceil 函数
func TestCeil(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected float64
	}{
		{"正数", 3.14, 4.0},
		{"负数", -3.14, -3.0},
		{"整数", 3.0, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Ceil(tt.value)
			if result != tt.expected {
				t.Errorf("Ceil(%f) = %f, want %f",
					tt.value, result, tt.expected)
			}
		})
	}
}

// TestFloor 测试 Floor 函数
func TestFloor(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected float64
	}{
		{"正数", 3.14, 3.0},
		{"负数", -3.14, -4.0},
		{"整数", 3.0, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Floor(tt.value)
			if result != tt.expected {
				t.Errorf("Floor(%f) = %f, want %f",
					tt.value, result, tt.expected)
			}
		})
	}
}

// TestTrunc 测试 Trunc 函数
func TestTrunc(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected float64
	}{
		{"正数", 3.14, 3.0},
		{"负数", -3.14, -3.0},
		{"整数", 3.0, 3.0},
		{"大正数", 123.456, 123.0},
		{"大负数", -123.456, -123.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Trunc(tt.value)
			if result != tt.expected {
				t.Errorf("Trunc(%f) = %f, want %f",
					tt.value, result, tt.expected)
			}
		})
	}
}

// Benchmark 测试
func BenchmarkMin(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Min(3, 1, 4, 1, 5)
	}
}

func BenchmarkMax(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Max(3, 1, 4, 1, 5)
	}
}

func BenchmarkAbs(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Abs(-12345)
	}
}
