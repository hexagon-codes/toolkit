package slicex

import (
	"math/rand/v2"
)

// Shuffle 原地随机打乱切片
//
// 参数:
//   - slice: 要打乱的切片
//
// 示例:
//
//	nums := []int{1, 2, 3, 4, 5}
//	slicex.Shuffle(nums)  // nums 被原地打乱
func Shuffle[T any](slice []T) {
	rand.Shuffle(len(slice), func(i, j int) { // #nosec G404 -- 洗牌随机性不用于密钥、令牌或其他安全用途。
		slice[i], slice[j] = slice[j], slice[i]
	})
}

// ShuffleCopy 返回打乱后的新切片（不修改原切片）
//
// 参数:
//   - slice: 要打乱的切片
//
// 返回:
//   - []T: 打乱后的新切片
//
// 示例:
//
//	shuffled := slicex.ShuffleCopy([]int{1, 2, 3, 4, 5})
func ShuffleCopy[T any](slice []T) []T {
	if len(slice) == 0 {
		return nil
	}
	result := make([]T, len(slice))
	copy(result, slice)
	Shuffle(result)
	return result
}

// Sample 随机采样 n 个元素（不重复）
//
// 参数:
//   - slice: 要采样的切片
//   - n: 采样数量
//
// 返回:
//   - []T: 采样结果（如果 n >= len(slice)，返回打乱后的完整切片）
//
// 示例:
//
//	sample := slicex.Sample([]int{1, 2, 3, 4, 5}, 3)  // 随机 3 个元素
func Sample[T any](slice []T, n int) []T {
	if len(slice) == 0 || n <= 0 {
		return nil
	}
	if n >= len(slice) {
		return ShuffleCopy(slice)
	}

	// Fisher-Yates 部分洗牌
	result := make([]T, len(slice))
	copy(result, slice)
	for i := 0; i < n; i++ {
		j := i + rand.IntN(len(result)-i) // #nosec G404 -- 抽样随机性不用于安全选择。
		result[i], result[j] = result[j], result[i]
	}
	return result[:n]
}

// SampleOne 随机采样一个元素
//
// 参数:
//   - slice: 要采样的切片
//
// 返回:
//   - T: 随机选中的元素
//   - bool: 是否成功（空切片返回 false）
//
// 示例:
//
//	item, ok := slicex.SampleOne([]string{"a", "b", "c"})
func SampleOne[T any](slice []T) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	return slice[rand.IntN(len(slice))], true // #nosec G404 -- 抽样随机性不用于安全选择。
}

// SampleWithReplacement 有放回采样（可重复）
//
// 参数:
//   - slice: 要采样的切片
//   - n: 采样数量
//
// 返回:
//   - []T: 采样结果（元素可能重复）
//
// 示例:
//
//	sample := slicex.SampleWithReplacement([]int{1, 2, 3}, 5)
//	// 可能返回 [1, 3, 1, 2, 1]
func SampleWithReplacement[T any](slice []T, n int) []T {
	if len(slice) == 0 || n <= 0 {
		return nil
	}
	result := make([]T, n)
	for i := range result {
		result[i] = slice[rand.IntN(len(slice))] // #nosec G404 -- 抽样随机性不用于安全选择。
	}
	return result
}
