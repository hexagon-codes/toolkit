package reflectx

import (
	"fmt"
	"math/big"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

var synchronizationStateTypes = []reflect.Type{
	reflect.TypeFor[sync.Mutex](),
	reflect.TypeFor[sync.RWMutex](),
	reflect.TypeFor[sync.Once](),
	reflect.TypeFor[sync.WaitGroup](),
	reflect.TypeFor[sync.Map](),
	reflect.TypeFor[sync.Cond](),
	reflect.TypeFor[sync.Pool](),
	reflect.TypeFor[atomic.Bool](),
	reflect.TypeFor[atomic.Int32](),
	reflect.TypeFor[atomic.Int64](),
	reflect.TypeFor[atomic.Uint32](),
	reflect.TypeFor[atomic.Uint64](),
	reflect.TypeFor[atomic.Uintptr](),
	reflect.TypeFor[atomic.Value](),
}

// DeepCopyChecked 保类型复制数据，并拒绝无法隔离的非空函数、通道、裸指针及私有引用字段。
// 不支持的值返回错误和 T 的零值；源数据保持不变。循环引用沿用 DeepCopy 的处理。
// time.Time 按其不可变时区的值语义复制；其他私有字段包含引用的对象应通过所属类型的复制契约处理。
// 可访问的 big.Int 使用自身的 Set 复制合同，私有字段内不可访问的可变值仍不放行。
// 标准库 sync / sync/atomic 的非零状态类型（Mutex、RWMutex、Once、WaitGroup、atomic 等）
// 复制后不能保证可安全使用，显式拒绝；基于这些类型的命名类型同样按同步状态处理。
// 零值状态视为未使用，可复制为独立零值。
// sync.Locker 接口按动态值继续校验：nil 或自定义无同步状态实现可复制。
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
	// time.Time 的时区由 time 包按不可变值使用，复制时间值不需要隔离内部缓存。
	if v.Type() == reflect.TypeFor[time.Time]() {
		return nil
	}
	if v.Type() == reflect.TypeFor[big.Int]() && v.CanInterface() {
		return nil
	}
	if v.Kind() == reflect.Interface && v.IsNil() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		return validateCopy(v.Elem(), path, seen)
	}
	if copyTypeHasSynchronizationState(v.Type()) && !v.IsZero() {
		return fmt.Errorf("cannot isolate non-zero synchronization state at %s (%s)", path, v.Type())
	}
	switch v.Kind() {
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

func copyTypeHasSynchronizationState(t reflect.Type) bool {
	switch t.PkgPath() {
	case "sync", "sync/atomic":
		return true
	}
	// 命名类型（type MyMutex sync.Mutex）的 PkgPath 属于用户包，但底层布局相同；
	// 可转换为标准同步类型时仍按同步状态处理。
	for _, syncType := range synchronizationStateTypes {
		if t.ConvertibleTo(syncType) {
			return true
		}
	}
	return false
}

func copyHasReference(v reflect.Value) bool {
	if v.Type() == reflect.TypeFor[time.Time]() {
		return false
	}
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
