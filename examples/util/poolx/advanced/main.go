// Package main 演示 poolx 包的高级功能。
//
// 本示例涵盖：
// - 自动扩缩容配置
// - 优先级任务调度
// - Async/Await 模式
// - Context 取消
// - 指标监控
// - 命名池管理
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
	fmt.Println("=== 协程池高级示例 ===")
	fmt.Println()

	if err := autoScalingExample(); err != nil {
		return err
	}

	// 示例 2: 优先级队列
	if err := priorityQueueExample(); err != nil {
		return err
	}

	if err := asyncAwaitExample(); err != nil {
		return err
	}

	if err := contextCancellationExample(); err != nil {
		return err
	}

	if err := metricsMonitoringExample(); err != nil {
		return err
	}

	// 示例 6: 命名池
	namedPoolsExample()

	if err := globalPoolExample(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("=== 所有高级示例完成 ===")
	return nil
}

// autoScalingExample 演示自动扩缩容配置
func autoScalingExample() error {
	fmt.Println("--- 示例 1: 自动扩缩容 ---")

	p := poolx.New("autoscale-pool",
		poolx.WithMaxWorkers(20),
		poolx.WithMinWorkers(2),
		poolx.WithAutoScale(true),
		poolx.WithScaleInterval(100*time.Millisecond),
	)
	defer p.Release()

	fmt.Printf("初始 worker 数: %d\n", p.Running())

	// 产生高负载
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		if err := p.Submit(func() {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond)
		}); err != nil {
			wg.Done()
			return fmt.Errorf("submit auto-scaling task %d: %w", i, err)
		}
	}

	// 检查负载期间的 worker 数量
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("负载期间 worker 数: %d\n", p.Running())

	wg.Wait()

	// 等待缩容
	time.Sleep(1 * time.Second)
	fmt.Printf("缩容后 worker 数: %d\n", p.Running())
	fmt.Println()
	return nil
}

// priorityQueueExample 演示优先级调度
func priorityQueueExample() error {
	fmt.Println("--- 示例 2: 优先级队列 ---")
	order, err := priorityExecutionOrder()
	if err != nil {
		return err
	}
	fmt.Printf("Execution order (highest priority first): %v\n", order)
	fmt.Println()
	return nil
}

func priorityExecutionOrder() ([]int, error) {
	queue := poolx.NewPriorityQueue(4)
	var order []int

	// 提交不同优先级的任务
	priorities := []struct {
		priority int
		label    int
	}{
		{poolx.PriorityLow, 1},    // 优先级 0
		{poolx.PriorityNormal, 2}, // 优先级 5
		{poolx.PriorityHigh, 3},   // 优先级 10
		{15, 4},                   // 自定义高优先级
	}

	for _, pr := range priorities {
		label := pr.label
		if ok := queue.Push(func() {
			order = append(order, label)
		}, pr.priority); !ok {
			return nil, fmt.Errorf("enqueue priority task %d: queue is full", label)
		}
	}

	for !queue.IsEmpty() {
		task := queue.Pop()
		if task == nil {
			return nil, fmt.Errorf("pop priority task: queue returned nil before becoming empty")
		}
		task()
	}
	return order, nil
}

// asyncAwaitExample 演示 Async/Await 模式
func asyncAwaitExample() error {
	fmt.Println("--- 示例 3: Async/Await 模式 ---")

	// Async: 异步启动任务
	f1 := poolx.Async(func() (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "result 1", nil
	})

	f2 := poolx.Async(func() (string, error) {
		time.Sleep(20 * time.Millisecond)
		return "result 2", nil
	})

	f3 := poolx.Async(func() (string, error) {
		time.Sleep(5 * time.Millisecond)
		return "result 3", nil
	})

	// Await 第一个完成的
	result, idx, err := poolx.AwaitFirst(f1, f2, f3)
	if err != nil {
		return fmt.Errorf("await first future: %w", err)
	}
	fmt.Printf("First result: %q (index: %d)\n", result, idx)

	// Await 所有完成
	results, err := poolx.AwaitAll(f1, f2, f3)
	if err != nil {
		return fmt.Errorf("await all futures: %w", err)
	}
	fmt.Printf("All results: %v\n", results)
	fmt.Println()
	return nil
}

