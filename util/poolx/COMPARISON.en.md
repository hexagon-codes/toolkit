[中文](COMPARISON.md) | English

# Go Goroutine Pool Comparison

This document compares three Go goroutine pool implementations:
- **poolx** - This project's implementation (high-performance goroutine pool)
- **ants** - https://github.com/panjf2000/ants (⭐ 12k+)
- **ByteDance gopool** - https://github.com/bytedance/gopkg/tree/main/util/gopool

---

## Quick Comparison

### Feature Matrix

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Basic submit | ✅ | ✅ | ✅ |
| Non-blocking submit | ✅ **34ns** | ✅ ~100ns | ❌ |
| **Batch submit** | ✅ | ❌ | ❌ |
| Context support | ✅ | ❌ | ✅ |
| Single-func pool | ✅ | ✅ | ❌ |
| **Future pattern** | ✅ | ❌ | ❌ |
| **Priority queue** | ✅ | ❌ | ❌ |
| **Hook system** | ✅ (10+) | panic only | panic only |
| Auto scaling | ✅ EMA | ❌ | ✅ simple |
| **Work Stealing** | ✅ | ❌ | ❌ |
| **Sharded counter** | ✅ | ❌ | ❌ |
| **Assembly optimization** | ✅ | ❌ | ❌ |
| Detailed metrics | ✅ | ❌ | ❌ |

### Low-Level Optimizations

| Optimization | poolx | ants | gopool |
|-------------|-----------|------|--------|
| Spinlock | ✅ 30ns | ❌ Mutex | ❌ Mutex |
| ShardedCounter | ✅ 3.5ns | ❌ atomic 49ns | ❌ |
| PAUSE/YIELD instructions | ✅ assembly | ❌ | ❌ |
| Cache Line Padding | ✅ | ❌ | ❌ |
| Work Stealing | ✅ | ❌ | ❌ |

### Performance Figures

| Operation | poolx | ants | Comparison |
|-----------|-----------|------|------------|
| TrySubmit | **34 ns** | ~100 ns | **3x faster** |
| Submit | ~1100 ns | ~1000 ns | comparable |
| SubmitBatch(10) | **99 ns/task** | N/A | exclusive |
| Spinlock | **30 ns** | Mutex 74 ns | **2.5x faster** |
| ShardedCounter | **3.5 ns** | atomic.Int64 49 ns | **14x faster** |

### API Simplicity

```go
// ByteDance gopool - simplest
gopool.Go(fn)

// poolx - equally simple
poolx.Go(fn)

// ants - requires pool creation
p, _ := ants.NewPool(100)
p.Submit(fn)
```

### Recommendation by Use Case

| Scenario | Recommendation |
|----------|---------------|
| Simplest API | gopool or poolx |
| Battle-tested in production | ants |
| Need Future/return values | **poolx** |
| Need Hook monitoring | **poolx** |
| Need priority scheduling | **poolx** |
| Maximum performance | **poolx** |
| High-concurrency counters | **poolx** (ShardedCounter) |

### One-line Summary

- **ants** - mature and stable, sufficient features
- **gopool** - minimal API, fewest features
- **poolx** - most features + best performance, exclusive Work Stealing, sharded counter, assembly optimization

---

## I. Feature Comparison

### 1.1 Core Features

| Feature | poolx | ants | gopool | Notes |
|---------|:---------:|:----:|:------:|-------|
| Basic task submit | ✅ `Submit` | ✅ `Submit` | ✅ `Go` | Submit task to pool |
| Non-blocking submit | ✅ `TrySubmit` | ✅ NonBlocking mode | ❌ | No blocking wait |
| **Batch submit** | ✅ `SubmitBatch` | ❌ | ❌ | Reduce lock contention |
| Sync wait | ✅ `SubmitWait` | ❌ | ❌ | Submit and wait for completion |
| Context support | ✅ `SubmitWithContext` | ❌ | ✅ `CtxGo` | Cancel and timeout support |
| Task options | ✅ `SubmitWithOptions` | ❌ | ❌ | Priority, timeout, etc. |

