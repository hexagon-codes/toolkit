// Package main 演示 poolx 包的使用方法。
//
// 本示例展示协程池的各种功能：
// - 基本任务提交
// - 单函数池 PoolWithFunc
// - Future 模式获取异步结果
// - Hook 生命周期回调
// - 自动扩缩容配置
// - 优先级队列
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexagon-codes/toolkit/util/poolx"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fmt.Println("=== 协程池示例 ===")
	fmt.Println()

	for _, example := range []func() error{
		basicUsage,
		poolWithFuncExample,
		futureExample,
		hooksExample,
		nonBlockingExample,
		taskTimeoutExample,
		multiPoolExample,
		mapForEachExample,
		batchSubmitExample,
	} {
		if err := example(); err != nil {
			return err
		}
	}

	fmt.Println()
	fmt.Println("=== 所有示例完成 ===")
	return nil
}

// basicUsage 演示基本的池操作
func basicUsage() error {
	fmt.Println("--- 示例 1: 基础用法 ---")

	// 创建一个 4 个 worker 的池
	p := poolx.New("basic-pool",
		poolx.WithMaxWorkers(4),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	// 提交 100 个任务
	for i := 0; i < 100; i++ {
		wg.Add(1)
		if err := p.Submit(func() {
			defer wg.Done()
			counter.Add(1)
		}); err != nil {
			wg.Done()
			return fmt.Errorf("submit basic task %d: %w", i, err)
		}
	}

	wg.Wait()
	fmt.Printf("完成 %d 个任务，使用 %d 个 worker\n", counter.Load(), p.Running())

	// 获取指标
	metrics := p.Metrics()
	fmt.Printf("指标: 提交=%d, 完成=%d\n",
		metrics.SubmittedTasks,
		metrics.CompletedTasks)
	fmt.Println()
	return nil
}

// poolWithFuncExample 演示单函数池
func poolWithFuncExample() error {
	fmt.Println("--- 示例 2: 单函数池 ---")

	var sum atomic.Int64
	var wg sync.WaitGroup

	// 创建一个处理整数的池
	p := poolx.NewPoolWithFunc("sum-pool",
		func(arg any) {
			defer wg.Done()
			n := arg.(int)
			sum.Add(int64(n))
		},
		poolx.WithMaxWorkers(4),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	// 提交 100 个数字
	for i := 1; i <= 100; i++ {
		wg.Add(1)
		if err := p.Invoke(i); err != nil {
			wg.Done()
			return fmt.Errorf("invoke sum task %d: %w", i, err)
		}
	}

	wg.Wait()
	fmt.Printf("1-100 的和: %d (期望: 5050)\n", sum.Load())
	fmt.Println()
	return nil
}

// futureExample 演示 Future 模式
func futureExample() error {
	fmt.Println("--- 示例 3: Future 模式 ---")

	p := poolx.New("future-pool",
		poolx.WithMaxWorkers(4),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	// 提交一个返回结果的任务
	future := poolx.SubmitFunc(p, func() (int, error) {
		time.Sleep(10 * time.Millisecond)
		return 42, nil
	})

	// 在等待时可以做其他工作...
	fmt.Println("正在做其他工作...")

	// 获取结果（阻塞直到完成）
	result, err := future.Get()
	if err != nil {
		return fmt.Errorf("get future result: %w", err)
	}
	fmt.Printf("Future result: %d\n", result)

	// FutureGroup: 等待多个 future
	group := poolx.NewFutureGroup[int]()
	for i := 0; i < 5; i++ {
		n := i
		f := poolx.SubmitFunc(p, func() (int, error) {
			return n * n, nil
		})
		group.Add(f)
	}

	results, err := group.Wait()
	if err != nil {
		return fmt.Errorf("wait for future group: %w", err)
	}
	fmt.Printf("FutureGroup 结果: %v (0-4 的平方)\n", results)
	fmt.Println()
	return nil
}

// hooksExample 演示生命周期 hook
func hooksExample() error {
	fmt.Println("--- 示例 4: Hook 回调 ---")

	var taskCount atomic.Int32

	hooks := poolx.NewHookBuilder().
		BeforeTask(func(info *poolx.TaskInfo) {
			taskCount.Add(1)
		}).
		AfterTask(func(info *poolx.TaskInfo) {
			// 任务完成
		}).
		OnPanic(func(info *poolx.TaskInfo) {
			fmt.Printf("任务 panic: %v\n", info.Error)
		}).
		Build()

	p := poolx.New("hooks-pool",
		poolx.WithMaxWorkers(2),
		poolx.WithAutoScale(false),
		poolx.WithHooks(hooks),
		poolx.WithPanicHandler(func(v any) {}), // 抑制默认 panic 输出
	)
	defer p.Release()
	var wg sync.WaitGroup

	// 提交正常任务
	for i := 0; i < 5; i++ {
		wg.Add(1)
		if err := p.Submit(func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)
		}); err != nil {
			wg.Done()
			return fmt.Errorf("submit hook task %d: %w", i, err)
		}
	}

	// 提交一个会 panic 的任务
	wg.Add(1)
	if err := p.Submit(func() {
		defer wg.Done()
		panic("故意的 panic")
	}); err != nil {
		wg.Done()
		return fmt.Errorf("submit panic hook task: %w", err)
	}

	wg.Wait()
	fmt.Printf("BeforeTask hook 调用了 %d 次\n", taskCount.Load())
	fmt.Println()
	return nil
}

// nonBlockingExample 演示非阻塞模式
func nonBlockingExample() error {
	fmt.Println("--- 示例 5: 非阻塞模式 ---")

	p := poolx.New("nonblocking-pool",
		poolx.WithMaxWorkers(2),
		poolx.WithNonBlocking(true),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	var submitted, rejected atomic.Int32
	blocker := make(chan struct{})

	// 用阻塞任务填满池
	for i := 0; i < 2; i++ {
		if err := p.Submit(func() {
			<-blocker
		}); err != nil {
			close(blocker)
			return fmt.Errorf("submit blocking task %d: %w", i, err)
		}
	}

	time.Sleep(10 * time.Millisecond)

	// 尝试提交更多（应该被拒绝）
	for i := 0; i < 10; i++ {
		err := p.Submit(func() {})
		if errors.Is(err, poolx.ErrPoolOverload) {
			rejected.Add(1)
		} else if err == nil {
			submitted.Add(1)
		} else {
			close(blocker)
			return fmt.Errorf("submit non-blocking task %d: %w", i, err)
		}
	}

	close(blocker)
	fmt.Printf("已提交: %d, 已拒绝: %d\n", submitted.Load(), rejected.Load())
	fmt.Println()
	return nil
}

// taskTimeoutExample 演示由 context 驱动的协作式单任务超时
func taskTimeoutExample() error {
	fmt.Println("--- 示例 6: 任务超时 ---")

	p := poolx.New("timeout-pool",
		poolx.WithMaxWorkers(2),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	// 调用方创建并持有超时 context，任务通过监听 Done 协作结束。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	taskDone := make(chan struct{})
	err := p.SubmitWithContext(ctx, func(taskCtx context.Context) {
		defer close(taskDone)

		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-taskCtx.Done():
		}
	})

	if err != nil {
		return fmt.Errorf("submit context task: %w", err)
	}

	<-taskDone
	fmt.Printf("Task timed out: %v\n", ctx.Err() == context.DeadlineExceeded)
	fmt.Println()
	return nil
}

// multiPoolExample 演示多池负载均衡
func multiPoolExample() error {
	fmt.Println("--- 示例 7: 多池负载均衡 ---")

	// 创建 3 个池，每个 4 个 worker，使用轮询策略
	mp, err := poolx.NewMultiPool(3, 4, poolx.RoundRobin,
		poolx.WithAutoScale(false),
	)
	if err != nil {
		return fmt.Errorf("create multi-pool: %w", err)
	}
	defer mp.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	// 提交 100 个任务（分发到各个池）
	for i := 0; i < 100; i++ {
		wg.Add(1)
		if err := mp.Submit(func() {
			defer wg.Done()
			counter.Add(1)
		}); err != nil {
			wg.Done()
			return fmt.Errorf("submit multi-pool task %d: %w", i, err)
		}
	}

	wg.Wait()
	fmt.Printf("多池完成 %d 个任务\n", counter.Load())
	fmt.Printf("总运行 worker: %d, 空闲容量: %d\n",
		mp.Running(), mp.Free())
	fmt.Println()
	return nil
}

// mapForEachExample 演示并行 Map 和 ForEach
func mapForEachExample() error {
	fmt.Println("--- 示例 8: 并行 Map/ForEach ---")

	// 并行 Map: 计算每个数字的平方（4 个并发 worker）
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	squares, err := poolx.Map(context.Background(), numbers, 4, func(n int) (int, error) {
		return n * n, nil
	})
	if err != nil {
		return fmt.Errorf("map numbers: %w", err)
	}
	fmt.Printf("Squares: %v\n", squares)

	// 并行 ForEach: 处理每个元素（4 个并发 worker）
	var sum atomic.Int64
	err = poolx.ForEach(context.Background(), numbers, 4, func(n int) error {
		sum.Add(int64(n))
		return nil
	})
	if err != nil {
		return fmt.Errorf("iterate numbers: %w", err)
	}
	fmt.Printf("Sum: %d (expected: 55)\n", sum.Load())
	fmt.Println()
	return nil
}

// batchSubmitExample 演示批量提交
func batchSubmitExample() error {
	fmt.Println("--- 示例 9: 批量提交 ---")

	p := poolx.New("batch-pool",
		poolx.WithMaxWorkers(4),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	// 创建一批任务
	var counter atomic.Int32
	var wg sync.WaitGroup
	batch := make([]func(), 100)
	for i := range batch {
		wg.Add(1)
		batch[i] = func() {
			counter.Add(1)
			wg.Done()
		}
	}

	// 批量提交（减少锁开销）
	submitted, err := p.SubmitBatch(batch)
	if err != nil {
		for i := submitted; i < len(batch); i++ {
			wg.Done()
		}
		wg.Wait()
		return fmt.Errorf("submit task batch: %w", err)
	}
	if submitted != len(batch) {
		for i := submitted; i < len(batch); i++ {
			wg.Done()
		}
		wg.Wait()
		return fmt.Errorf("submit task batch: submitted %d of %d tasks", submitted, len(batch))
	}

	wg.Wait()
	fmt.Printf("批量提交 %d 个任务，完成 %d 个\n", submitted, counter.Load())

	// TrySubmitBatch: 非阻塞批量提交
	var tryWG sync.WaitGroup
	batch2 := make([]func(), 20)
	for i := range batch2 {
		tryWG.Add(1)
		batch2[i] = func() {
			defer tryWG.Done()
			time.Sleep(time.Millisecond)
		}
	}
	submitted2 := p.TrySubmitBatch(batch2)
	for i := submitted2; i < len(batch2); i++ {
		tryWG.Done()
	}
	tryWG.Wait()
	fmt.Printf("TrySubmitBatch 提交了 %d 个任务\n", submitted2)
	fmt.Println()
	return nil
}
