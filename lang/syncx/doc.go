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
//	import "github.com/everyday-items/toolkit/lang/syncx"
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
package syncx
