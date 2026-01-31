package poolx

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// ============================================================================
// Core Pool Benchmarks
// ============================================================================

func BenchmarkPoolSubmit(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Submit(func() {})
		}
	})
}

func BenchmarkPoolTrySubmit(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.TrySubmit(func() {})
		}
	})
}

func BenchmarkPoolSubmitWithOptions(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.SubmitWithOptions(func() {}, WithTaskPriority(PriorityHigh))
	}
}

func BenchmarkPoolSubmitWait(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.SubmitWait(func() {})
	}
}

// ============================================================================
// PoolWithFunc Benchmarks
// ============================================================================

func BenchmarkPoolWithFuncInvoke(b *testing.B) {
	p := NewPoolWithFunc("bench", func(arg any) {
		_ = arg
	}, WithMaxWorkers(int32(runtime.NumCPU())), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Invoke(1)
		}
	})
}

func BenchmarkPoolWithFuncTryInvoke(b *testing.B) {
	p := NewPoolWithFunc("bench", func(arg any) {
		_ = arg
	}, WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.TryInvoke(1)
		}
	})
}

// ============================================================================
// Future Benchmarks
// ============================================================================

func BenchmarkFutureSubmitFunc(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := SubmitFunc(p, func() (int, error) {
			return 42, nil
		})
		_, _ = f.Get()
	}
}

func BenchmarkFutureAsyncAwait(b *testing.B) {
	initDefaultPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := Async(func() (int, error) {
			return 42, nil
		})
		_, _ = Await(f)
	}
}

func BenchmarkFutureGroup(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group := NewFutureGroup[int]()
		for j := 0; j < 10; j++ {
			f := SubmitFunc(p, func() (int, error) {
				return j, nil
			})
			group.Add(f)
		}
		_, _ = group.Wait()
	}
}

// ============================================================================
// Queue Benchmarks
// ============================================================================

func BenchmarkLockFreeQueueEnqueue(b *testing.B) {
	q := NewLockFreeQueue[int](1024)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			q.Enqueue(i)
			i++
		}
	})
}

func BenchmarkLockFreeQueueDequeue(b *testing.B) {
	q := NewLockFreeQueue[int](1024)
	for i := 0; i < 1024; i++ {
		q.Enqueue(i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, ok := q.Dequeue(); !ok {
				// Queue empty, refill
				for i := 0; i < 100; i++ {
					q.Enqueue(i)
				}
			}
		}
	})
}

func BenchmarkLockFreeQueueMixed(b *testing.B) {
	q := NewLockFreeQueue[int](1024)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				q.Enqueue(i)
			} else {
				q.Dequeue()
			}
			i++
		}
	})
}

func BenchmarkPriorityQueuePush(b *testing.B) {
	pq := NewPriorityQueue(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Push(func() {}, i%10)
	}
}

func BenchmarkPriorityQueuePushPop(b *testing.B) {
	pq := NewPriorityQueue(0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Push(func() {}, i%10)
		pq.Pop()
	}
}

// ============================================================================
// Spinlock Benchmarks
// ============================================================================

func BenchmarkSpinlock(b *testing.B) {
	var lock Spinlock

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lock.Lock()
			lock.Unlock()
		}
	})
}

func BenchmarkMutex(b *testing.B) {
	var mu sync.Mutex

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			mu.Unlock()
		}
	})
}

func BenchmarkSpinlockContended(b *testing.B) {
	var lock Spinlock
	var counter int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lock.Lock()
			counter++
			lock.Unlock()
		}
	})
}

func BenchmarkMutexContended(b *testing.B) {
	var mu sync.Mutex
	var counter int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
}

// ============================================================================
// Work Stealing Deque Benchmarks
// ============================================================================

func BenchmarkWorkStealingDequePush(b *testing.B) {
	d := NewWorkStealingDeque[int](1024)
	val := 42

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.PushBottom(&val)
		d.PopBottom()
	}
}

func BenchmarkWorkStealingDequeSteal(b *testing.B) {
	d := NewWorkStealingDeque[int](1024)
	val := 42
	for i := 0; i < 100; i++ {
		d.PushBottom(&val)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if d.Steal() == nil {
				d.PushBottom(&val)
			}
		}
	})
}

// ============================================================================
// Comparison Benchmarks
// ============================================================================

func BenchmarkNativeGoroutine(b *testing.B) {
	var wg sync.WaitGroup
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			wg.Done()
		}()
	}
	wg.Wait()
}

func BenchmarkGo(b *testing.B) {
	initDefaultPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Go(func() {})
	}
}

func BenchmarkMultiPoolSubmit(b *testing.B) {
	mp := NewMultiPool(4, int32(runtime.NumCPU()), RoundRobin)
	defer mp.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = mp.Submit(func() {})
		}
	})
}

// ============================================================================
// Parallel Map/ForEach Benchmarks
// ============================================================================

func BenchmarkMap(b *testing.B) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Map(ctx, items, runtime.NumCPU(), func(n int) (int, error) {
			return n * 2, nil
		})
	}
}

func BenchmarkForEach(b *testing.B) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ForEach(ctx, items, runtime.NumCPU(), func(n int) error {
			return nil
		})
	}
}

// ============================================================================
// Hooks Overhead Benchmarks
// ============================================================================

