package optional

import "reflect"

// Option 表示一个可能存在也可能不存在的值
type Option[T any] struct {
	value   T
	present bool
}

// Some 创建一个包含值的 Option
//
// 参数:
//   - value: 要包装的值
//
// 返回:
//   - Option[T]: 包含值的 Option
//
// 示例:
//
//	opt := optional.Some(42)
//	opt := optional.Some("hello")
func Some[T any](value T) Option[T] {
	return Option[T]{value: value, present: true}
}

// None 创建一个空的 Option
//
// 返回:
//   - Option[T]: 空的 Option
//
// 示例:
//
//	opt := optional.None[int]()
//	opt := optional.None[string]()
func None[T any]() Option[T] {
	return Option[T]{}
}

// FromPtr 从指针创建 Option
//
// 参数:
//   - ptr: 指针，如果为 nil 则返回 None
//
// 返回:
//   - Option[T]: 如果指针非 nil 则为 Some(*ptr)，否则为 None
//
// 示例:
//
//	var name *string = &"Alice"
//	opt := optional.FromPtr(name)  // Some("Alice")
//
//	var empty *string
//	opt := optional.FromPtr(empty)  // None
func FromPtr[T any](ptr *T) Option[T] {
	if ptr == nil {
		return None[T]()
	}
	return Some(*ptr)
}

// FromValue 根据条件创建 Option
//
// 参数:
//   - value: 值
//   - ok: 如果为 true 则创建 Some，否则创建 None
//
// 返回:
//   - Option[T]: 根据条件创建的 Option
//
// 示例:
//
//	value, ok := someMap["key"]
//	opt := optional.FromValue(value, ok)
func FromValue[T any](value T, ok bool) Option[T] {
	if ok {
		return Some(value)
	}
	return None[T]()
}

// FromZero 如果值为零值则返回 None，否则返回 Some
//
// 参数:
//   - value: 要检查的值
//
// 返回:
//   - Option[T]: 如果非零值则为 Some，否则为 None
//
// 示例:
//
//	opt := optional.FromZero("")     // None
//	opt := optional.FromZero("hi")   // Some("hi")
//	opt := optional.FromZero(0)      // None
//	opt := optional.FromZero(42)     // Some(42)
func FromZero[T comparable](value T) Option[T] {
	var zero T
	if value == zero {
		return None[T]()
	}
	return Some(value)
}

// IsSome 检查 Option 是否包含值
//
// 返回:
//   - bool: 如果包含值返回 true
//
// 示例:
//
//	opt := optional.Some(42)
//	opt.IsSome()  // true
//
//	opt := optional.None[int]()
//	opt.IsSome()  // false
func (o Option[T]) IsSome() bool {
	return o.present
}

// IsNone 检查 Option 是否为空
//
// 返回:
//   - bool: 如果为空返回 true
//
// 示例:
//
//	opt := optional.None[int]()
//	opt.IsNone()  // true
func (o Option[T]) IsNone() bool {
	return !o.present
}

// Unwrap 获取 Option 中的值
//
// 返回:
//   - T: Option 中的值（如果为 None 则返回零值）
//
// 注意: 如果 Option 为 None，返回零值。建议先用 IsSome() 检查
//
// 示例:
//
//	opt := optional.Some(42)
//	value := opt.Unwrap()  // 42
func (o Option[T]) Unwrap() T {
	return o.value
}

// UnwrapOr 获取值，如果为 None 则返回默认值
//
// 参数:
//   - defaultVal: Option 为 None 时返回的默认值
//
// 返回:
//   - T: Option 中的值或默认值
//
// 示例:
//
//	opt := optional.None[int]()
//	value := opt.UnwrapOr(0)  // 0
func (o Option[T]) UnwrapOr(defaultVal T) T {
	if o.present {
		return o.value
	}
	return defaultVal
}