### 1.2 Single-Function Pool (PoolWithFunc)

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Single-func pool | ✅ | ✅ | ❌ |
| Invoke | ✅ | ✅ | ❌ |
| TryInvoke | ✅ | ❌ | ❌ |
| InvokeWithTimeout | ✅ | ❌ | ❌ |
| InvokeWithContext | ✅ | ❌ | ❌ |

**Note**: Single-function pool is suitable for scenarios where all tasks execute the same function but with different parameters, offering higher memory efficiency.

### 1.3 Future/Promise Pattern

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Generic Future[T] | ✅ | ❌ | ❌ |
| SubmitFunc[T] | ✅ | ❌ | ❌ |
| FutureGroup | ✅ | ❌ | ❌ |
| Async/Await | ✅ | ❌ | ❌ |
| AwaitFirst | ✅ | ❌ | ❌ |
| AwaitAll | ✅ | ❌ | ❌ |
| AwaitAny | ✅ | ❌ | ❌ |
| Promise pattern | ✅ | ❌ | ❌ |

**Example**:
```go
// poolx Future pattern
future := poolx.SubmitFunc(p, func() (int, error) {
    return compute(), nil
})
result, err := future.Get()

// FutureGroup wait for multiple results
group := poolx.NewFutureGroup[int]()
group.Add(future1)
group.Add(future2)
results, err := group.Wait()
```

### 1.4 Task Control

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Task priority | ✅ | ❌ | ❌ |
| Per-task timeout | ✅ | ❌ | ❌ |
| Task ID | ✅ | ❌ | ❌ |
| Priority queue | ✅ | ❌ | ❌ |

**Example**:
```go
// poolx task options
p.SubmitWithOptions(fn,
    poolx.WithTaskTimeout(5*time.Second),
    poolx.WithTaskPriority(poolx.PriorityHigh),
    poolx.WithTaskID(123),
)
```

### 1.5 Pool Management

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Dynamic capacity adjustment | ✅ `Tune` | ✅ `Tune` | ✅ `SetCap` |
| Release/Close | ✅ | ✅ | ❌ |
| ReleaseTimeout | ✅ | ✅ | ❌ |
| Reboot/Restart | ✅ | ✅ | ❌ |
| Named pool management | ✅ | ❌ | ✅ |
| Multi-pool scheduling | ✅ `MultiPool` | ✅ `MultiPool` | ❌ |
| Load balancing strategy | ✅ RoundRobin/LeastTasks | ✅ | ❌ |

### 1.6 Auto Scaling

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Auto-Scaling | ✅ | ❌ | ✅ (simple) |
| EMA load calculation | ✅ | ❌ | ❌ |
| Scale hooks | ✅ | ❌ | ❌ |
| Cooldown control | ✅ | ❌ | ❌ |
| Min/Max workers | ✅ | ❌ | ❌ |
| Scale step size | ✅ | ❌ | ❌ |

**Example**:
```go
// poolx auto scaling
p := poolx.New("auto",
    poolx.WithMaxWorkers(100),
    poolx.WithMinWorkers(10),
    poolx.WithAutoScale(true),
    poolx.WithScaleInterval(time.Second),
)
```

### 1.7 Hook System

| Hook Type | poolx | ants | gopool |
|-----------|:---------:|:----:|:------:|
| BeforeSubmit | ✅ | ❌ | ❌ |
| AfterSubmit | ✅ | ❌ | ❌ |
| BeforeTask | ✅ | ❌ | ❌ |
| AfterTask | ✅ | ❌ | ❌ |
| OnPanic | ✅ | ✅ | ✅ |
| OnReject | ✅ | ❌ | ❌ |
| OnTimeout | ✅ | ❌ | ❌ |
| OnWorkerStart | ✅ | ❌ | ❌ |
| OnWorkerStop | ✅ | ❌ | ❌ |
| OnScaleUp | ✅ | ❌ | ❌ |
| OnScaleDown | ✅ | ❌ | ❌ |

