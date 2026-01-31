package mathx

// Abs 返回绝对值（泛型版本，支持有符号数）
//
// 参数:
//   - value: 要计算绝对值的数
//
// 返回:
//   - T: 绝对值
//
// 示例:
//
//	abs := mathx.Abs(-5)    // 5
//	abs := mathx.Abs(5)     // 5
//	abs := mathx.Abs(-3.14) // 3.14
func Abs[T Signed](value T) T {
	if value < 0 {
		return -value
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
