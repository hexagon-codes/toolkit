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
