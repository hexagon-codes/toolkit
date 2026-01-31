package slicex

// Unique 对切片去重，保持原有顺序
//
// 参数:
//   - slice: 要去重的切片
//
// 返回:
//   - []T: 去重后的新切片
//
// 示例:
//
//	result := slicex.Unique([]int{1, 2, 2, 3, 3, 4})  // [1, 2, 3, 4]
//	result := slicex.Unique([]string{"a", "b", "a", "c"})  // ["a", "b", "c"]
func Unique[T comparable](slice []T) []T {
	if len(slice) == 0 {
		return []T{}
	}

	seen := make(map[T]struct{}, len(slice))
	result := make([]T, 0, len(slice))

	for _, item := range slice {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	return result
}

// UniqueFunc 使用自定义函数对切片去重
//
// 参数:
//   - slice: 要去重的切片
//   - keyFn: 提取用于比较的 key 的函数
//
// 返回:
//   - []T: 去重后的新切片
//
// 示例:
//
//	type User struct { ID int; Name string }
//	users := []User{{1, "Alice"}, {2, "Bob"}, {1, "Alice2"}}
//	result := slicex.UniqueFunc(users, func(u User) int {
//	    return u.ID  // 按 ID 去重
//	})  // [{1, "Alice"}, {2, "Bob"}]
func UniqueFunc[T any, K comparable](slice []T, keyFn func(T) K) []T {
	if len(slice) == 0 {
		return []T{}
	}

	seen := make(map[K]struct{}, len(slice))
	result := make([]T, 0, len(slice))

	for _, item := range slice {
		key := keyFn(item)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, item)
		}
	}

	return result
}
