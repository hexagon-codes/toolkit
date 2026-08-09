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
//	if err := poolx.Go(func() { /* 任务 */ }); err != nil {
//	    handle(err) // 处理提交错误
//	} // 结束错误分支
//	if err := poolx.GoCtx(ctx, func(ctx context.Context) { /* 协作式任务 */ }); err != nil {
//	    handle(err) // 处理提交错误
//	} // 结束错误分支
package poolx