func BenchmarkPoolWithHooks(b *testing.B) {
	hooks := NewHookBuilder().
		BeforeTask(func(info *TaskInfo) {}).
		AfterTask(func(info *TaskInfo) {}).
		Build()

	p := New("bench",
		WithMaxWorkers(int32(runtime.NumCPU())),
		WithAutoScale(false),
		WithHooks(hooks),
	)
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Submit(func() {})
	}
}

func BenchmarkPoolWithoutHooks(b *testing.B) {
	p := New("bench",
		WithMaxWorkers(int32(runtime.NumCPU())),
		WithAutoScale(false),
	)
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Submit(func() {})
	}
}

// ============================================================================
// Memory Allocation Benchmarks
// ============================================================================

func BenchmarkPoolSubmitAllocs(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Submit(func() {})
	}
}

func BenchmarkPoolWithFuncInvokeAllocs(b *testing.B) {
	p := NewPoolWithFunc("bench", func(arg any) {
		_ = arg
	}, WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Invoke(1)
	}
}

func BenchmarkFutureAllocs(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := SubmitFunc(p, func() (int, error) {
			return 42, nil
		})
		_, _ = f.Get()
	}
}

// ============================================================================
// Throughput Benchmarks
// ============================================================================

func BenchmarkPoolThroughput1Worker(b *testing.B) {
	benchmarkPoolThroughput(b, 1)
}

func BenchmarkPoolThroughput4Workers(b *testing.B) {
	benchmarkPoolThroughput(b, 4)
}

func BenchmarkPoolThroughput16Workers(b *testing.B) {
	benchmarkPoolThroughput(b, 16)
}

func BenchmarkPoolThroughputNumCPU(b *testing.B) {
	benchmarkPoolThroughput(b, int32(runtime.NumCPU()))
}

func benchmarkPoolThroughput(b *testing.B, workers int32) {
	p := New("bench", WithMaxWorkers(workers), WithAutoScale(false))
	defer p.Release()

	var counter atomic.Int64
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		_ = p.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
	}
	wg.Wait()
}

// ============================================================================
// Latency Benchmarks
// ============================================================================

func BenchmarkPoolLatency(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())), WithAutoScale(false))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		_ = p.Submit(func() {
			close(done)
		})
		<-done
	}
}

// ============================================================================
// Concurrent Access Benchmarks
// ============================================================================

func BenchmarkPoolConcurrent10(b *testing.B) {
	benchmarkPoolConcurrent(b, 10)
}

func BenchmarkPoolConcurrent100(b *testing.B) {
	benchmarkPoolConcurrent(b, 100)
}

func BenchmarkPoolConcurrent1000(b *testing.B) {
	benchmarkPoolConcurrent(b, 1000)
}

func benchmarkPoolConcurrent(b *testing.B, goroutines int) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	var wg sync.WaitGroup
	tasksPerGoroutine := b.N / goroutines
	if tasksPerGoroutine == 0 {
		tasksPerGoroutine = 1
	}

	b.ResetTimer()
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				_ = p.Submit(func() {})
			}
		}()
	}
	wg.Wait()
}

// ============================================================================
// Sharded Counter Benchmarks
// ============================================================================

func BenchmarkShardedCounterAdd(b *testing.B) {
	c := NewShardedCounter()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Add(1)
		}
	})
}

func BenchmarkAtomicInt64Add(b *testing.B) {
	var c atomic.Int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Add(1)
		}
	})
}

func BenchmarkShardedCounterLoad(b *testing.B) {
	c := NewShardedCounter()
	for i := 0; i < 1000; i++ {
		c.Add(1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Load()
	}
}

func BenchmarkFastCounterAdd(b *testing.B) {
	c := NewFastCounter()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Add(1)
		}
	})
}

// ============================================================================
// Batch Submit Benchmarks
// ============================================================================

func BenchmarkPoolSubmitBatch10(b *testing.B) {
	benchmarkPoolSubmitBatch(b, 10)
}

func BenchmarkPoolSubmitBatch100(b *testing.B) {
	benchmarkPoolSubmitBatch(b, 100)
}

func benchmarkPoolSubmitBatch(b *testing.B, batchSize int) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	// Pre-create batch of functions
	batch := make([]func(), batchSize)
	for i := range batch {
		batch[i] = func() {}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.SubmitBatch(batch)
	}
}

func BenchmarkPoolTrySubmitBatch10(b *testing.B) {
	p := New("bench", WithMaxWorkers(int32(runtime.NumCPU())*4), WithAutoScale(false))
	defer p.Release()

	batch := make([]func(), 10)
	for i := range batch {
		batch[i] = func() {}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.TrySubmitBatch(batch)
	}
}

// ============================================================================
// Work Stealing Benchmarks
// ============================================================================

func BenchmarkPoolWithWorkStealing(b *testing.B) {
	p := New("bench",
		WithMaxWorkers(int32(runtime.NumCPU())),
		WithAutoScale(false),
		WithWorkStealing(true),
	)
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Submit(func() {})
		}
	})
}

func BenchmarkPoolWithoutWorkStealing(b *testing.B) {
	p := New("bench",
		WithMaxWorkers(int32(runtime.NumCPU())),
		WithAutoScale(false),
		WithWorkStealing(false),
	)
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Submit(func() {})
		}
	})
}