// UnwrapOrElse 获取值，如果为 None 则调用函数获取默认值
//
// 参数:
//   - fn: 返回默认值的函数
//
// 返回:
//   - T: Option 中的值或函数返回的值
//
// 示例:
//
//	opt := optional.None[int]()
//	value := opt.UnwrapOrElse(func() int { return computeDefault() })
func (o Option[T]) UnwrapOrElse(fn func() T) T {
	if o.present {
		return o.value
	}
	return fn()
}

// UnwrapOrZero 获取值，如果为 None 则返回零值（与 Unwrap 相同，但更明确语义）
//
// 返回:
//   - T: Option 中的值或类型的零值
func (o Option[T]) UnwrapOrZero() T {
	return o.value
}

// Expect 获取值，如果为 None 则 panic
//
// 参数:
//   - msg: panic 时的错误消息
//
// 返回:
//   - T: Option 中的值
//
// 示例:
//
//	opt := optional.Some(42)
//	value := opt.Expect("value should exist")  // 42
func (o Option[T]) Expect(msg string) T {
	if !o.present {
		panic(msg)
	}
	return o.value
}

// ToPtr 将 Option 转换为指针
//
// 返回:
//   - *T: 如果 Some 则返回值的指针，否则返回 nil
//
// 示例:
//
//	opt := optional.Some(42)
//	ptr := opt.ToPtr()  // *42
//
//	opt := optional.None[int]()
//	ptr := opt.ToPtr()  // nil
func (o Option[T]) ToPtr() *T {
	if o.present {
		return &o.value
	}
	return nil
}

// Filter 根据条件过滤 Option
//
// 参数:
//   - predicate: 过滤函数
//
// 返回:
//   - Option[T]: 如果 Some 且满足条件则返回原 Option，否则返回 None
//
// 示例:
//
//	opt := optional.Some(42)
//	filtered := opt.Filter(func(n int) bool { return n > 50 })  // None
func (o Option[T]) Filter(predicate func(T) bool) Option[T] {
	if o.present && predicate(o.value) {
		return o
	}
	return None[T]()
}

// Or 如果当前 Option 为 None，则返回另一个 Option
//
// 参数:
//   - other: 替代的 Option
//
// 返回:
//   - Option[T]: 当前 Option 或替代 Option
//
// 示例:
//
//	opt1 := optional.None[int]()
//	opt2 := optional.Some(42)
//	result := opt1.Or(opt2)  // Some(42)
func (o Option[T]) Or(other Option[T]) Option[T] {
	if o.present {
		return o
	}
	return other
}

// OrElse 如果当前 Option 为 None，则调用函数获取替代 Option
//
// 参数:
//   - fn: 返回替代 Option 的函数
//
// 返回:
//   - Option[T]: 当前 Option 或函数返回的 Option
func (o Option[T]) OrElse(fn func() Option[T]) Option[T] {
	if o.present {
		return o
	}
	return fn()
}

// And 如果当前 Option 为 Some，则返回另一个 Option
//
// 参数:
//   - other: 另一个 Option
//
// 返回:
//   - Option[U]: 如果当前为 Some 则返回 other，否则返回 None
//
// 示例:
//
//	opt1 := optional.Some(42)
//	opt2 := optional.Some("hello")
//	result := And(opt1, opt2)  // Some("hello")
func And[T, U any](o Option[T], other Option[U]) Option[U] {
	if o.present {
		return other
	}
	return None[U]()
}

// Map 转换 Option 中的值
//
// 参数:
//   - o: 输入 Option
//   - fn: 转换函数
//
// 返回:
//   - Option[U]: 转换后的 Option
//
// 示例:
//
//	opt := optional.Some(42)
//	result := optional.Map(opt, func(n int) string {
//	    return strconv.Itoa(n)
//	})  // Some("42")
func Map[T, U any](o Option[T], fn func(T) U) Option[U] {
	if o.present {
		return Some(fn(o.value))
	}
	return None[U]()
}

