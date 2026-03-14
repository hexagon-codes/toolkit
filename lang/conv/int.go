package conv

import (
	"math"
	"strconv"
	"strings"
)

// Int 将任意类型转换为 int
//
// 支持的类型:
//   - int/int8/int16/int32/int64: 直接转换
//   - uint/uint8/uint16/uint32/uint64: 直接转换
//   - float32/float64: 截断为整数
//   - bool: true=1, false=0
//   - string: 解析为十进制整数
//   - []byte: 转为字符串后解析
//   - 其他: 转为字符串后解析
//
// 转换失败时返回 0
//
// 示例:
//
//	conv.Int("123")       // 123
//	conv.Int(45.67)       // 45
//	conv.Int(true)        // 1
//	conv.Int("invalid")   // 0
func Int(any any) int {
	return int(Int64(any))
}

// Int64 将任意类型转换为 int64
//
// 转换失败时返回 0
func Int64(any any) int64 {
	if any == nil {
		return 0
	}
	switch value := any.(type) {
	case int:
		return int64(value)
	case int8:
		return int64(value)
	case int16:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case uint:
		// 防止溢出：大于 math.MaxInt64 时返回 0
		if uint64(value) > math.MaxInt64 {
			return 0
		}
		return int64(value)
	case uint8:
		return int64(value)
	case uint16:
		return int64(value)
	case uint32:
		return int64(value)
	case uint64:
		// 防止溢出：大于 math.MaxInt64 时返回 0
		if value > math.MaxInt64 {
			return 0
		}
		return int64(value)
	case float32:
		// 检查 NaN 和 Inf
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return 0
		}
		// 检查溢出
		if value > math.MaxInt64 || value < math.MinInt64 {
			return 0
		}
		return int64(value)
	case float64:
		// 检查 NaN 和 Inf
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		// 检查溢出
		if value > math.MaxInt64 || value < math.MinInt64 {
			return 0
		}
		return int64(value)
	case bool:
		if value {
			return 1
		}
		return 0
	case []byte:
		v, _ := strconv.ParseInt(string(value), 10, 64)
		return v
	case string:
		v, _ := strconv.ParseInt(value, 10, 64)
		return v
	default:
		// 尝试转为字符串后解析
		v, _ := strconv.ParseInt(String(any), 10, 64)
		return v
	}
}

// TryInt64 将任意类型转换为 int64，返回是否成功
//
// 与 Int64 不同，此函数可以区分转换失败和实际值为 0 的情况
//
// 示例:
//
//	v, ok := conv.TryInt64("123")  // 123, true
//	v, ok := conv.TryInt64("abc")  // 0, false
//	v, ok := conv.TryInt64("0")    // 0, true
func TryInt64(any any) (int64, bool) {
	if any == nil {
		return 0, false
	}
	switch value := any.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		if uint64(value) > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value > math.MaxInt64 {
			return 0, false
		}
		return int64(value), true
	case float32:
		// 检查 NaN 和 Inf
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return 0, false
		}
		// 检查溢出
		if value > math.MaxInt64 || value < math.MinInt64 {
			return 0, false
		}
		return int64(value), true
	case float64:
		// 检查 NaN 和 Inf
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		// 检查溢出
		if value > math.MaxInt64 || value < math.MinInt64 {
			return 0, false
		}
		return int64(value), true
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	case []byte:
		v, err := strconv.ParseInt(string(value), 10, 64)
		return v, err == nil
	case string:
		v, err := strconv.ParseInt(value, 10, 64)
		return v, err == nil
	default:
		v, err := strconv.ParseInt(String(any), 10, 64)
		return v, err == nil
	}
}

// TryInt 将任意类型转换为 int，返回是否成功
func TryInt(any any) (int, bool) {
	v, ok := TryInt64(any)
	return int(v), ok
}

// Int32 将任意类型转换为 int32
//
// 转换失败或值超出 int32 范围时返回 0
func Int32(any any) int32 {
	v := Int64(any)
	// 防止溢出：超出 int32 范围时返回 0
	if v > math.MaxInt32 || v < math.MinInt32 {
		return 0
	}
	return int32(v)
}

// Uint 将任意类型转换为 uint
//
// 转换失败时返回 0
func Uint(any any) uint {
	return uint(Uint64(any))
}

// Uint64 将任意类型转换为 uint64
//
// 转换失败或负数时返回 0
func Uint64(any any) uint64 {
	if any == nil {
		return 0
	}
	switch value := any.(type) {
	case int:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case int8:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case int16:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case int32:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case int64:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case uint:
		return uint64(value)
	case uint8:
		return uint64(value)
	case uint16:
		return uint64(value)
	case uint32:
		return uint64(value)
	case uint64:
		return value
	case float32:
		// 检查 NaN 和 Inf
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return 0
		}
		if value < 0 {
			return 0
		}
		return uint64(value)
	case float64:
		// 检查 NaN 和 Inf
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		if value < 0 {
			return 0
		}
		return uint64(value)
	case bool:
		if value {
			return 1
		}
		return 0
	case []byte:
		v, _ := strconv.ParseUint(string(value), 10, 64)
		return v
	case string:
		v, _ := strconv.ParseUint(value, 10, 64)
		return v
	default:
		v, _ := strconv.ParseUint(String(any), 10, 64)
		return v
	}
}

// Uint32 将任意类型转换为 uint32
//
// 转换失败或值超出 uint32 范围时返回 0
func Uint32(any any) uint32 {
	v := Uint64(any)
	// 防止溢出：超出 uint32 范围时返回 0
	if v > math.MaxUint32 {
		return 0
	}
	return uint32(v)
}

// Bool 将任意类型转换为布尔值
//
// 支持的类型:
//   - bool: 直接返回
//   - int/uint: 0=false, 其他=true
//   - float: 0.0=false, 其他=true
//   - string: "true"/"1"/"yes"/"on"=true, 其他=false (不区分大小写)
//   - nil: false
//
// 示例:
//
//	conv.Bool(1)        // true
//	conv.Bool(0)        // false
//	conv.Bool("true")   // true
//	conv.Bool("yes")    // true
func Bool(any any) bool {
	if any == nil {
		return false
	}
	switch value := any.(type) {
	case bool:
		return value
	case int, int8, int16, int32, int64:
		return Int64(value) != 0
	case uint, uint8, uint16, uint32, uint64:
		return Uint64(value) != 0
	case float32, float64:
		return Float64(value) != 0
	case string:
		return parseBoolExtended(value)
	case []byte:
		return parseBoolExtended(string(value))
	default:
		return parseBoolExtended(String(any))
	}
}

// parseBoolExtended 扩展的布尔值解析
// 除了 strconv.ParseBool 支持的值外，还支持 "yes"/"no"/"on"/"off"（不区分大小写）
func parseBoolExtended(s string) bool {
	v, err := strconv.ParseBool(s)
	if err == nil {
		return v
	}
	// strconv.ParseBool 不支持的扩展值
	switch strings.ToLower(s) {
	case "yes", "on":
		return true
	case "no", "off":
		return false
	default:
		return false
	}
}
