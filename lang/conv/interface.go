package conv

import "reflect"

// iString 用于 String() 方法的类型断言接口
type iString interface {
	String() string
}

// iFloat32 用于 Float32() 方法的类型断言接口
type iFloat32 interface {
	Float32() float32
}

// iFloat64 用于 Float64() 方法的类型断言接口
type iFloat64 interface {
	Float64() float64
}

// isNilValue 识别装入接口后的 typed nil。
func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
