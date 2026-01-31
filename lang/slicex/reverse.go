package slicex

// Reverse 反转切片（创建新切片）
//
// 参数:
//   - slice: 要反转的切片
//
// 返回:
//   - []T: 反转后的新切片
//
// 示例:
//
//	reversed := slicex.Reverse([]int{1, 2, 3})  // [3, 2, 1]
func Reverse[T any](slice []T) []T {
	result := make([]T, len(slice))
	for i, item := range slice {
		result[len(slice)-1-i] = item
	}
	return result
}

// ReverseInPlace 原地反转切片（修改原切片）
//
// 参数:
//   - slice: 要反转的切片
//
// 示例:
//
//	nums := []int{1, 2, 3}
//	slicex.ReverseInPlace(nums)  // nums 变为 [3, 2, 1]
func ReverseInPlace[T any](slice []T) {
	for i, j := 0, len(slice)-1; i < j; i, j = i+1, j-1 {
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// Chunk 将切片分块
//
// 参数:
//   - slice: 要分块的切片
//   - size: 每块的大小
//
// 返回:
//   - [][]T: 分块后的二维切片
//
// 警告: 返回的子切片与原切片共享底层数组
// 修改子切片会影响原切片，反之亦然
// 如需独立副本，请使用 ChunkCopy
//
// 示例:
//
//	chunks := slicex.Chunk([]int{1, 2, 3, 4, 5}, 2)  // [[1, 2], [3, 4], [5]]
func Chunk[T any](slice []T, size int) [][]T {
	if size <= 0 {
		return [][]T{}
	}

	chunks := make([][]T, 0, (len(slice)+size-1)/size)
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// ChunkCopy 将切片分块（返回独立副本）
//
// 与 Chunk 不同，ChunkCopy 返回的子切片是独立的副本
// 修改子切片不会影响原切片
//
// 参数:
//   - slice: 要分块的切片
//   - size: 每块的大小
//
// 返回:
//   - [][]T: 分块后的二维切片（每个子切片都是独立副本）
//
// 示例:
//
//	chunks := slicex.ChunkCopy([]int{1, 2, 3, 4, 5}, 2)  // [[1, 2], [3, 4], [5]]
func ChunkCopy[T any](slice []T, size int) [][]T {
	if size <= 0 {
		return [][]T{}
	}

	chunks := make([][]T, 0, (len(slice)+size-1)/size)
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		// 创建独立副本
		chunk := make([]T, end-i)
		copy(chunk, slice[i:end])
		chunks = append(chunks, chunk)
	}
	return chunks
}

// Take 取前 n 个元素
//
// 参数:
//   - slice: 原切片
//   - n: 要取的元素数量
//
// 返回:
//   - []T: 前 n 个元素的新切片
//
// 示例:
//
//	first3 := slicex.Take([]int{1, 2, 3, 4, 5}, 3)  // [1, 2, 3]
func Take[T any](slice []T, n int) []T {
	if n <= 0 {
		return []T{}
	}
	if n >= len(slice) {
		result := make([]T, len(slice))
		copy(result, slice)
		return result
	}
	result := make([]T, n)
	copy(result, slice[:n])
	return result
}

// Drop 跳过前 n 个元素
//
// 参数:
//   - slice: 原切片
//   - n: 要跳过的元素数量
//
// 返回:
//   - []T: 跳过后的新切片
//
// 示例:
//
//	rest := slicex.Drop([]int{1, 2, 3, 4, 5}, 2)  // [3, 4, 5]
func Drop[T any](slice []T, n int) []T {
	if n <= 0 {
		result := make([]T, len(slice))
		copy(result, slice)
		return result
	}
	if n >= len(slice) {
		return []T{}
	}
	result := make([]T, len(slice)-n)
	copy(result, slice[n:])
	return result
}
