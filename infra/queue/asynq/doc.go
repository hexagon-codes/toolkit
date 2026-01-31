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
