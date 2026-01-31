// Package mapx provides generic map operations.
//
// All functions use Go 1.18+ generics for type safety.
//
// Basic usage:
//
//	m := map[string]int{"a": 1, "b": 2}
//	keys := mapx.Keys(m)       // []string{"a", "b"}
//	values := mapx.Values(m)   // []int{1, 2}
//	clone := mapx.Clone(m)     // creates a shallow copy
//	merged := mapx.Merge(m1, m2)
package mapx