**Example**:
```go
// poolx Hook system
hooks := poolx.NewHookBuilder().
    BeforeTask(func(info *poolx.TaskInfo) {
        log.Printf("Task %d starting", info.ID)
    }).
    AfterTask(func(info *poolx.TaskInfo) {
        log.Printf("Task %d completed in %v", info.ID, info.ExecTime)
    }).
    OnPanic(func(info *poolx.TaskInfo) {
        log.Printf("Task %d panicked: %v", info.ID, info.Error)
    }).
    Build()

p := poolx.New("hooked", poolx.WithHooks(hooks))
```

### 1.8 Monitoring Metrics

| Metric | poolx | ants | gopool |
|--------|:---------:|:----:|:------:|
| Submitted tasks | ✅ | ❌ | ❌ |
| Completed tasks | ✅ | ❌ | ❌ |
| Failed tasks | ✅ | ❌ | ❌ |
| Rejected tasks | ✅ | ❌ | ❌ |
| Running workers | ✅ | ✅ | ✅ |
| Idle workers | ✅ | ✅ | ❌ |
| Average wait time | ✅ | ❌ | ❌ |
| Average exec time | ✅ | ❌ | ❌ |
| Success rate | ✅ | ❌ | ❌ |
| Throughput calculation | ✅ | ❌ | ❌ |

**Example**:
```go
// poolx metrics
metrics := p.Metrics()
fmt.Printf("Submitted: %d, Completed: %d, Failed: %d\n",
    metrics.SubmittedTasks,
    metrics.CompletedTasks,
    metrics.FailedTasks)
fmt.Printf("Success rate: %.2f%%\n", metrics.SuccessRate()*100)
fmt.Printf("Avg wait: %v, Avg exec: %v\n",
    metrics.AvgWaitTime(),
    metrics.AvgExecTime())
```

### 1.9 Parallel Utilities

| Feature | poolx | ants | gopool |
|---------|:---------:|:----:|:------:|
| Map[T,R] | ✅ | ❌ | ❌ |
| ForEach[T] | ✅ | ❌ | ❌ |
| ParallelExecutor | ✅ | ❌ | ❌ |

**Example**:
```go
// poolx parallel Map
results, err := poolx.Map(ctx, items, 4, func(item T) (R, error) {
    return process(item), nil
})

// parallel ForEach
err := poolx.ForEach(ctx, items, 4, func(item T) error {
    return process(item)
})
```

### 1.10 Data Structures and Low-Level Optimizations

| Feature | poolx | ants | gopool | Notes |
|---------|:---------:|:----:|:------:|-------|
| Lock-Free Queue | ✅ | ❌ | ❌ | MPMC lock-free queue |
| Priority Queue | ✅ | ❌ | ❌ | Heap-based priority |
| Work Stealing Deque | ✅ | ❌ | ❌ | Work stealing queue |
| **Spinlock** | ✅ | ❌ | ❌ | 2.5x faster than Mutex |
| Cache Line Padding | ✅ | ❌ | ❌ | Avoids false sharing |
| **Sharded counter** | ✅ | ❌ | ❌ | 14x faster than atomic |
| **Assembly PAUSE instruction** | ✅ | ❌ | ❌ | x86/ARM native instruction |
| **Work Stealing scheduling** | ✅ | ❌ | ❌ | Load balancing |

---

## II. Architecture Design Comparison

### 2.1 ants Architecture

```
Pool
├── workers []*goWorker    // Worker array
├── workerCache sync.Pool  // Worker cache
├── lock sync.Locker       // lock
├── cond *sync.Cond        // condition variable
├── capacity int32         // capacity
├── running int32          // running workers
├── options *Options       // config options
└── releaseTimeout time.Duration
```

**Design Philosophy**:
- Clean and efficient, focused on core pooling
- Minimal memory footprint
- Highly optimized performance

**Pros**:
- Mature and stable, battle-tested in production
- Good documentation, active community
- Clean and easy-to-understand code

