package mathx

import "math"

// Round 四舍五入到整数
//
// 参数:
//   - value: 要四舍五入的浮点数
//
// 返回:
//   - float64: 四舍五入后的值
//
// 示例:
//
//	rounded := mathx.Round(3.14)   // 3.0
//	rounded := mathx.Round(3.5)    // 4.0
//	rounded := mathx.Round(-3.5)   // -4.0
func Round(value float64) float64 {
	return math.Round(value)
}

// RoundTo 四舍五入到指定小数位
//
// 参数:
//   - value: 要四舍五入的浮点数
//   - decimals: 保留的小数位数
//
// 返回:
//   - float64: 四舍五入后的值
//
// 示例:
//
//	rounded := mathx.RoundTo(3.14159, 2)   // 3.14
//	rounded := mathx.RoundTo(3.14159, 0)   // 3.0
//	rounded := mathx.RoundTo(123.456, 1)   // 123.5
func RoundTo(value float64, decimals int) float64 {
	shift := math.Pow(10, float64(decimals))
	return math.Round(value*shift) / shift
}

// Ceil 向上取整
//
// 参数:
//   - value: 要取整的浮点数
//
// 返回:
//   - float64: 向上取整后的值
//
// 示例:
//
//	ceiled := mathx.Ceil(3.14)   // 4.0
//	ceiled := mathx.Ceil(-3.14)  // -3.0
func Ceil(value float64) float64 {
	return math.Ceil(value)
}

// Floor 向下取整
//
// 参数:
//   - value: 要取整的浮点数
//
// 返回:
//   - float64: 向下取整后的值
//
// 示例:
//
//	floored := mathx.Floor(3.14)   // 3.0
//	floored := mathx.Floor(-3.14)  // -4.0
func Floor(value float64) float64 {
	return math.Floor(value)
}

// Trunc 截断小数部分
//
// 参数:
//   - value: 要截断的浮点数
//
// 返回:
//   - float64: 截断后的值
//
// 示例:
//
//	truncated := mathx.Trunc(3.14)   // 3.0
//	truncated := mathx.Trunc(-3.14)  // -3.0
func Trunc(value float64) float64 {
	return math.Trunc(value)
}
