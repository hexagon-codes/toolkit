// Package main 演示协程池的性能基准测试。
//
// 本示例展示如何测量池性能并比较不同的池配置。
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/everyday-items/toolkit/util/poolx"
)

func main() {
	fmt.Println("=== 协程池性能基准测试 ===")
	fmt.Println()

	numTasks := 1000000
	numWorkers := runtime.NumCPU()

	// 基准测试 1: 原生 goroutine
	benchmarkNativeGoroutines(numTasks)

	// 基准测试 2: Pool Submit
	benchmarkPoolSubmit(numTasks, numWorkers)

	// 基准测试 3: Pool TrySubmit
	benchmarkPoolTrySubmit(numTasks, numWorkers)

	// 基准测试 4: PoolWithFunc
	benchmarkPoolWithFunc(numTasks, numWorkers)

	// 基准测试 5: 不同 worker 数量
	benchmarkWorkerScaling(numTasks)

	fmt.Println()
	fmt.Println("=== 基准测试完成 ===")
}

func benchmarkNativeGoroutines(n int) {
	fmt.Printf("--- 原生 Goroutine (%d 任务) ---\n", n)

	var wg sync.WaitGroup
	var counter atomic.Int64

	start := time.Now()
	for range n {
		wg.Add(1)
		go func() {
			counter.Add(1)
			wg.Done()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	fmt.Printf("  耗时: %v\n", elapsed)
	fmt.Printf("  吞吐量: %.2f 任务/秒\n", float64(n)/elapsed.Seconds())
	fmt.Printf("  计数器: %d\n\n", counter.Load())
}

func benchmarkPoolSubmit(n int, workers int) {
	fmt.Printf("--- Pool Submit (%d 任务, %d worker) ---\n", n, workers)

	p := poolx.New("bench-submit",
		poolx.WithMaxWorkers(int32(workers)),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	var wg sync.WaitGroup
	var counter atomic.Int64

	start := time.Now()
	for range n {
		wg.Add(1)
		_ = p.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	metrics := p.Metrics()
	fmt.Printf("  耗时: %v\n", elapsed)
	fmt.Printf("  吞吐量: %.2f 任务/秒\n", float64(n)/elapsed.Seconds())
	fmt.Printf("  计数器: %d\n", counter.Load())
	fmt.Printf("  完成: %d, 失败: %d\n\n", metrics.CompletedTasks, metrics.FailedTasks)
}

func benchmarkPoolTrySubmit(n int, workers int) {
	fmt.Printf("--- Pool TrySubmit (%d 任务, %d worker) ---\n", n, workers)

	p := poolx.New("bench-trysubmit",
		poolx.WithMaxWorkers(int32(workers)),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	var wg sync.WaitGroup
	var counter atomic.Int64
	var submitted, rejected atomic.Int64

	start := time.Now()
	for range n {
		wg.Add(1)
		if p.TrySubmit(func() {
			counter.Add(1)
			wg.Done()
		}) {
			submitted.Add(1)
		} else {
			rejected.Add(1)
			wg.Done() // 任务被拒绝，递减等待组
		}
	}
	wg.Wait()
	elapsed := time.Since(start)

	fmt.Printf("  耗时: %v\n", elapsed)
	fmt.Printf("  吞吐量: %.2f 任务/秒\n", float64(n)/elapsed.Seconds())
	fmt.Printf("  已提交: %d, 已拒绝: %d\n\n", submitted.Load(), rejected.Load())
}

func benchmarkPoolWithFunc(n int, workers int) {
	fmt.Printf("--- PoolWithFunc (%d 任务, %d worker) ---\n", n, workers)

	var counter atomic.Int64

	p := poolx.NewPoolWithFunc("bench-func",
		func(arg any) {
			counter.Add(arg.(int64))
		},
		poolx.WithMaxWorkers(int32(workers)),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	var wg sync.WaitGroup

	start := time.Now()
	for range n {
		wg.Add(1)
		_ = p.Invoke(int64(1))
	}

	// 等待所有任务完成
	for counter.Load() < int64(n) {
		runtime.Gosched()
	}
	elapsed := time.Since(start)

	fmt.Printf("  耗时: %v\n", elapsed)
	fmt.Printf("  吞吐量: %.2f 任务/秒\n", float64(n)/elapsed.Seconds())
	fmt.Printf("  计数器: %d\n\n", counter.Load())
}

func benchmarkWorkerScaling(n int) {
	fmt.Println("--- Worker 数量伸缩对比 ---")

	workerCounts := []int{1, 2, 4, 8, 16, 32}

	for _, workers := range workerCounts {
		p := poolx.New("bench-scale",
			poolx.WithMaxWorkers(int32(workers)),
			poolx.WithAutoScale(false),
		)

		var wg sync.WaitGroup
		var counter atomic.Int64

		start := time.Now()
		for range n / 10 { // 使用较少任务进行伸缩测试
			wg.Add(1)
			_ = p.Submit(func() {
				counter.Add(1)
				wg.Done()
			})
		}
		wg.Wait()
		elapsed := time.Since(start)

		p.Release()
		fmt.Printf("  Worker: %2d | 耗时: %10v | 吞吐量: %10.0f/秒\n",
			workers, elapsed, float64(n/10)/elapsed.Seconds())
	}
	fmt.Println()
}
