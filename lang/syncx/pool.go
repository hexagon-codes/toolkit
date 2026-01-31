package syncx

import "sync"

// Pool 是对 sync.Pool 的简单封装，提供更友好的 API
//
// sync.Pool 是一个临时对象池，用于减少 GC 压力
// 注意：Pool 中的对象可能随时被 GC 回收，不要用于存储持久化数据
type Pool struct {
	pool sync.Pool
}

// NewPool 创建一个新的对象池
//
// 参数:
//   - newFunc: 创建新对象的函数，当池为空时会调用
//
// 返回:
//   - *Pool: 对象池实例
//
// 示例:
//
//	pool := syncx.NewPool(func() any {
//	    return &bytes.Buffer{}  // 创建 Buffer 池
//	})
func NewPool(newFunc func() any) *Pool {
	return &Pool{
		pool: sync.Pool{
			New: newFunc,
		},
	}
}

// Get 从池中获取一个对象
// 如果池为空，会调用 newFunc 创建新对象
//
// 返回:
//   - any: 从池中获取的对象
//
// 示例:
//
//	buf := pool.Get().(*bytes.Buffer)
//	defer pool.Put(buf)
//	buf.WriteString("hello")
func (p *Pool) Get() any {
	return p.pool.Get()
}

// Put 将对象放回池中以供后续复用
// 注意：放回池中的对象应该被重置到初始状态
//
// 参数:
//   - x: 要放回的对象
//
// 示例:
//
//	buf := pool.Get().(*bytes.Buffer)
//	buf.WriteString("hello")
//	buf.Reset()  // 重置状态
//	pool.Put(buf)
func (p *Pool) Put(x any) {
	p.pool.Put(x)
}

// TypedPool 是类型安全的对象池（使用泛型）
type TypedPool[T any] struct {
	pool *Pool
}

// NewTypedPool 创建一个类型安全的对象池
//
// 参数:
//   - newFunc: 创建新对象的函数
//
// 返回:
//   - *TypedPool[T]: 类型安全的对象池
//
// 示例:
//
//	pool := syncx.NewTypedPool(func() *bytes.Buffer {
//	    return &bytes.Buffer{}
//	})
//	buf := pool.Get()  // 类型安全，无需类型断言
//	defer pool.Put(buf)
func NewTypedPool[T any](newFunc func() T) *TypedPool[T] {
	return &TypedPool[T]{
		pool: NewPool(func() any {
			return newFunc()
		}),
	}
}

// Get 从池中获取一个对象（类型安全）
//
// 返回:
//   - T: 从池中获取的对象
func (p *TypedPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put 将对象放回池中
//
// 参数:
//   - x: 要放回的对象
func (p *TypedPool[T]) Put(x T) {
	p.pool.Put(x)
}