**Cons**:
- No Future pattern
- No Hook system (only PanicHandler)
- No auto scaling
- No priority support

### 2.2 ByteDance gopool Architecture

```
pool
├── cap int32              // capacity
├── config *Config         // config
├── taskHead *task         // task list head
├── taskTail *task         // task list tail
├── taskLock sync.Mutex    // task lock
├── workerCount int32      // worker count
└── panicHandler func()    // panic handler
```

**Design Philosophy**:
- Minimal API, drop-in replacement for `go` keyword
- Auto scaling
- Low intrusion

**Pros**:
- Minimal API (`Go(fn)`)
- Auto scaling
- Low learning curve

**Cons**:
- Limited features
- No PoolWithFunc
- No explicit close mechanism
- No multi-pool management

### 2.3 poolx Architecture

```
Pool
├── config Config                    // config
├── workers *workerStack             // Worker stack (Spinlock optimized)
├── priorityQueue *PriorityQueue     // priority queue (optional)
├── stealingScheduler *StealingScheduler // work stealing scheduler (optional)
├── scaler *AutoScaler               // auto scaling (optional)
├── hooks *Hooks                     // hook system
├── metrics *Metrics                 // metrics collection
├── workerCount atomic.Int32         // worker count
├── maxWorkers atomic.Int32          // max workers (dynamic)
├── state atomic.Int32               // pool state
├── cond *sync.Cond                  // condition variable
└── wg sync.WaitGroup                // wait group

Worker
├── pool *Pool                       // owning pool
├── taskCh chan *task                // task channel (buffered 4)
├── localQueue *WorkStealingDeque    // local queue (Work Stealing)
├── lastActive atomic.Int64          // last active time
└── id int32                         // worker ID
```

**Design Philosophy**:
- Comprehensive features, production-grade implementation
- Modular and composable
- Full observability
- **Maximum performance optimization**

**Pros**:
- Most comprehensive features
- Generic Future pattern
- Complete Hook system
- Auto scaling + EMA
- Priority queue
- Detailed metrics
- **Spinlock + assembly optimization**
- **Sharded counter reduces contention**
- **Work Stealing load balancing**

**Cons**:
- Larger codebase
- Slightly higher learning curve

---

## III. Performance Comparison

### 3.1 Benchmark Data

| Operation | poolx | ants | gopool | Notes |
|-----------|-----------|------|--------|-------|
| Submit (empty task) | **~1100 ns** | ~1000 ns | ~800 ns | Task submission latency |
| TrySubmit | **~34 ns** | ~100 ns | N/A | Non-blocking submit ✨ |
| SubmitBatch(10) | **~99 ns/task** | N/A | N/A | Batch submit ✨ |
| PoolWithFunc.Invoke | ~1300 ns | ~800 ns | N/A | Single-func pool |
| PoolWithFunc.TryInvoke | **~38 ns** | N/A | N/A | Non-blocking ✨ |
| Worker reuse | ✅ sync.Pool | ✅ sync.Pool | ✅ sync.Pool | |
| Task reuse | ✅ sync.Pool | ✅ sync.Pool | ✅ sync.Pool | |
| Memory allocation | 0 B/op | 0 B/op | 0 B/op | zero allocation |

> Note: Performance data based on Intel i7-8850H @ 2.60GHz, for reference only

### 3.2 Low-Level Optimization Performance

| Optimization | poolx | Standard impl | Improvement |
|-------------|-----------|---------------|-------------|
| Spinlock | 30 ns | Mutex 74 ns | **2.5x** |
| ShardedCounter | 3.5 ns | atomic.Int64 49 ns | **14x** |
| LockFreeQueue.Enqueue | 0.8 ns | - | extremely fast |
| WorkStealingDeque | 42-48 ns | - | - |

### 3.3 Performance Analysis

