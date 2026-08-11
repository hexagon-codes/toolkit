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
// 注意: 支持基本类型、结构体、切片、map、指针，并自动保留循环引用关系。
// 未导出字段按值保留，其内部引用不会通过 unsafe 强行递归复制；chan、func 等
// 无法安全复制的引用类型保持原值。
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
	visited := make(map[copyIdentity]reflect.Value)
	result := deepCopyValue(v, visited)
	if !result.IsValid() {
		var zero T
		return zero
	}
	copied, ok := result.Interface().(T)
	if !ok {
		var zero T
		return zero
	}
	return copied
}

// deepCopyValue 递归深拷贝 reflect.Value
// visited 用于记录已访问的指针地址，防止循环引用导致无限递归
func deepCopyValue(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	if !src.IsValid() {
		return src
	}

	switch src.Kind() {
	case reflect.Pointer:
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
func deepCopyPtr(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}

	// 检测循环引用：如果指针地址已访问过，返回之前创建的副本
	identity := identityOf(src)
	if existing, ok := visited[identity]; ok {
		return existing
	}

	// 先创建目标指针并记录，防止循环引用时无限递归
	dst := reflect.New(src.Type().Elem())
	visited[identity] = dst

	// 递归拷贝指针指向的值
	dst.Elem().Set(deepCopyValue(src.Elem(), visited))
	return dst
}

// deepCopyInterface 深拷贝接口
func deepCopyInterface(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	return deepCopyValue(src.Elem(), visited)
}

// deepCopyStruct 深拷贝结构体
func deepCopyStruct(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	// 先保留完整值，再递归替换可安全访问的导出字段。
	dst.Set(src)
	for i := range src.NumField() {
		srcField := src.Field(i)
		dstField := dst.Field(i)
		if srcField.CanInterface() && dstField.CanSet() {
			dstField.Set(deepCopyValue(srcField, visited))
		}
	}
	return dst
}

// deepCopySlice 深拷贝切片
func deepCopySlice(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}

	// 检测循环引用：切片底层数组可能被多次引用
	identity := identityOf(src)
	if existing, ok := visited[identity]; ok {
		return existing
	}

	dst := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
	visited[identity] = dst

	for i := range src.Len() {
		dst.Index(i).Set(deepCopyValue(src.Index(i), visited))
	}
	return dst
}

// deepCopyMap 深拷贝 map
func deepCopyMap(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}

	// 检测循环引用
	identity := identityOf(src)
	if existing, ok := visited[identity]; ok {
		return existing
	}

	dst := reflect.MakeMap(src.Type())
	visited[identity] = dst

	for _, key := range src.MapKeys() {
		dst.SetMapIndex(deepCopyValue(key, visited), deepCopyValue(src.MapIndex(key), visited))
	}
	return dst
}

// deepCopyArray 深拷贝数组
func deepCopyArray(src reflect.Value, visited map[copyIdentity]reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	for i := range src.Len() {
		dst.Index(i).Set(deepCopyValue(src.Index(i), visited))
	}
	return dst
}

type copyIdentity struct {
	kind     reflect.Kind
	typeOf   reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

func identityOf(value reflect.Value) copyIdentity {
	identity := copyIdentity{
		kind:    value.Kind(),
		typeOf:  value.Type(),
		pointer: value.Pointer(),
	}
	if value.Kind() == reflect.Slice {
		identity.length = value.Len()
		identity.capacity = value.Cap()
	}
	return identity
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
// 对于 nil interface 直接返回零值
func Clone[T any](src T) T {
	v := reflect.ValueOf(&src).Elem()
	// 当 T 为接口类型且值为 nil 时，reflect.TypeOf 会 panic，需要特殊处理
	if v.Kind() == reflect.Interface && v.IsNil() {
		var zero T
		return zero
	}
	dst := reflect.New(reflect.TypeOf(src)).Elem()
	dst.Set(reflect.ValueOf(src))
	cloned, ok := dst.Interface().(T)
	if !ok {
		var zero T
		return zero
	}
	return cloned
}
