package slicex

import "github.com/everyday-items/toolkit/lang/mathx"

// Range 生成一个数字范围切片
//
// 参数:
//   - start: 起始值（包含）
//   - end: 结束值（不包含）
//   - step: 步长（必须非零）
//
// 返回:
//   - []T: 数字范围切片
//
// 注意: 无符号整数类型（uint, uint8, uint16, uint32, uint64, uintptr）不支持负步长，
// 因为无符号类型无法表示负数，负步长条件永远不会生效。请对无符号类型仅使用正步长。
//
// 示例:
//
//	slicex.Range(0, 5, 1)   // [0, 1, 2, 3, 4]
//	slicex.Range(0, 10, 2)  // [0, 2, 4, 6, 8]
//	slicex.Range(5, 0, -1)  // [5, 4, 3, 2, 1]（仅适用于有符号类型）
func Range[T mathx.Signed | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr](start, end, step T) []T {
	if step == 0 {
		return nil
	}

	// 计算长度
	var length int
	if step > 0 {
		if end <= start {
			return nil
		}
		length = int((end - start + step - 1) / step)
	} else {
		if end >= start {
			return nil
		}
		length = int((start - end - step - 1) / (-step))
	}

	result := make([]T, length)
	for i := range result {
		result[i] = start + T(i)*step
	}
	return result
}

// RangeN 生成从 0 到 n-1 的整数切片
//
// 参数:
//   - n: 范围大小
//
// 返回:
//   - []int: [0, 1, 2, ..., n-1]
//
// 示例:
//
//	slicex.RangeN(5)  // [0, 1, 2, 3, 4]
func RangeN(n int) []int {
	if n <= 0 {
		return nil
	}
	result := make([]int, n)
	for i := range result {
		result[i] = i
	}
	return result
}

// RangeFrom 从 start 开始生成 n 个连续整数
//
// 参数:
//   - start: 起始值
//   - n: 数量
//
// 返回:
//   - []int: [start, start+1, ..., start+n-1]
//
// 示例:
//
//	slicex.RangeFrom(5, 3)  // [5, 6, 7]
func RangeFrom(start, n int) []int {
	if n <= 0 {
		return nil
	}
	result := make([]int, n)
	for i := range result {
		result[i] = start + i
	}
	return result
}

// Repeat 重复一个值 n 次
//
// 参数:
//   - value: 要重复的值
//   - n: 重复次数
//
// 返回:
//   - []T: 包含 n 个相同值的切片
//
// 示例:
//
//	slicex.Repeat("hello", 3)  // ["hello", "hello", "hello"]
//	slicex.Repeat(0, 5)        // [0, 0, 0, 0, 0]
func Repeat[T any](value T, n int) []T {
	if n <= 0 {
		return nil
	}
	result := make([]T, n)
	for i := range result {
		result[i] = value
	}
	return result
}

// RepeatFunc 使用函数生成 n 个值
//
// 参数:
//   - n: 生成次数
//   - fn: 生成函数，接收索引
//
// 返回:
//   - []T: 生成的切片
//
// 示例:
//
//	slicex.RepeatFunc(3, func(i int) int { return i * 2 })  // [0, 2, 4]
func RepeatFunc[T any](n int, fn func(int) T) []T {
	if n <= 0 {
		return nil
	}
	result := make([]T, n)
	for i := range result {
		result[i] = fn(i)
	}
	return result
}