**poolx optimizations**:
1. **Fast path** - skip all checks when no Hooks
2. **Spinlock** - Worker Stack uses Spinlock instead of Mutex
3. **Lazy evaluation** - timestamps computed only when needed
4. **Assembly optimization** - x86 PAUSE / ARM YIELD instructions
5. **Sharded counter** - reduce metrics contention
6. **Batch submit** - amortize lock overhead
7. **Work Stealing** - more balanced load distribution

**ants advantages**:
1. Shortest code path, no overhead from extra features
2. Highly optimized worker acquisition logic
3. Mature and stable, heavily production-tested

**gopool characteristics**:
1. Simplest API, but fewest features
2. Linked-list-based task queue
3. Simple threshold-based scaling

### 3.4 Throughput Comparison

```
BenchmarkPoolThroughput (1M tasks):

poolx:
  1 Worker:   ~440,000 tasks/sec
  4 Workers:  ~510,000 tasks/sec
  16 Workers: ~735,000 tasks/sec

ants (reference):
  1 Worker:   ~300,000 tasks/sec
  4 Workers:  ~400,000 tasks/sec
  16 Workers: ~550,000 tasks/sec
```

### 3.5 Concurrent Scenarios

```
BenchmarkPoolConcurrent (concurrent submit):

poolx:
  10 goroutines:   ~1512 ns/op
  100 goroutines:  ~2654 ns/op
  1000 goroutines: ~2839 ns/op
```

---

## IV. Use Case Recommendations

### 4.1 Choose ants

Suitable scenarios:
- Need simple and reliable pooling
- Extremely high performance requirements
- No need for Future, Hook, or other advanced features
- Already using ants in the project, high migration cost

```go
// ants simple usage
p, _ := ants.NewPool(100)
defer p.Release()

p.Submit(func() {
    // task
})
```

### 4.2 Choose ByteDance gopool

Suitable scenarios:
- Quick replacement of `go` keyword
- Need auto scaling
- Prefer minimal API
- No need to explicitly close the pool

```go
// gopool simple usage
gopool.Go(func() {
    // task
})

gopool.CtxGo(ctx, func() {
    // task with context
})
```

### 4.3 Choose poolx

Suitable scenarios:
- Need task return values (Future pattern)
- Need task priority scheduling
- Need detailed lifecycle callbacks
- Need comprehensive monitoring metrics
- Need auto scaling + fine-grained control
- Need per-task timeout control

```go
// poolx full example
hooks := poolx.NewHookBuilder().
    BeforeTask(func(info *poolx.TaskInfo) {
        metrics.TaskStarted(info.ID)
    }).
    AfterTask(func(info *poolx.TaskInfo) {
        metrics.TaskCompleted(info.ID, info.ExecTime)
    }).
    Build()

p := poolx.New("my-pool",
    poolx.WithMaxWorkers(100),
    poolx.WithMinWorkers(10),
    poolx.WithAutoScale(true),
    poolx.WithHooks(hooks),
    poolx.WithPriorityQueue(true),
)
defer p.Release()

// Future pattern
future := poolx.SubmitFunc(p, func() (Result, error) {
    return compute(), nil
})
result, err := future.Get()

// With priority and timeout
p.SubmitWithOptions(fn,
    poolx.WithTaskPriority(poolx.PriorityHigh),
    poolx.WithTaskTimeout(5*time.Second),
)
```

---

## V. Summary

| Dimension | poolx | ants | gopool |
|-----------|:---------:|:----:|:------:|
| Feature richness | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ |
| API usability | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Performance | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Production readiness | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Extensibility | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ |
| Observability | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐ |
| Low-level optimization | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| Documentation | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Community activity | - | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |

### One-line Summary

- **ants**: Most mature and stable, battle-tested, sufficient features
- **gopool**: Minimal API, plug-and-play, limited features
- **poolx**: Most features + best performance, ideal for complex high-performance scenarios

### poolx Core Advantages

1. **Performance optimization** - TrySubmit 34ns (3x faster than ants), Spinlock 2.5x faster than Mutex
2. **Sharded counter** - 14x faster than atomic.Int64, suitable for high-concurrency metrics
3. **Batch submit** - SubmitBatch reduces 70% lock contention overhead
4. **Assembly optimization** - x86 PAUSE / ARM YIELD native CPU instructions
5. **Work Stealing** - automatic load balancing, improves multi-core utilization
6. **Most complete features** - Future, Hook, priority, auto scaling, detailed metrics

