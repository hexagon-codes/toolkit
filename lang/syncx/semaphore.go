package syncx

import (
	"context"
	"errors"
	"sync"
)

// ErrNilSemaphoreContext 表示信号量操作收到了空上下文。
var ErrNilSemaphoreContext = errors.New("syncx: semaphore context must not be nil")

// Semaphore 信号量，用于限制并发访问数量
//
// 基于 channel 实现，提供简单的并发控制
type Semaphore struct {
	once sync.Once
	sem  chan struct{}
}

// channel 延迟建立零值信号量的单个许可。
func (s *Semaphore) channel() chan struct{} {
	s.once.Do(func() {
		if s.sem == nil {
			s.sem = make(chan struct{}, 1)
		}
	})
	return s.sem
}

// NewSemaphore 创建一个新的信号量
//
// 参数:
//   - n: 最大并发数（必须大于 0）
//
// 返回:
//   - *Semaphore: 信号量实例
//
// 示例:
//
//	sem := syncx.NewSemaphore(3)  // 最多 3 个并发
//	for i := 0; i < 10; i++ {
//	    go func() {
//	        sem.Acquire()
//	        defer sem.Release()
//	        // 执行工作...
//	    }()
//	}
func NewSemaphore(n int) *Semaphore {
	if n <= 0 {
		n = 1
	}
	return &Semaphore{
		sem: make(chan struct{}, n),
	}
}

// Acquire 获取信号量（阻塞直到获取成功）
//
// 示例:
//
//	sem.Acquire()
//	defer sem.Release()
func (s *Semaphore) Acquire() {
	s.channel() <- struct{}{}
}

// TryAcquire 尝试获取信号量（非阻塞）
//
// 返回:
//   - bool: 如果获取成功返回 true，否则返回 false
//
// 示例:
//
//	if sem.TryAcquire() {
//	    defer sem.Release()
//	    // 执行工作...
//	} else {
//	    // 信号量已满
//	}
func (s *Semaphore) TryAcquire() bool {
	sem := s.channel()
	select {
	case sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// AcquireContext 获取信号量，支持取消
//
// 参数:
//   - ctx: context，用于取消等待
//
// 返回:
//   - error: 如果 context 被取消返回错误
//
// 示例:
//
//	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
//	defer cancel()
//	if err := sem.AcquireContext(ctx); err != nil {
//	    // 超时或取消
//	}
//	defer sem.Release()
func (s *Semaphore) AcquireContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilSemaphoreContext
	}
	sem := s.channel()
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 释放信号量
//
// 注意: 必须与 Acquire/TryAcquire/AcquireContext 配对使用
// 如果释放次数超过获取次数，会 panic
//
// 示例:
//
//	sem.Acquire()
//	defer sem.Release()
func (s *Semaphore) Release() {
	sem := s.channel()
	select {
	case <-sem:
	default:
		panic("syncx: semaphore release without acquire")
	}
}

// TryRelease 尝试释放信号量（非阻塞，不 panic）
//
// 返回:
//   - bool: 如果释放成功返回 true，否则返回 false
//
// 示例:
//
//	if !sem.TryRelease() {
//	    // 没有信号量可释放
//	}
func (s *Semaphore) TryRelease() bool {
	sem := s.channel()
	select {
	case <-sem:
		return true
	default:
		return false
	}
}

// Available 返回当前可用的信号量数量
//
// 返回:
//   - int: 可用信号量数量
//
// 注意: 返回值可能在获取后立即过期（竞态条件）
func (s *Semaphore) Available() int {
	sem := s.channel()
	return cap(sem) - len(sem)
}

// Capacity 返回信号量容量
//
// 返回:
//   - int: 信号量总容量
func (s *Semaphore) Capacity() int {
	return cap(s.channel())
}

// Held 返回当前持有的信号量数量
//
// 返回:
//   - int: 已被获取的信号量数量
func (s *Semaphore) Held() int {
	return len(s.channel())
}
