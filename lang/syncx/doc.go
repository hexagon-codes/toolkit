// Package syncx 提供并发同步的工具函数
//
// 这个包是对 Go 标准库 sync 包的增强，提供常用的并发模式实现。
//
// # 主要功能
//
// 防缓存击穿:
//   - Singleflight: 防止重复执行相同的函数调用
//
// 对象池:
//   - Pool: sync.Pool 的简单封装
//   - TypedPool: 类型安全的对象池（泛型）
//
// # 使用示例
//
//	import "github.com/hexagon-codes/toolkit/lang/syncx"
//
//	// Singleflight - 防缓存击穿
//	sf := syncx.NewSingleflight()
//	result, err := sf.Do("user:123", func() (any, error) {
//	    return db.GetUser(123)  // 并发请求只执行一次
//	})
//
//	// Pool - 对象复用
//	pool := syncx.NewPool(func() any {
//	    return &bytes.Buffer{}
//	})
//	buf := pool.Get().(*bytes.Buffer)
//	defer pool.Put(buf)
//
//	// TypedPool - 类型安全的对象池
//	pool := syncx.NewTypedPool(func() *bytes.Buffer {
//	    return &bytes.Buffer{}
//	})
//	buf := pool.Get()  // 无需类型断言
//	defer pool.Put(buf)
//
// # 典型应用场景
//
// 1. Singleflight 用于:
//   - 防止缓存击穿（多个请求同时查询同一个不存在的 key）
//   - 减少数据库压力（合并重复查询）
//   - API 请求去重
//
// 2. Pool 用于:
//   - 高频创建的临时对象（bytes.Buffer, strings.Builder）
//   - 减少 GC 压力
//   - 提升性能
//
// # 设计原则
//
// 1. 零外部依赖：只使用 Go 标准库
// 2. 简单易用：API 简洁明了
// 3. 类型安全：提供泛型版本
//
// # 注意事项
//
// - Singleflight: 适用于读多写少的场景
// - Pool: 对象可能被 GC 回收，不要存储重要数据
// - 所有类型都是并发安全的
//
// --- English ---
//
// Package syncx provides utility functions for concurrent synchronization.
//
// This package enhances the Go standard library sync package with
// implementations of common concurrency patterns.
//
// # Main Features
//
// Cache stampede prevention:
//   - Singleflight: prevents duplicate execution of the same function call
//
// Object pools:
//   - Pool: a simple wrapper around sync.Pool
//   - TypedPool: a type-safe object pool (generics)
//
// # Usage Examples
//
//	import "github.com/hexagon-codes/toolkit/lang/syncx"
//
//	// Singleflight - prevent cache stampede
//	sf := syncx.NewSingleflight()
//	result, err := sf.Do("user:123", func() (any, error) {
//	    return db.GetUser(123)  // concurrent requests execute only once
//	})
//
//	// Pool - object reuse
//	pool := syncx.NewPool(func() any {
//	    return &bytes.Buffer{}
//	})
//	buf := pool.Get().(*bytes.Buffer)
//	defer pool.Put(buf)
//
//	// TypedPool - type-safe object pool
//	pool := syncx.NewTypedPool(func() *bytes.Buffer {
//	    return &bytes.Buffer{}
//	})
//	buf := pool.Get()  // no type assertion needed
//	defer pool.Put(buf)
//
// # Typical Use Cases
//
// 1. Singleflight is used for:
//   - Preventing cache stampede (multiple requests querying the same missing key)
//   - Reducing database load (merging duplicate queries)
//   - Deduplicating API requests
//
// 2. Pool is used for:
//   - Frequently created temporary objects (bytes.Buffer, strings.Builder)
//   - Reducing GC pressure
//   - Improving performance
//
// # Design Principles
//
// 1. Zero external dependencies: only uses Go standard library
// 2. Simple and easy to use: clean and straightforward API
// 3. Type-safe: provides generic versions
//
// # Notes
//
// - Singleflight: best suited for read-heavy, write-light scenarios
// - Pool: objects may be garbage collected; do not store important data in them
// - All types are concurrency-safe
package syncx
