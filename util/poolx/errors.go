package poolx

import "errors"

// ============================================================================
// Error definitions
// ============================================================================

var (
	// ErrPoolClosed indicates the pool has been closed
	ErrPoolClosed = errors.New("pool is closed")

	// ErrPoolOverload indicates the pool is overloaded and cannot accept more tasks
	ErrPoolOverload = errors.New("pool is overloaded")

	// ErrTimeout indicates the operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrTaskRejected indicates the task was rejected
	ErrTaskRejected = errors.New("task rejected")

	// ErrInvalidArg indicates an invalid argument was provided
	ErrInvalidArg = errors.New("invalid argument")

	// ErrQueueFull indicates the task queue is full
	ErrQueueFull = errors.New("queue is full")

	// ErrNoWorkerAvailable indicates no worker is available
	ErrNoWorkerAvailable = errors.New("no worker available")

	// ErrFutureCanceled indicates the future was canceled
	ErrFutureCanceled = errors.New("future canceled")

	// ErrFutureTimeout indicates the future get operation timed out
	ErrFutureTimeout = errors.New("future get timed out")
)
