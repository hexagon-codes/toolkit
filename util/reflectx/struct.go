package reflectx

import (
	"fmt"
	"reflect"
	"strings"
)

// StructToMap 将结构体转换为 map
//
// 参数:
//   - v: 结构体或结构体指针
//
// 返回:
//   - map[string]any: 字段名到值的映射
//
// 注意: 只转换导出字段，使用字段名作为 key
//
// 示例:
//
//	type User struct {
//	    Name string
//	    Age  int
//	}
//	m := reflectx.StructToMap(User{Name: "Alice", Age: 20})
//	// map[string]any{"Name": "Alice", "Age": 20}
func StructToMap(v any) map[string]any {
	return StructToMapWithTag(v, "")
}

// StructToMapWithTag 将结构体转换为 map，使用指定的 tag 作为 key
//
// 参数:
//   - v: 结构体或结构体指针
//   - tagName: tag 名称，如 "json"，为空则使用字段名
//
// 返回:
//   - map[string]any: tag 值/字段名到值的映射
//
// 示例:
//
//	type User struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//	m := reflectx.StructToMapWithTag(User{Name: "Alice", Age: 20}, "json")
//	// map[string]any{"name": "Alice", "age": 20}
func StructToMapWithTag(v any, tagName string) map[string]any {
	rv, ok := readableStructValue(v)
	if !ok {
		return nil
	}

	result := make(map[string]any)
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		key := field.Name
		if tagName != "" {
			if tag := field.Tag.Get(tagName); tag != "" {
				// 处理 tag 中的选项（如 `json:"name,omitempty"`）
				if idx := strings.Index(tag, ","); idx != -1 {
					tag = tag[:idx]
				}
				if tag == "-" {
					continue
				}
				if tag != "" {
					key = tag
				}
			}
		}

		result[key] = rv.Field(i).Interface()
	}
	return result
}

// MapToStruct 将 map 转换为结构体
//
// 参数:
//   - m: 源 map
//   - v: 目标结构体指针
//
// 返回:
//   - error: 转换错误
//
// 注意: 只设置导出字段，使用字段名匹配 key（不区分大小写）
//
// 示例:
//
//	data := map[string]any{"Name": "Alice", "Age": 20}
//	var user User
//	err := reflectx.MapToStruct(data, &user)
func MapToStruct(m map[string]any, v any) error {
	return MapToStructWithTag(m, v, "")
}

// MapToStructWithTag 将 map 转换为结构体，使用指定的 tag 匹配 key
//
// 参数:
//   - m: 源 map
//   - v: 目标结构体指针
//   - tagName: tag 名称
//
// 返回:
//   - error: 转换错误
func MapToStructWithTag(m map[string]any, v any, tagName string) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("v must be a non-nil pointer to struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("v must be a pointer to struct")
	}

	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		// 确定要查找的 key
		key := field.Name
		if tagName != "" {
			if tag := field.Tag.Get(tagName); tag != "" {
				if idx := strings.Index(tag, ","); idx != -1 {
					tag = tag[:idx]
				}
				if tag == "-" {
					continue
				}
				if tag != "" {
					key = tag
				}
			}
		}

		// 查找 map 中的值（大小写不敏感）
		var value any
		var found bool
		for k, v := range m {
			if strings.EqualFold(k, key) {
				value = v
				found = true
				break
			}
		}
		if !found {
			continue
		}

		// 设置字段值
		fieldValue := rv.Field(i)
		if !fieldValue.CanSet() {
			continue
		}

		if err := setFieldValue(fieldValue, value); err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}
	}
	return nil
}

// setFieldValue 设置字段值
func setFieldValue(field reflect.Value, value any) error {
	if value == nil {
		switch field.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			field.SetZero()
			return nil
		default:
			return fmt.Errorf("cannot assign nil to %v", field.Type())
		}
	}

	rv := reflect.ValueOf(value)
	if rv.Type().AssignableTo(field.Type()) {
		field.Set(rv)
		return nil
	}

	if rv.Type().ConvertibleTo(field.Type()) {
		field.Set(rv.Convert(field.Type()))
		return nil
	}

	return fmt.Errorf("cannot assign %v to %v", rv.Type(), field.Type())
}

