// Package poolx 提供高性能 goroutine 池
//
// 支持任务窃取、自动伸缩、优先级队列和 Future/Promise 模式。
//
// 简单用法:
//
//	p := poolx.NewSimple(4)  // 4 个工作协程
//	defer p.Release()
//	p.Submit(func() { /* 任务 */ })
//
// 自动伸缩:
//
//	p := poolx.NewAuto(10, 100)  // 最少 10，最多 100 个工作协程
//
// Future 模式:
//
//	future := poolx.SubmitFunc(p, func() (int, error) {
//	    return compute(), nil
//	})
//	result, err := future.Get()
//
// 全局默认池:
//
//	poolx.Go(func() { /* 任务 */ })
//	poolx.GoCtx(ctx, func() { /* 任务 */ })
//
// --- English ---
//
// Package poolx provides a high-performance goroutine pool.
//
// Features work stealing, auto-scaling, priority queues, and Future/Promise pattern.
//
// Simple usage:
//
//	p := poolx.NewSimple(4)  // 4 workers
//	defer p.Release()
//	p.Submit(func() { /* task */ })
//
// With auto-scaling:
//
//	p := poolx.NewAuto(10, 100)  // min 10, max 100 workers
//
// Future pattern:
//
//	future := poolx.SubmitFunc(p, func() (int, error) {
//	    return compute(), nil
//	})
//	result, err := future.Get()
//
// Global default pool:
//
//	poolx.Go(func() { /* task */ })
//	poolx.GoCtx(ctx, func() { /* task */ })
package poolx
