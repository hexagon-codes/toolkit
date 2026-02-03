package conv

import (
	"math"
	"strconv"
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
		return int64(value)
	case float64:
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

// Int32 将任意类型转换为 int32
//
// 转换失败时返回 0
func Int32(any any) int32 {
	return int32(Int64(any))
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
		if value < 0 {
			return 0
		}
		return uint64(value)
	case float64:
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
// 转换失败时返回 0
func Uint32(any any) uint32 {
	return uint32(Uint64(any))
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
		v, _ := strconv.ParseBool(value)
		return v
	case []byte:
		v, _ := strconv.ParseBool(string(value))
		return v
	default:
		v, _ := strconv.ParseBool(String(any))
		return v
	}
}