// GetField 获取结构体字段值
//
// 参数:
//   - v: 结构体或结构体指针
//   - name: 字段名
//
// 返回:
//   - any: 字段值
//   - bool: 是否找到
//
// 示例:
//
//	user := User{Name: "Alice", Age: 20}
//	name, ok := reflectx.GetField(user, "Name")
//	// "Alice", true
func GetField(v any, name string) (any, bool) {
	rv, ok := readableStructValue(v)
	if !ok {
		return nil, false
	}

	field := rv.FieldByName(name)
	if !field.IsValid() || !field.CanInterface() {
		return nil, false
	}
	return field.Interface(), true
}

// GetFieldValue 获取结构体字段值（泛型版本）
//
// 参数:
//   - v: 结构体或结构体指针
//   - name: 字段名
//
// 返回:
//   - T: 字段值
//   - bool: 是否找到且类型匹配
//
// 示例:
//
//	user := User{Name: "Alice", Age: 20}
//	name, ok := reflectx.GetFieldValue[string](user, "Name")
func GetFieldValue[T any](v any, name string) (T, bool) {
	var zero T
	value, ok := GetField(v, name)
	if !ok {
		return zero, false
	}
	result, ok := value.(T)
	return result, ok
}

// SetField 设置结构体字段值
//
// 参数:
//   - v: 结构体指针
//   - name: 字段名
//   - value: 要设置的值
//
// 返回:
//   - error: 设置错误
//
// 示例:
//
//	user := &User{Name: "Alice", Age: 20}
//	err := reflectx.SetField(user, "Age", 21)
func SetField(v any, name string, value any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("v must be a non-nil pointer to struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("v must be a pointer to struct")
	}

	field := rv.FieldByName(name)
	if !field.IsValid() {
		return fmt.Errorf("field %s not found", name)
	}
	if !field.CanSet() {
		return fmt.Errorf("field %s cannot be set", name)
	}

	return setFieldValue(field, value)
}

// HasField 检查结构体是否有指定字段
//
// 参数:
//   - v: 结构体或结构体指针
//   - name: 字段名
//
// 返回:
//   - bool: 是否有该字段
func HasField(v any, name string) bool {
	rv, ok := readableStructValue(v)
	if !ok {
		return false
	}
	field := rv.FieldByName(name)
	return field.IsValid() && field.CanInterface()
}

// FieldNames 返回结构体所有导出字段名
//
// 参数:
//   - v: 结构体或结构体指针
//
// 返回:
//   - []string: 字段名列表
func FieldNames(v any) []string {
	rv, ok := readableStructValue(v)
	if !ok {
		return nil
	}

	rt := rv.Type()
	names := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.IsExported() {
			names = append(names, field.Name)
		}
	}
	return names
}

// FieldTags 返回结构体字段的 tag 值
//
// 参数:
//   - v: 结构体或结构体指针
//   - tagName: tag 名称
//
// 返回:
//   - map[string]string: 字段名到 tag 值的映射
func FieldTags(v any, tagName string) map[string]string {
	rv, ok := readableStructValue(v)
	if !ok {
		return nil
	}

	rt := rv.Type()
	result := make(map[string]string)
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		if tag := field.Tag.Get(tagName); tag != "" {
			result[field.Name] = tag
		}
	}
	return result
}

type pointerIdentity struct {
	typeOf  reflect.Type
	pointer uintptr
}

func readableStructValue(v any) (reflect.Value, bool) {
	if v == nil {
		return reflect.Value{}, false
	}
	rv := reflect.ValueOf(v)
	visited := make(map[pointerIdentity]struct{})
	for {
		switch rv.Kind() {
		case reflect.Interface:
			if rv.IsNil() {
				return reflect.Value{}, false
			}
			rv = rv.Elem()
		case reflect.Pointer:
			if rv.IsNil() {
				return reflect.Value{}, false
			}
			identity := pointerIdentity{typeOf: rv.Type(), pointer: rv.Pointer()}
			if _, exists := visited[identity]; exists {
				return reflect.Value{}, false
			}
			visited[identity] = struct{}{}
			rv = rv.Elem()
		default:
			return rv, rv.Kind() == reflect.Struct
		}
	}
}
