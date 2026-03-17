// Package asynq 提供基于 hibiken/asynq 的异步任务队列
//
// 支持任务状态机、带退避策略的重试和死信队列。
//
// 基本用法:
//
//	mgr, _ := asynq.NewManager(&asynq.Config{
//	    RedisAddrs: []string{"localhost:6379"},
//	})
//	defer mgr.Close()
//
//	// 入队任务
//	task := asynq.NewTask("email:send", payload)
//	mgr.Enqueue(ctx, task)
//
// 任务处理器:
//
//	mux := asynq.NewServeMux()
//	mux.HandleFunc("email:send", func(ctx context.Context, task *asynq.Task) error {
//	    // 处理任务
//	    return nil
//	})
//
// 状态机:
//
//	sm := asynq.GetTaskStateMachine()
//	sm.CanTransition(asynq.StatePending, asynq.EventStart)
//
// --- English ---
//
// Package asynq provides an async task queue based on hibiken/asynq.
//
// Features task state machine, retry with backoff, and dead letter queue.
//
// Basic usage:
//
//	mgr, _ := asynq.NewManager(&asynq.Config{
//	    RedisAddrs: []string{"localhost:6379"},
//	})
//	defer mgr.Close()
//
//	// Enqueue a task
//	task := asynq.NewTask("email:send", payload)
//	mgr.Enqueue(ctx, task)
//
// Task handler:
//
//	mux := asynq.NewServeMux()
//	mux.HandleFunc("email:send", func(ctx context.Context, task *asynq.Task) error {
//	    // process task
//	    return nil
//	})
//
// State machine:
//
//	sm := asynq.GetTaskStateMachine()
//	sm.CanTransition(asynq.StatePending, asynq.EventStart)
package asynq