// contextCancellationExample 演示 Context 取消
func contextCancellationExample() error {
	fmt.Println("--- 示例 4: Context 取消 ---")

	p := poolx.New("ctx-pool",
		poolx.WithMaxWorkers(4),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())

	var started, completed atomic.Int32

	// 提交一个长时间运行的任务
	future := poolx.SubmitFuncCtx(ctx, p, func(ctx context.Context) (string, error) {
		started.Add(1)
		select {
		case <-time.After(1 * time.Second):
			completed.Add(1)
			return "完成", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// 50ms 后取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	// 尝试获取结果
	result, err := future.GetWithTimeout(100 * time.Millisecond)
	fmt.Printf("已启动: %d, 已完成: %d\n", started.Load(), completed.Load())
	if errors.Is(err, context.Canceled) {
		fmt.Printf("任务已取消: %v\n", err)
	} else if err != nil {
		return fmt.Errorf("wait for canceled future: %w", err)
	} else {
		return fmt.Errorf("wait for canceled future: unexpected result %q", result)
	}
	fmt.Println()
	return nil
}

// metricsMonitoringExample 演示指标采集
func metricsMonitoringExample() error {
	fmt.Println("--- 示例 5: 指标监控 ---")

	p := poolx.New("metrics-pool",
		poolx.WithMaxWorkers(1),
		poolx.WithAutoScale(false),
		poolx.WithNonBlocking(true),
	)
	defer p.Release()

	// 先占满所有 worker，确定性触发非阻塞拒绝。
	blocker := make(chan struct{})
	var blockingTasks sync.WaitGroup
	for i := 0; i < 1; i++ {
		started := make(chan struct{})
		blockingTasks.Add(1)
		if err := p.Submit(func() {
			defer blockingTasks.Done()
			close(started)
			<-blocker
		}); err != nil {
			blockingTasks.Done()
			close(blocker)
			return fmt.Errorf("submit metrics blocker %d: %w", i, err)
		}
		<-started
	}

	// 池满时提交必须被拒绝。
	for i := 0; i < 10; i++ {
		err := p.Submit(func() {})
		if !errors.Is(err, poolx.ErrPoolOverload) {
			close(blocker)
			blockingTasks.Wait()
			return fmt.Errorf("submit overloaded metrics task %d: got %v, want pool overload", i, err)
		}
	}
	close(blocker)
	blockingTasks.Wait()

	panicDone := make(chan struct{})
	if err := p.Submit(func() {
		defer close(panicDone)
		panic("example panic")
	}); err != nil {
		return fmt.Errorf("submit metrics panic task: %w", err)
	}
	<-panicDone
	if err := p.SubmitWait(func() {}); err != nil {
		return fmt.Errorf("wait for metrics panic accounting: %w", err)
	}

	// 获取指标快照
	snapshot := p.Metrics()
	fmt.Printf("指标快照:\n")
	fmt.Printf("  提交任务数:   %d\n", snapshot.SubmittedTasks)
	fmt.Printf("  完成任务数:   %d\n", snapshot.CompletedTasks)
	fmt.Printf("  失败任务数:   %d\n", snapshot.FailedTasks)
	fmt.Printf("  拒绝任务数:   %d\n", snapshot.RejectedTasks)
	fmt.Printf("  运行 Worker:  %d\n", snapshot.RunningWorkers)
	fmt.Printf("  成功率:       %.2f%%\n", snapshot.SuccessRate()*100)
	fmt.Println()
	return nil
}

// namedPoolsExample 演示命名池管理
func namedPoolsExample() {
	fmt.Println("--- 示例 6: 命名池 ---")

	// 创建命名池
	pool1 := poolx.New("worker-pool-1", poolx.WithMaxWorkers(4))
	pool2 := poolx.New("worker-pool-2", poolx.WithMaxWorkers(8))
	defer pool1.Release()
	defer pool2.Release()

	// 按名称获取
	p1, ok := poolx.GetPool("worker-pool-1")
	if ok {
		fmt.Printf("找到池: %s 容量 %d\n", p1.Name(), p1.Cap())
	}

	// 列出所有池
	fmt.Println("所有已注册的池:")
	poolx.RangePool(func(name string, p *poolx.Pool) bool {
		fmt.Printf("  - %s (容量: %d, 运行: %d)\n", name, p.Cap(), p.Running())
		return true
	})

	// 使用 MustGetPool (找不到会 panic)
	p2 := poolx.MustGetPool("worker-pool-2")
	fmt.Printf("MustGetPool: %s\n", p2.Name())
	fmt.Println()
}

// globalPoolExample 演示全局默认池
func globalPoolExample() error {
	fmt.Println("--- 示例 7: 全局池 ---")

	var counter atomic.Int32

	var wg sync.WaitGroup
	wg.Add(2)

	// 使用 Go() 简单异步执行
	if err := poolx.Go(func() {
		defer wg.Done()
		counter.Add(1)
	}); err != nil {
		wg.Done()
		return fmt.Errorf("submit global task: %w", err)
	}

	// 使用 GoCtx() 带 context 执行
	ctx := context.Background()
	err := poolx.GoCtx(ctx, func(context.Context) {
		defer wg.Done()
		counter.Add(1)
	})
	if err != nil {
		wg.Done()
		return fmt.Errorf("submit global context task: %w", err)
	}

	wg.Wait()
	fmt.Printf("全局池执行了 %d 个任务\n", counter.Load())

	// 调整全局池容量
	poolx.SetCap(100)
	fmt.Printf("默认池容量: %d\n", poolx.DefaultPool().Cap())
	fmt.Println()
	return nil
}
