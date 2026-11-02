package reflectx

import (
	"fmt"
	"reflect"
)

// DeepCopyChecked 保类型复制数据，并拒绝无法隔离的非空函数、通道、裸指针及私有引用字段。
// 不支持的值返回错误和 T 的零值；源数据保持不变。循环引用沿用 DeepCopy 的处理。
// 私有字段包含引用的自定义对象应通过所属类型的复制契约处理。
func DeepCopyChecked[T any](src T) (T, error) {
	if err := validateCopy(reflect.ValueOf(src), "$", make(map[copyIdentity]bool)); err != nil {
		var zero T
		return zero, err
	}
	value := reflect.ValueOf(&src).Elem()
	if value.Kind() == reflect.Interface && value.IsNil() {
		var zero T
		return zero, nil
	}
	result := deepCopyValue(value, make(map[copyIdentity]reflect.Value))
	if result.IsValid() {
		if copied, ok := result.Interface().(T); ok {
			return copied, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("cannot preserve copied value type %s", value.Type())
}

func validateCopy(v reflect.Value, path string, seen map[copyIdentity]bool) error {
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return validateCopy(v.Elem(), path, seen)
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return nil
		}
		id := identityOf(v)
		if seen[id] {
			return nil
		}
		seen[id] = true
		if v.Kind() == reflect.Pointer {
			return validateCopy(v.Elem(), path+"*", seen)
		}
		if v.Kind() == reflect.Map {
			iter := v.MapRange()
			for iter.Next() {
				if err := validateCopy(iter.Key(), path+"[key]", seen); err != nil {
					return err
				}
				if err := validateCopy(iter.Value(), path+"[value]", seen); err != nil {
					return err
				}
			}
			return nil
		}
		for i := range v.Len() {
			if err := validateCopy(v.Index(i), fmt.Sprintf("%s[%d]", path, i), seen); err != nil {
				return err
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			if err := validateCopy(v.Index(i), fmt.Sprintf("%s[%d]", path, i), seen); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			value := v.Field(i)
			fieldPath := path + "." + field.Name
			if !field.IsExported() && copyHasReference(value) {
				return fmt.Errorf("cannot isolate unexported field %s (%s)", fieldPath, field.Type)
			}
			if err := validateCopy(value, fieldPath, seen); err != nil {
				return err
			}
		}
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		if !v.IsZero() {
			return fmt.Errorf("cannot isolate value at %s (%s)", path, v.Type())
		}
	}
	return nil
}

func copyHasReference(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return !v.IsZero()
	case reflect.Interface:
		return !v.IsNil() && copyHasReference(v.Elem())
	case reflect.Array:
		for i := range v.Len() {
			if copyHasReference(v.Index(i)) {
				return true
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if copyHasReference(v.Field(i)) {
				return true
			}
		}
	}
	return false
}
