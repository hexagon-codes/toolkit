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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/everyday-items/toolkit/util/poolx"
)

func main() {
	fmt.Println("=== 协程池高级示例 ===")
	fmt.Println()

	// 示例 1: 自动扩缩容
	autoScalingExample()

	// 示例 2: 优先级队列
	priorityQueueExample()

	// 示例 3: Async/Await 模式
	asyncAwaitExample()

	// 示例 4: Context 取消
	contextCancellationExample()

	// 示例 5: 指标监控
	metricsMonitoringExample()

	// 示例 6: 命名池
	namedPoolsExample()

	// 示例 7: 全局池 (Go/GoCtx)
	globalPoolExample()

	fmt.Println()
	fmt.Println("=== 所有高级示例完成 ===")
}

// autoScalingExample 演示自动扩缩容配置
func autoScalingExample() {
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
		_ = p.Submit(func() {
			time.Sleep(50 * time.Millisecond)
			wg.Done()
		})
	}

	// 检查负载期间的 worker 数量
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("负载期间 worker 数: %d\n", p.Running())

	wg.Wait()

	// 等待缩容
	time.Sleep(1 * time.Second)
	fmt.Printf("缩容后 worker 数: %d\n", p.Running())
	fmt.Println()
}

// priorityQueueExample 演示优先级调度
func priorityQueueExample() {
	fmt.Println("--- 示例 2: 优先级队列 ---")

	p := poolx.New("priority-pool",
		poolx.WithMaxWorkers(1), // 单 worker 以显示顺序
		poolx.WithPriorityQueue(true),
		poolx.WithAutoScale(false),
	)
	defer p.Release()

	// 阻塞 worker
	blocker := make(chan struct{})
	_ = p.Submit(func() {
		<-blocker
	})

	time.Sleep(10 * time.Millisecond)

	var order []int
	var mu sync.Mutex

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
		priority := pr.priority
		label := pr.label
		_ = poolx.DefaultPool().SubmitWithOptions(func() {
			mu.Lock()
			order = append(order, label)
			mu.Unlock()
		}, poolx.WithTaskPriority(priority))
	}

	close(blocker)
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("执行顺序 (高优先级先执行): %v\n", order)
	fmt.Println()
}

// asyncAwaitExample 演示 Async/Await 模式
func asyncAwaitExample() {
	fmt.Println("--- 示例 3: Async/Await 模式 ---")

	// Async: 异步启动任务
	f1 := poolx.Async(func() (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "结果 1", nil
	})

	f2 := poolx.Async(func() (string, error) {
		time.Sleep(20 * time.Millisecond)
		return "结果 2", nil
	})

	f3 := poolx.Async(func() (string, error) {
		time.Sleep(5 * time.Millisecond)
		return "结果 3", nil
	})

	// Await 第一个完成的
	result, idx, err := poolx.AwaitFirst(f1, f2, f3)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
	} else {
		fmt.Printf("第一个结果: %q (索引: %d)\n", result, idx)
	}

	// Await 所有完成
	results, err := poolx.AwaitAll(f1, f2, f3)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
	} else {
		fmt.Printf("所有结果: %v\n", results)
	}
	fmt.Println()
}

// contextCancellationExample 演示 Context 取消
func contextCancellationExample() {
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
	future := poolx.SubmitFuncCtx(p, ctx, func(ctx context.Context) (string, error) {
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
	if err != nil {
		fmt.Printf("任务已取消: %v\n", err)
	} else {
		fmt.Printf("结果: %s\n", result)
	}
	fmt.Println()
}

// metricsMonitoringExample 演示指标采集
func metricsMonitoringExample() {
	fmt.Println("--- 示例 5: 指标监控 ---")

	p := poolx.New("metrics-pool",
		poolx.WithMaxWorkers(4),
		poolx.WithAutoScale(false),
		poolx.WithNonBlocking(true),
	)
	defer p.Release()

	// 提交各种任务
	for i := 0; i < 100; i++ {
		_ = p.Submit(func() {
			time.Sleep(time.Millisecond)
		})
	}

	// 提交一个会 panic 的任务
	_ = p.Submit(func() {
		panic("测试 panic")
	})

	// 池满时尝试提交（可能被拒绝）
	for i := 0; i < 10; i++ {
		_ = p.Submit(func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	time.Sleep(200 * time.Millisecond)

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
func globalPoolExample() {
	fmt.Println("--- 示例 7: 全局池 ---")

	var counter atomic.Int32

	// 使用 Go() 简单异步执行
	poolx.Go(func() {
		counter.Add(1)
	})

	// 使用 GoCtx() 带 context 执行
	ctx := context.Background()
	err := poolx.GoCtx(ctx, func() {
		counter.Add(1)
	})
	if err != nil {
		fmt.Printf("GoCtx 错误: %v\n", err)
	}

	time.Sleep(50 * time.Millisecond)
	fmt.Printf("全局池执行了 %d 个任务\n", counter.Load())

	// 调整全局池容量
	poolx.SetCap(100)
	fmt.Printf("默认池容量: %d\n", poolx.DefaultPool().Cap())
	fmt.Println()
}
