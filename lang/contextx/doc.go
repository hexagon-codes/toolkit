// Package contextx 提供类型安全的 context 值操作工具
//
// 与标准库 context.WithValue 使用 interface{} 作为键和值不同，
// contextx 使用泛型确保编译时类型安全。
//
// 基本用法:
//
//	type userKey struct{}
//	ctx := contextx.WithValue(context.Background(), userKey{}, user)
//	user, ok := contextx.Value[*User](ctx, userKey{})
//
// --- English ---
//
// Package contextx provides type-safe context value handling.
//
// Unlike the standard context.WithValue which uses interface{} keys and values,
// contextx uses generics to ensure compile-time type safety.
//
// Basic usage:
//
//	type userKey struct{}
//	ctx := contextx.WithValue(context.Background(), userKey{}, user)
//	user, ok := contextx.Value[*User](ctx, userKey{})
package contextx
