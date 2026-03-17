[中文](README.md) | English

# Hexpool Goroutine Pool Examples

This directory contains usage examples for the `util/poolx` package — a comprehensive, high-performance Go goroutine pool implementation.

## Core Features

- ✅ **Maximum performance** - TrySubmit 34ns, 3x faster than ants
- ✅ **Batch submit** - SubmitBatch reduces 70% lock contention overhead
- ✅ **Generic Future** - type-safe async result retrieval
- ✅ **Work Stealing** - automatic load balancing
- ✅ **Spinlock optimization** - 2.5x faster than Mutex
- ✅ **Sharded counter** - 14x faster than atomic.Int64
- ✅ **Assembly optimization** - x86 PAUSE / ARM YIELD instructions
- ✅ **Complete Hook system** - 10+ lifecycle callbacks
- ✅ **Auto scaling** - EMA smooth load calculation

## Example List

### Basic Usage (`main.go`)

Demonstrates basic pool operations:
- Basic task submit `Submit`
- Single-function pool PoolWithFunc
- Future pattern for async result retrieval
- Hook lifecycle callbacks
- Non-blocking mode
- Task timeout
- Multi-pool load balancing
- Map/ForEach parallel operations
- **Batch submit SubmitBatch** ✨

```bash
go run ./examples/util/poolx/
```

### Advanced Usage (`advanced/main.go`)

Covers advanced features:
- Auto scaling configuration
- Priority task scheduling
- Async/Await pattern
- Context cancellation
- Metrics monitoring
- Named pool management
- Global pool (Go/GoCtx)

```bash
go run ./examples/util/poolx/advanced/
```

### Performance Testing (`benchmark/main.go`)

Performance benchmarks:
- Native goroutine vs pool comparison
- Submit vs TrySubmit
- PoolWithFunc performance
- Worker count scaling analysis

```bash
go run ./examples/util/poolx/benchmark/
```

## Quick Start

```go
package main

import (
    "fmt"
    "sync"
    "github.com/everyday-items/toolkit/util/poolx"
)

func main() {
    // Create a pool with 4 workers
    p := poolx.New("my-pool", poolx.WithMaxWorkers(4))
    defer p.Release()

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        p.Submit(func() {
            defer wg.Done()
            // your task logic
        })
    }
    wg.Wait()

    // Get metrics
    metrics := p.Metrics()
    fmt.Printf("Completed tasks: %d\n", metrics.CompletedTasks)
}
```

## Performance Optimization Tips

1. **Use TrySubmit for non-blocking operations** - ~34ns vs Submit's ~1100ns
2. **Use SubmitBatch for bulk tasks** - reduces lock contention overhead
3. **Disable hooks when not needed** - Hooks add ~400ns overhead per task
4. **Use PoolWithFunc for repeated operations** - higher memory efficiency
5. **Tune worker count to workload** - start with NumCPU * 2 as baseline
6. **Enable auto scaling for variable load** - automatically adjusts worker count
7. **Use ShardedCounter for high-concurrency counting** - 14x faster than atomic.Int64

## Performance Benchmarks

Run benchmarks to see performance characteristics:

```bash
go test ./util/poolx/... -bench=. -benchmem
```

Typical results:

| Operation | Latency | Memory Allocation |
|-----------|---------|-------------------|
| Submit | ~1100 ns | 0 B |
| TrySubmit | ~34 ns | 0 B |
| SubmitBatch(10) | ~99 ns/task | 0 B |
| PoolWithFunc.Invoke | ~1300 ns | 0 B |
| PoolWithFunc.TryInvoke | ~38 ns | 0 B |
| Future.SubmitFunc | ~2100 ns | 224 B |
| Spinlock vs Mutex | 30 ns vs 74 ns | 2.5x faster |
| ShardedCounter vs atomic | 3.5 ns vs 49 ns | 14x faster |

## Comparison with Other Libraries

| Feature | poolx | ants | ByteDance gopool |
|---------|---------|------|------------------|
| Basic submit | ✅ | ✅ | ✅ |
| Non-blocking submit | ✅ (~34ns) | ✅ (~100ns) | ❌ |
| **Batch submit** | ✅ | ❌ | ❌ |
| Context support | ✅ | ❌ | ✅ |
| Single-func pool | ✅ | ✅ | ❌ |
| Future pattern | ✅ | ❌ | ❌ |
| Priority queue | ✅ | ❌ | ❌ |
| Auto scaling | ✅ | ❌ | ✅ |
| Hook system | ✅ (10+) | ❌ | ❌ |
| Task timeout | ✅ | ❌ | ❌ |
| Multi-pool scheduling | ✅ | ✅ | ❌ |
| **Work Stealing** | ✅ | ❌ | ❌ |
| **Sharded counter** | ✅ | ❌ | ❌ |
| **Assembly optimization** | ✅ | ❌ | ❌ |

## Full API Reference

```go
// Create pool
poolx.New(name, opts...)
poolx.NewPoolWithFunc(name, fn, opts...)
poolx.NewMultiPool(size, poolSize, strategy, opts...)

// Task submit
p.Submit(fn)                    // blocking submit
p.TrySubmit(fn)                 // non-blocking submit (~34ns)
p.SubmitBatch(fns)              // batch submit
p.TrySubmitBatch(fns)           // non-blocking batch submit
p.SubmitWait(fn)                // submit and wait for completion
p.SubmitWithContext(ctx, fn)    // Context support
p.SubmitWithOptions(fn, opts)   // submit with options

// Future pattern
poolx.SubmitFunc(p, fn)       // returns Future[T]
poolx.Async(fn)               // global pool Future
future.Get()                    // blocking result retrieval
future.GetWithTimeout(timeout)  // timeout result retrieval

// Pool management
p.Tune(newCap)                  // dynamic capacity adjustment
p.Release()                     // release resources
p.Reboot()                      // restart pool

// Status query
p.Running()                     // running worker count
p.Free()                        // free slot count
p.Cap()                         // capacity
p.Metrics()                     // detailed metrics

// High-performance counter
poolx.NewShardedCounter()     // int64 sharded counter
poolx.NewShardedCounter32()   // int32 sharded counter
poolx.NewFastCounter()        // dynamic sharded counter
```

## Configuration Options

```go
poolx.WithMaxWorkers(100)        // max worker count
poolx.WithMinWorkers(10)         // min worker count
poolx.WithAutoScale(true)        // enable auto scaling
poolx.WithWorkStealing(true)     // enable Work Stealing
poolx.WithPriorityQueue(true)    // enable priority queue
poolx.WithNonBlocking(true)      // non-blocking mode
poolx.WithHooks(hooks)           // hook callbacks
poolx.WithPanicHandler(fn)       // panic handler
poolx.WithWorkerExpiry(duration) // worker expiry duration
poolx.WithPreAlloc(true)         // pre-allocate resources
```

## More Documentation

- [Detailed Comparison Analysis](../../../util/poolx/COMPARISON.md)
- [Benchmark Reports](../../../util/poolx/benchmark_test.go)