// MapOr 转换 Option 中的值，如果为 None 则返回默认值
//
// 参数:
//   - o: 输入 Option
//   - defaultVal: None 时的默认值
//   - fn: 转换函数
//
// 返回:
//   - U: 转换后的值或默认值
//
// 示例:
//
//	opt := optional.None[int]()
//	result := optional.MapOr(opt, "default", func(n int) string {
//	    return strconv.Itoa(n)
//	})  // "default"
func MapOr[T, U any](o Option[T], defaultVal U, fn func(T) U) U {
	if o.present {
		return fn(o.value)
	}
	return defaultVal
}

// FlatMap 链式转换 Option（返回 Option 的转换）
//
// 参数:
//   - o: 输入 Option
//   - fn: 转换函数，返回 Option
//
// 返回:
//   - Option[U]: 转换后的 Option
//
// 示例:
//
//	opt := optional.Some(42)
//	result := optional.FlatMap(opt, func(n int) Option[string] {
//	    if n > 0 {
//	        return optional.Some(strconv.Itoa(n))
//	    }
//	    return optional.None[string]()
//	})
func FlatMap[T, U any](o Option[T], fn func(T) Option[U]) Option[U] {
	if o.present {
		return fn(o.value)
	}
	return None[U]()
}

// Flatten 将嵌套的 Option 展平
//
// 参数:
//   - o: 嵌套的 Option
//
// 返回:
//   - Option[T]: 展平后的 Option
//
// 示例:
//
//	nested := optional.Some(optional.Some(42))
//	flat := optional.Flatten(nested)  // Some(42)
func Flatten[T any](o Option[Option[T]]) Option[T] {
	if o.present {
		return o.value
	}
	return None[T]()
}

// Zip 将两个 Option 组合成元组 Option
//
// 参数:
//   - o1: 第一个 Option
//   - o2: 第二个 Option
//
// 返回:
//   - Option[[2]any]: 如果两个都是 Some，返回包含两个值的 Option
//
// 注意: 由于 Go 泛型限制，返回数组类型。可以用 ZipWith 获得更好的类型
func Zip[T, U any](o1 Option[T], o2 Option[U]) Option[struct {
	First  T
	Second U
}] {
	if o1.present && o2.present {
		return Some(struct {
			First  T
			Second U
		}{o1.value, o2.value})
	}
	return None[struct {
		First  T
		Second U
	}]()
}

// ZipWith 使用函数组合两个 Option
//
// 参数:
//   - o1: 第一个 Option
//   - o2: 第二个 Option
//   - fn: 组合函数
//
// 返回:
//   - Option[R]: 组合后的 Option
//
// 示例:
//
//	opt1 := optional.Some(10)
//	opt2 := optional.Some(5)
//	result := optional.ZipWith(opt1, opt2, func(a, b int) int { return a + b })
//	// Some(15)
func ZipWith[T, U, R any](o1 Option[T], o2 Option[U], fn func(T, U) R) Option[R] {
	if o1.present && o2.present {
		return Some(fn(o1.value, o2.value))
	}
	return None[R]()
}

// Contains 检查 Option 是否包含指定值
//
// 参数:
//   - value: 要检查的值
//
// 返回:
//   - bool: 如果 Some 且值相等返回 true
//
// 示例:
//
//	opt := optional.Some(42)
//	opt.Contains(42)  // true
//	opt.Contains(43)  // false
func (o Option[T]) Contains(value T) bool {
	if !o.present {
		return false
	}
	// 使用 reflect.DeepEqual 进行比较，避免不可比较类型（如 slice/map）导致 panic
	return reflect.DeepEqual(o.value, value)
}

// String 返回 Option 的字符串表示
//
// 返回:
//   - string: "Some(value)" 或 "None"
func (o Option[T]) String() string {
	if o.present {
		return "Some(?)"
	}
	return "None"
}
