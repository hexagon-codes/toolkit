package mathx

// Ordered 是可排序类型的约束接口
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~string
}

// Signed 是有符号数类型的约束接口
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~float32 | ~float64
}

// Float 是浮点数类型的约束接口
type Float interface {
	~float32 | ~float64
}

// Min 返回多个数中的最小值（泛型版本）
//
// 参数:
//   - values: 要比较的值（至少一个）
//
// 返回:
//   - T: 最小值
//
// 示例:
//
//	min := mathx.Min(3, 1, 4, 1, 5)  // 1
//	minf := mathx.Min(3.14, 2.71, 1.41)  // 1.41
func Min[T Ordered](values ...T) T {
	if len(values) == 0 {
		var zero T
		return zero
	}

	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

// Max 返回多个数中的最大值（泛型版本）
//
// 参数:
//   - values: 要比较的值（至少一个）
//
// 返回:
//   - T: 最大值
//
// 示例:
//
//	max := mathx.Max(3, 1, 4, 1, 5)  // 5
//	maxf := mathx.Max(3.14, 2.71, 1.41)  // 3.14
func Max[T Ordered](values ...T) T {
	if len(values) == 0 {
		var zero T
		return zero
	}

	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// MinMax 同时返回最小值和最大值
//
// 参数:
//   - values: 要比较的值（至少一个）
//
// 返回:
//   - T: 最小值
//   - T: 最大值
//
// 示例:
//
//	min, max := mathx.MinMax(3, 1, 4, 1, 5)  // 1, 5
func MinMax[T Ordered](values ...T) (T, T) {
	if len(values) == 0 {
		var zero T
		return zero, zero
	}

	min, max := values[0], values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// Clamp 将值限制在指定范围内
//
// 参数:
//   - value: 要限制的值
//   - min: 最小值
//   - max: 最大值（必须 >= min，否则行为未定义）
//
// 返回:
//   - T: 限制后的值
//
// 注意：调用者必须保证 min <= max，函数不做运行时检查（出于性能考虑）。
// 如需边界检查，请使用 ClampSafe。
//
// 示例:
//
//	clamped := mathx.Clamp(15, 0, 10)  // 10
//	clamped := mathx.Clamp(-5, 0, 10)  // 0
//	clamped := mathx.Clamp(5, 0, 10)   // 5
func Clamp[T Ordered](value, min, max T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ClampSafe 将值限制在指定范围内（带边界检查）
//
// 与 Clamp 不同，当 min > max 时会自动交换它们的值。
// 性能略低于 Clamp，适用于参数可能来自外部输入的场景。
//
// 参数:
//   - value: 要限制的值
//   - min: 最小值
//   - max: 最大值
//
// 返回:
//   - T: 限制后的值
//
// 示例:
//
//	clamped := mathx.ClampSafe(5, 10, 0)   // 5（自动交换 min/max）
//	clamped := mathx.ClampSafe(15, 10, 0)  // 10
func ClampSafe[T Ordered](value, min, max T) T {
	if min > max {
		min, max = max, min
	}
	return Clamp(value, min, max)
}
