package reflectx

import (
	"reflect"
)

// IsZero 检查值是否为零值
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是零值返回 true
//
// 示例:
//
//	reflectx.IsZero(0)        // true
//	reflectx.IsZero("")       // true
//	reflectx.IsZero(nil)      // true
//	reflectx.IsZero(1)        // false
//	reflectx.IsZero("hello")  // false
func IsZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.IsZero()
}

// IsNil 检查值是否为 nil
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是 nil 返回 true
//
// 注意: 只检查可以为 nil 的类型（指针、接口、切片、map、channel、函数）
//
// 示例:
//
//	var p *int
//	reflectx.IsNil(p)     // true
//	reflectx.IsNil(nil)   // true
//	reflectx.IsNil(&p)    // false（指针本身不是 nil）
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

// TypeName 返回值的类型名称
//
// 参数:
//   - v: 要获取类型的值
//
// 返回:
//   - string: 类型名称
//
// 示例:
//
//	reflectx.TypeName(42)        // "int"
//	reflectx.TypeName("hello")   // "string"
//	reflectx.TypeName(User{})    // "User"
func TypeName(v any) string {
	if v == nil {
		return "nil"
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		return "*" + t.Elem().Name()
	}
	return t.Name()
}

// FullTypeName 返回值的完整类型名称（包含包路径）
//
// 参数:
//   - v: 要获取类型的值
//
// 返回:
//   - string: 完整类型名称
//
// 示例:
//
//	reflectx.FullTypeName(User{})  // "package.User"
func FullTypeName(v any) string {
	if v == nil {
		return "nil"
	}
	t := reflect.TypeOf(v)
	return t.String()
}

// KindOf 返回值的 Kind
//
// 参数:
//   - v: 要获取 Kind 的值
//
// 返回:
//   - reflect.Kind: Kind 值
func KindOf(v any) reflect.Kind {
	if v == nil {
		return reflect.Invalid
	}
	return reflect.TypeOf(v).Kind()
}

// IsPtr 检查值是否为指针
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是指针返回 true
func IsPtr(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Pointer
}

// IsStruct 检查值是否为结构体
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是结构体返回 true
func IsStruct(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}

// IsSlice 检查值是否为切片
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是切片返回 true
func IsSlice(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Slice
}

// IsMap 检查值是否为 map
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是 map 返回 true
func IsMap(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Map
}

// IsFunc 检查值是否为函数
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是函数返回 true
func IsFunc(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Func
}

// IsChan 检查值是否为 channel
//
// 参数:
//   - v: 要检查的值
//
// 返回:
//   - bool: 如果是 channel 返回 true
func IsChan(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Chan
}

// Indirect 返回值的非指针版本
//
// 参数:
//   - v: 要处理的值
//
// 返回:
//   - any: 如果是指针则返回指向的值，否则返回原值
func Indirect(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	return rv.Interface()
}

// New 创建类型的新实例
//
// 参数:
//   - v: 类型模板
//
// 返回:
//   - any: 新实例的指针
//
// 示例:
//
//	type User struct { Name string }
//	ptr := reflectx.New(User{})  // *User
func New(v any) any {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return reflect.New(t).Interface()
}

// SliceLen 返回切片长度
//
// 参数:
//   - v: 切片
//
// 返回:
//   - int: 长度，如果不是切片返回 -1
func SliceLen(v any) int {
	if v == nil {
		return -1
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return -1
	}
	return rv.Len()
}

// MapLen 返回 map 长度
//
// 参数:
//   - v: map
//
// 返回:
//   - int: 长度，如果不是 map 返回 -1
func MapLen(v any) int {
	if v == nil {
		return -1
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return -1
	}
	return rv.Len()
}
