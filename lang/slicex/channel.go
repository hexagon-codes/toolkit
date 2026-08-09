package slicex

// ToChannel 将切片元素发送到 channel
//
// 参数:
//   - slice: 要发送的切片
//
// 返回:
//   - <-chan T: 只读 channel
//
// 注意: 返回的 channel 会在所有元素发送完后关闭
//
// 示例:
//
//	ch := slicex.ToChannel([]int{1, 2, 3})
//	for v := range ch {
//	    fmt.Println(v)
//	}
func ToChannel[T any](slice []T) <-chan T {
	ch := make(chan T, len(slice))
	go func() {
		defer close(ch)
		for _, v := range slice {
			ch <- v
		}
	}()
	return ch
}

// ToChannelBuffered 将切片元素发送到带缓冲的 channel
//
// 参数:
//   - slice: 要发送的切片
//   - bufferSize: channel 缓冲区大小
//
// 返回:
//   - <-chan T: 只读 channel
//
// 示例:
//
//	ch := slicex.ToChannelBuffered([]int{1, 2, 3}, 10)
func ToChannelBuffered[T any](slice []T, bufferSize int) <-chan T {
	if bufferSize <= 0 {
		bufferSize = 1
	}
	ch := make(chan T, bufferSize)
	go func() {
		defer close(ch)
		for _, v := range slice {
			ch <- v
		}
	}()
	return ch
}

// FromChannel 从 channel 收集元素到切片
//
// 参数:
//   - ch: 要收集的 channel
//
// 返回:
//   - []T: 收集到的切片
//
// 注意: 会阻塞直到 channel 关闭
//
// 示例:
//
//	ch := make(chan int, 3)
//	ch <- 1; ch <- 2; ch <- 3
//	close(ch)
//	slice := slicex.FromChannel(ch)  // [1, 2, 3]
func FromChannel[T any](ch <-chan T) []T {
	var result []T
	for v := range ch {
		result = append(result, v)
	}
	return result
}

// FromChannelN 从 channel 收集最多 n 个元素
//
// 参数:
//   - ch: 要收集的 channel
//   - n: 最大收集数量
//
// 返回:
//   - []T: 收集到的切片
//
// 示例:
//
//	slice := slicex.FromChannelN(ch, 10)  // 最多收集 10 个
func FromChannelN[T any](ch <-chan T, n int) []T {
	if n <= 0 {
		return nil
	}
	result := make([]T, 0, n)
	for i := 0; i < n; i++ {
		v, ok := <-ch
		if !ok {
			break
		}
		result = append(result, v)
	}
	return result
}

// Drain 消费 channel 中的所有元素（丢弃）
//
// 参数:
//   - ch: 要消费的 channel
//
// 示例:
//
//	slicex.Drain(ch)  // 丢弃所有元素
func Drain[T any](ch <-chan T) {
	for item := range ch {
		_ = item
	}
}
