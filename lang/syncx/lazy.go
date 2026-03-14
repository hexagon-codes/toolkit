package syncx

import (
	"sync"
	"sync/atomic"
)

// Lazy 延迟初始化容器
//
// 值在第一次访问时才会被初始化
type Lazy[T any] struct {
	once        sync.Once
	init        func() T
	value       T
	initialized atomic.Bool // 追踪初始化状态
}

// NewLazy 创建一个延迟初始化容器
//
// 参数:
//   - init: 初始化函数
//
// 返回:
//   - *Lazy[T]: 延迟初始化容器
//
// 示例:
//
//	config := syncx.NewLazy(func() *Config {
//	    return loadConfigFromFile()
//	})
//	// ... 其他代码 ...
//	cfg := config.Get()  // 此时才加载配置
func NewLazy[T any](init func() T) *Lazy[T] {
	return &Lazy[T]{init: init}
}

// Get 获取值，如果未初始化则执行初始化
//
// 返回:
//   - T: 值
//
// 示例:
//
//	value := lazy.Get()
func (l *Lazy[T]) Get() T {
	l.once.Do(func() {
		if l.init != nil {
			l.value = l.init()
		}
		l.initialized.Store(true)
	})
	return l.value
}

// IsInitialized 检查值是否已初始化
//
// 返回:
//   - bool: 如果已初始化返回 true
//
// 注意: 此方法不会触发初始化，仅检查当前状态
func (l *Lazy[T]) IsInitialized() bool {
	return l.initialized.Load()
}

// Reset 重置状态（非线程安全）
//
// 警告: 此方法不是线程安全的，严禁在存在并发 Get 调用时使用。
// 如果在并发环境中调用 Reset，可能导致数据竞争和未定义行为。
// 仅应在测试或确定无并发访问的初始化/清理阶段使用。
func (l *Lazy[T]) Reset() {
	l.once = sync.Once{}
	var zero T
	l.value = zero
	l.initialized.Store(false)
}

// LazyErr 带错误的延迟初始化容器
type LazyErr[T any] struct {
	once  sync.Once
	init  func() (T, error)
	value T
	err   error
}

// NewLazyErr 创建一个带错误处理的延迟初始化容器
//
// 参数:
//   - init: 初始化函数
//
// 返回:
//   - *LazyErr[T]: 延迟初始化容器
//
// 示例:
//
//	db := syncx.NewLazyErr(func() (*sql.DB, error) {
//	    return sql.Open("mysql", dsn)
//	})
//	conn, err := db.Get()
func NewLazyErr[T any](init func() (T, error)) *LazyErr[T] {
	return &LazyErr[T]{init: init}
}

// Get 获取值，如果未初始化则执行初始化
//
// 返回:
//   - T: 值
//   - error: 初始化错误
//
// 示例:
//
//	value, err := lazy.Get()
func (l *LazyErr[T]) Get() (T, error) {
	l.once.Do(func() {
		if l.init != nil {
			l.value, l.err = l.init()
		}
	})
	return l.value, l.err
}

// MustGet 获取值，如果有错误则 panic
//
// 返回:
//   - T: 值
func (l *LazyErr[T]) MustGet() T {
	value, err := l.Get()
	if err != nil {
		panic(err)
	}
	return value
}

// Err 返回初始化错误（如果有）
//
// 返回:
//   - error: 错误
func (l *LazyErr[T]) Err() error {
	return l.err
}

// LazyValue 便捷函数，创建返回值的懒加载函数
//
// 参数:
//   - init: 初始化函数
//
// 返回:
//   - func() T: 返回值的函数
//
// 示例:
//
//	getConfig := syncx.LazyValue(loadConfig)
//	config := getConfig()  // 首次调用时初始化
func LazyValue[T any](init func() T) func() T {
	lazy := NewLazy(init)
	return lazy.Get
}

// LazyValueErr 便捷函数，创建返回值和错误的懒加载函数
//
// 参数:
//   - init: 初始化函数
//
// 返回:
//   - func() (T, error): 返回值和错误的函数
//
// 示例:
//
//	getDB := syncx.LazyValueErr(connectDB)
//	db, err := getDB()  // 首次调用时连接
func LazyValueErr[T any](init func() (T, error)) func() (T, error) {
	lazy := NewLazyErr(init)
	return lazy.Get
}
