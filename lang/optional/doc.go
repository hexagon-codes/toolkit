// Package optional 提供 Option[T] 类型，用于显式表示可能缺失的值
//
// Option 类型是一种函数式编程模式，用于替代 nil 指针，
// 使代码更加类型安全和可读。
//
// 主要类型:
//   - Option[T]: 可能包含值的容器
//
// 主要函数:
//   - Some: 创建包含值的 Option
//   - None: 创建空的 Option
//   - FromPtr: 从指针创建 Option
//   - Map: 转换 Option 中的值
//   - FlatMap: 链式转换 Option
//
// 示例:
//
//	// 创建 Option
//	opt := optional.Some(42)
//	empty := optional.None[int]()
//
//	// 检查并获取值
//	if opt.IsSome() {
//	    value := opt.Unwrap()
//	}
//
//	// 使用默认值
//	value := opt.UnwrapOr(0)
//
//	// 链式转换
//	result := optional.Map(opt, func(n int) string {
//	    return strconv.Itoa(n)
//	})
//
// --- English ---
//
// Package optional provides the Option[T] type for explicitly representing
// values that may be absent.
//
// The Option type is a functional programming pattern used as an alternative
// to nil pointers, making code more type-safe and readable.
//
// Main types:
//   - Option[T]: a container that may or may not hold a value
//
// Main functions:
//   - Some: create an Option containing a value
//   - None: create an empty Option
//   - FromPtr: create an Option from a pointer
//   - Map: transform the value inside an Option
//   - FlatMap: chain Option transformations
//
// Example:
//
//	// Create an Option
//	opt := optional.Some(42)
//	empty := optional.None[int]()
//
//	// Check and retrieve value
//	if opt.IsSome() {
//	    value := opt.Unwrap()
//	}
//
//	// Use a default value
//	value := opt.UnwrapOr(0)
//
//	// Chain transformations
//	result := optional.Map(opt, func(n int) string {
//	    return strconv.Itoa(n)
//	})
package optional
