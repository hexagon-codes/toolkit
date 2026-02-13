package reflectx

import (
	"reflect"
)

// DeepCopy 深度拷贝值
//
// 参数:
//   - src: 源值
//
// 返回:
//   - T: 拷贝后的值
//
// 注意: 支持基本类型、结构体、切片、map、指针
// 对于不支持的类型（如 chan、func）返回零值
// 自动检测并处理循环引用，避免无限递归
//
// 示例:
//
//	type User struct { Name string }
//	user := User{Name: "Alice"}
//	copied := reflectx.DeepCopy(user)  // 独立副本
func DeepCopy[T any](src T) T {
	// 使用 &src 间接获取 reflect.Value，避免 reflect.ValueOf(nil interface) 返回无效值
	v := reflect.ValueOf(&src).Elem()
	if !v.IsValid() {
		var zero T
		return zero
	}
	// 当 T 为接口类型且值为 nil 时（如 var x any = nil），直接返回零值
	if v.Kind() == reflect.Interface && v.IsNil() {
		var zero T
		return zero
	}
	visited := make(map[uintptr]reflect.Value)
	result := deepCopyValue(v, visited)
	if !result.IsValid() {
		var zero T
		return zero
	}
	return result.Interface().(T)
}

// deepCopyValue 递归深拷贝 reflect.Value
// visited 用于记录已访问的指针地址，防止循环引用导致无限递归
func deepCopyValue(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	if !src.IsValid() {
		return src
	}

	switch src.Kind() {
	case reflect.Ptr:
		return deepCopyPtr(src, visited)
	case reflect.Interface:
		return deepCopyInterface(src, visited)
	case reflect.Struct:
		return deepCopyStruct(src, visited)
	case reflect.Slice:
		return deepCopySlice(src, visited)
	case reflect.Map:
		return deepCopyMap(src, visited)
	case reflect.Array:
		return deepCopyArray(src, visited)
	default:
		// 基本类型直接复制
		dst := reflect.New(src.Type()).Elem()
		dst.Set(src)
		return dst
	}
}

// deepCopyPtr 深拷贝指针
func deepCopyPtr(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}

	// 检测循环引用：如果指针地址已访问过，返回之前创建的副本
	ptr := src.Pointer()
	if existing, ok := visited[ptr]; ok {
		return existing
	}

	// 先创建目标指针并记录，防止循环引用时无限递归
	dst := reflect.New(src.Type().Elem())
	visited[ptr] = dst

	// 递归拷贝指针指向的值
	dst.Elem().Set(deepCopyValue(src.Elem(), visited))
	return dst
}

// deepCopyInterface 深拷贝接口
func deepCopyInterface(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	return deepCopyValue(src.Elem(), visited)
}

// deepCopyStruct 深拷贝结构体
func deepCopyStruct(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	for i := range src.NumField() {
		srcField := src.Field(i)
		dstField := dst.Field(i)
		if dstField.CanSet() {
			dstField.Set(deepCopyValue(srcField, visited))
		}
	}
	return dst
}

// deepCopySlice 深拷贝切片
func deepCopySlice(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}

	// 检测循环引用：切片底层数组可能被多次引用
	ptr := src.Pointer()
	if existing, ok := visited[ptr]; ok {
		return existing
	}

	dst := reflect.MakeSlice(src.Type(), src.Len(), src.Cap())
	visited[ptr] = dst

	for i := range src.Len() {
		dst.Index(i).Set(deepCopyValue(src.Index(i), visited))
	}
	return dst
}

// deepCopyMap 深拷贝 map
func deepCopyMap(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}

	// 检测循环引用
	ptr := src.Pointer()
	if existing, ok := visited[ptr]; ok {
		return existing
	}

	dst := reflect.MakeMap(src.Type())
	visited[ptr] = dst

	for _, key := range src.MapKeys() {
		dst.SetMapIndex(deepCopyValue(key, visited), deepCopyValue(src.MapIndex(key), visited))
	}
	return dst
}

// deepCopyArray 深拷贝数组
func deepCopyArray(src reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	for i := range src.Len() {
		dst.Index(i).Set(deepCopyValue(src.Index(i), visited))
	}
	return dst
}

// Clone 浅拷贝值（仅拷贝顶层）
//
// 参数:
//   - src: 源值
//
// 返回:
//   - T: 拷贝后的值
//
// 注意: 对于指针、切片、map 等引用类型，仅拷贝引用
func Clone[T any](src T) T {
	dst := reflect.New(reflect.TypeOf(src)).Elem()
	dst.Set(reflect.ValueOf(src))
	return dst.Interface().(T)
}