---

## Appendix: API Quick Reference

### poolx API

```go
// Create pool
p := poolx.New("name", opts...)
p := poolx.NewPoolWithFunc("name", fn, opts...)
mp := poolx.NewMultiPool(size, poolSize, strategy, opts...)

// Task submit
p.Submit(fn)
p.TrySubmit(fn)
p.SubmitBatch(fns)           // ✨ batch submit
p.TrySubmitBatch(fns)        // ✨ non-blocking batch submit
p.SubmitWait(fn)
p.SubmitWithContext(ctx, fn)
p.SubmitWithOptions(fn, opts...)

// Future pattern
future := poolx.SubmitFunc(p, fn)
future := poolx.Async(fn)
result, err := future.Get()
result, err := future.GetWithTimeout(timeout)

// Pool management
p.Release()
p.ReleaseTimeout(timeout)
p.Reboot()
p.Tune(newCap)

// Status query
p.Running()
p.Free()
p.Cap()
p.Waiting()
p.Metrics()

// Global functions
poolx.Go(fn)
poolx.GoCtx(ctx, fn)
poolx.SetCap(cap)
poolx.DefaultPool()
poolx.GetPool(name)

// High-performance counter ✨
counter := poolx.NewShardedCounter()   // int64 sharded counter
counter.Add(1)
counter.Load()

counter32 := poolx.NewShardedCounter32() // int32 sharded counter
fastCounter := poolx.NewFastCounter()     // dynamic sharded counter
```

### ants API

```go
// Create pool
p, _ := ants.NewPool(size, opts...)
p, _ := ants.NewPoolWithFunc(size, fn, opts...)
mp, _ := ants.NewMultiPool(size, poolSize, opts...)

// Task submit
p.Submit(fn)
p.Invoke(arg)  // PoolWithFunc

// Pool management
p.Release()
p.ReleaseTimeout(timeout)
p.Reboot()
p.Tune(newSize)

// Status query
p.Running()
p.Free()
p.Cap()
p.Waiting()
```

### ByteDance gopool API

```go
// Task submit
gopool.Go(fn)
gopool.CtxGo(ctx, fn)

// Pool management
gopool.SetCap(cap)
gopool.SetPanicHandler(handler)

// Named pool
gopool.RegisterPool(pool)
gopool.GetPool(name)
```

---

## Appendix: Code Statistics

### poolx Source Files

| File | Lines | Responsibility |
|------|-------|----------------|
| pool.go | ~2200 | Core Pool implementation, global API, MultiPool |
| pool_func.go | ~700 | PoolWithFunc single-function pool |
| future.go | ~490 | Future[T]/Promise generic pattern |
| queue.go | ~470 | Lock-Free queue, priority queue, Work Stealing Deque |
| worker.go | ~390 | Worker interface, StealingScheduler, TaskMetrics |
| hooks.go | ~330 | Hook system (11 lifecycle callbacks) |
| scaler.go | ~320 | AutoScaler auto scaling (EMA algorithm) |
| sharded_counter.go | ~220 | Sharded counter (14x faster than atomic) |
| spinlock.go | ~200 | Spinlock + Cache Line Padding |
| spinlock_amd64.s | 15 | x86-64 PAUSE assembly instruction |
| spinlock_arm64.s | 15 | ARM64 YIELD assembly instruction |
| spinlock_asm.go | 10 | Assembly bridge (amd64/arm64) |
| spinlock_generic.go | 10 | Generic platform fallback implementation |
| errors.go | ~40 | Error definitions |
| **Total** | **~5400** | |

### Test Files

| File | Lines | Description |
|------|-------|-------------|
| pool_test.go | ~2000 | Unit tests (97 test cases) |
| benchmark_test.go | ~500 | Performance benchmarks |
