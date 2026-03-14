package mathx

// Abs 返回绝对值（泛型版本，支持有符号数）
//
// 参数:
//   - value: 要计算绝对值的数
//
// 返回:
//   - T: 绝对值
//
// 注意: 对于整数类型的最小值（如 math.MinInt64），由于补码表示法，
// 其绝对值无法用同类型表示，此时返回 math.MaxInt64（或对应类型的最大值）。
// 浮点数类型不受此限制。
//
// 示例:
//
//	abs := mathx.Abs(-5)    // 5
//	abs := mathx.Abs(5)     // 5
//	abs := mathx.Abs(-3.14) // 3.14
func Abs[T Signed](value T) T {
	if value < 0 {
		neg := -value
		// 整数类型最小值取反后仍为负数（溢出），返回该类型最大值
		// 对于有符号整数，-MinValue == MinValue，所以 neg 仍 < 0
		// 此时 -(value + 1) 得到最大值（例如 -(MinInt64+1) == MaxInt64）
		if neg < 0 {
			return -(value + 1)
		}
		return neg
	}
	return value
}

// AbsDiff 返回两个数的差的绝对值
//
// 参数:
//   - a: 第一个数
//   - b: 第二个数
//
// 返回:
//   - T: |a - b|
//
// 示例:
//
//	diff := mathx.AbsDiff(5, 3)   // 2
//	diff := mathx.AbsDiff(3, 5)   // 2
//	diff := mathx.AbsDiff(-3, 2)  // 5
func AbsDiff[T Signed](a, b T) T {
	if a > b {
		return a - b
	}
	return b - a
}
