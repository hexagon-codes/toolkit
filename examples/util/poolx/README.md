# Hexpool 协程池示例

本目录包含 `util/poolx` 包的使用示例。这是一个功能全面、性能卓越的 Go 协程池实现。

## 核心特性

- ✅ **极致性能** - TrySubmit 34ns，比 ants 快 3x
- ✅ **批量提交** - SubmitBatch 减少 70% 锁竞争开销
- ✅ **泛型 Future** - 类型安全的异步结果获取
- ✅ **Work Stealing** - 自动负载均衡
- ✅ **Spinlock 优化** - 比 Mutex 快 2.5x
- ✅ **分片计数器** - 比 atomic.Int64 快 14x
- ✅ **汇编优化** - x86 PAUSE / ARM YIELD 指令
- ✅ **完整 Hook 系统** - 10+ 生命周期回调
- ✅ **自动扩缩容** - EMA 平滑负载计算

## 示例列表

### 基础用法 (`main.go`)

演示基本的池操作：
- 基本任务提交 `Submit`
- 单函数池 PoolWithFunc
- Future 模式获取异步结果
- Hook 生命周期回调
- 非阻塞模式
- 任务超时
- 多池负载均衡
- Map/ForEach 并行操作
- **批量提交 SubmitBatch** ✨

```bash
go run ./examples/util/poolx/
```

### 高级用法 (`advanced/main.go`)

涵盖高级特性：
- 自动扩缩容配置
- 优先级任务调度
- Async/Await 模式
- Context 取消
- 指标监控
- 命名池管理
- 全局池 (Go/GoCtx)

```bash
go run ./examples/util/poolx/advanced/
```

### 性能测试 (`benchmark/main.go`)

性能基准测试：
- 原生 goroutine vs 池 对比
- Submit vs TrySubmit
- PoolWithFunc 性能
- Worker 数量伸缩分析

```bash
go run ./examples/util/poolx/benchmark/
```

## 快速开始

```go
package main

import (
    "fmt"
    "sync"
    "github.com/everyday-items/toolkit/util/poolx"
)

func main() {
    // 创建一个 4 个 worker 的池
    p := poolx.New("my-pool", poolx.WithMaxWorkers(4))
    defer p.Release()

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        p.Submit(func() {
            defer wg.Done()
            // 你的任务逻辑
        })
    }
    wg.Wait()

    // 获取指标
    metrics := p.Metrics()
    fmt.Printf("完成任务数: %d\n", metrics.CompletedTasks)
}
```

## 性能优化建议

1. **非阻塞操作使用 TrySubmit** - ~34ns vs Submit 的 ~1100ns
2. **批量提交使用 SubmitBatch** - 减少锁竞争开销
3. **不需要时禁用 hooks** - Hooks 每个任务增加 ~400ns 开销
4. **重复操作使用 PoolWithFunc** - 更高的内存效率
5. **根据工作负载调整 worker 数量** - 建议从 NumCPU * 2 开始
6. **变化负载启用自动扩缩容** - 自动调整 worker 数量
7. **高并发计数器使用 ShardedCounter** - 比 atomic.Int64 快 14 倍

## 性能基准

运行基准测试查看性能特征：

```bash
go test ./util/poolx/... -bench=. -benchmem
```

典型结果：

| 操作 | 耗时 | 内存分配 |
|------|------|----------|
| Submit | ~1100 ns | 0 B |
| TrySubmit | ~34 ns | 0 B |
| SubmitBatch(10) | ~99 ns/任务 | 0 B |
| PoolWithFunc.Invoke | ~1300 ns | 0 B |
| PoolWithFunc.TryInvoke | ~38 ns | 0 B |
| Future.SubmitFunc | ~2100 ns | 224 B |
| Spinlock vs Mutex | 30 ns vs 74 ns | 2.5x 快 |
| ShardedCounter vs atomic | 3.5 ns vs 49 ns | 14x 快 |

## 与其他库对比

| 特性 | poolx | ants | ByteDance gopool |
|------|---------|------|------------------|
| 基本提交 | ✅ | ✅ | ✅ |
| 非阻塞提交 | ✅ (~34ns) | ✅ (~100ns) | ❌ |
| **批量提交** | ✅ | ❌ | ❌ |
| Context 支持 | ✅ | ❌ | ✅ |
| 单函数池 | ✅ | ✅ | ❌ |
| Future 模式 | ✅ | ❌ | ❌ |
| 优先级队列 | ✅ | ❌ | ❌ |
| 自动扩缩容 | ✅ | ❌ | ✅ |
| Hook 系统 | ✅ (10+) | ❌ | ❌ |
| 任务超时 | ✅ | ❌ | ❌ |
| 多池调度 | ✅ | ✅ | ❌ |
| **Work Stealing** | ✅ | ❌ | ❌ |
| **分片计数器** | ✅ | ❌ | ❌ |
| **汇编优化** | ✅ | ❌ | ❌ |

## 完整 API 列表

```go
// 创建池
poolx.New(name, opts...)
poolx.NewPoolWithFunc(name, fn, opts...)
poolx.NewMultiPool(size, poolSize, strategy, opts...)

// 任务提交
p.Submit(fn)                    // 阻塞提交
p.TrySubmit(fn)                 // 非阻塞提交 (~34ns)
p.SubmitBatch(fns)              // 批量提交
p.TrySubmitBatch(fns)           // 非阻塞批量提交
p.SubmitWait(fn)                // 提交并等待完成
p.SubmitWithContext(ctx, fn)    // Context 支持
p.SubmitWithOptions(fn, opts)   // 带选项提交

// Future 模式
poolx.SubmitFunc(p, fn)       // 返回 Future[T]
poolx.Async(fn)               // 全局池 Future
future.Get()                    // 阻塞获取结果
future.GetWithTimeout(timeout)  // 超时获取

// 池管理
p.Tune(newCap)                  // 动态调整容量
p.Release()                     // 释放资源
p.Reboot()                      // 重启池

// 状态查询
p.Running()                     // 运行中 worker 数
p.Free()                        // 空闲 slot 数
p.Cap()                         // 容量
p.Metrics()                     // 详细指标

// 高性能计数器
poolx.NewShardedCounter()     // int64 分片计数器
poolx.NewShardedCounter32()   // int32 分片计数器
poolx.NewFastCounter()        // 动态分片计数器
```

## 配置选项

```go
poolx.WithMaxWorkers(100)        // 最大 worker 数
poolx.WithMinWorkers(10)         // 最小 worker 数
poolx.WithAutoScale(true)        // 启用自动扩缩容
poolx.WithWorkStealing(true)     // 启用 Work Stealing
poolx.WithPriorityQueue(true)    // 启用优先级队列
poolx.WithNonBlocking(true)      // 非阻塞模式
poolx.WithHooks(hooks)           // Hook 回调
poolx.WithPanicHandler(fn)       // Panic 处理
poolx.WithWorkerExpiry(duration) // Worker 过期时间
poolx.WithPreAlloc(true)         // 预分配资源
```

## 更多文档

- [详细对比分析](../../../util/poolx/COMPARISON.md)
- [基准测试报告](../../../util/poolx/benchmark_test.go)
