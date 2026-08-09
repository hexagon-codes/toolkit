package slicex

// Partition 将切片按条件分成两部分
//
// 参数:
//   - slice: 要分区的切片
//   - predicate: 分区函数，返回 true 的元素放入第一个切片
//
// 返回:
//   - []T: 满足条件的元素（空切片返回 nil）
//   - []T: 不满足条件的元素（空切片返回 nil）
//
// 注意: 空输入返回 (nil, nil)；非空输入返回的空结果也为 nil 以保持一致性
//
// 示例:
//
//	even, odd := slicex.Partition([]int{1, 2, 3, 4, 5}, func(n int) bool {
//	    return n%2 == 0
//	})
//	// even: [2, 4], odd: [1, 3, 5]
func Partition[T any](slice []T, predicate func(T) bool) (matched, unmatched []T) {
	if len(slice) == 0 {
		return nil, nil
	}
	for _, item := range slice {
		if predicate(item) {
			matched = append(matched, item)
		} else {
			unmatched = append(unmatched, item)
		}
	}
	return matched, unmatched
}

// PartitionBy 将切片按函数返回值分组
//
// 参数:
//   - slice: 要分区的切片
//   - fn: 分类函数
//
// 返回:
//   - map[K][]T: 按分类键分组的 map
//
// 示例:
//
//	groups := slicex.PartitionBy(users, func(u User) string {
//	    return u.Department
//	})
//	// map[string][]User{"IT": [...], "HR": [...]}
func PartitionBy[T any, K comparable](slice []T, fn func(T) K) map[K][]T {
	if len(slice) == 0 {
		return nil
	}
	result := make(map[K][]T)
	for _, item := range slice {
		key := fn(item)
		result[key] = append(result[key], item)
	}
	return result
}
