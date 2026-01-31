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
