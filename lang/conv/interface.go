package conv

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
