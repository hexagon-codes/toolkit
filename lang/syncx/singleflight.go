package syncx

import "sync"

// call 表示一个正在执行或已完成的函数调用
type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

// Singleflight 防止重复执行相同的函数调用
// 当多个 goroutine 同时调用 Do 方法时，只有第一个会真正执行函数，
// 其他的会等待并共享第一个的结果
//
// 典型用途：防止缓存击穿
type Singleflight struct {
	mu sync.Mutex       // 保护 m
	m  map[string]*call // 懒加载
}

// NewSingleflight 创建一个新的 Singleflight 实例
//
// 示例:
//
//	sf := syncx.NewSingleflight()
func NewSingleflight() *Singleflight {
	return &Singleflight{}
}

// Do 执行并返回给定函数的结果，确保对于同一个 key，
// 同一时间只有一个执行在进行
//
// 参数:
//   - key: 用于标识这次调用的唯一键
//   - fn: 要执行的函数
//
// 返回:
//   - any: 函数返回的值
//   - error: 函数返回的错误
//
// 示例:
//
//	sf := syncx.NewSingleflight()
//	result, err := sf.Do("user:123", func() (any, error) {
//	    return db.GetUser(123)  // 多个并发请求只执行一次
//	})
func (g *Singleflight) Do(key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}

// Forget 删除指定 key 的记录，之后的调用将会重新执行函数
//
// 参数:
//   - key: 要删除的 key
//
// 示例:
//
//	sf.Forget("user:123")  // 清除缓存，下次调用会重新执行
func (g *Singleflight) Forget(key string) {
	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
}
