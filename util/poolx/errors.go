package poolx

import (
	"errors"
	"fmt"
)

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

	// ErrInvalidConfig 表示构造器收到无效配置。
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrQueueFull indicates the task queue is full
	ErrQueueFull = errors.New("queue is full")

	// ErrNoWorkerAvailable indicates no worker is available
	ErrNoWorkerAvailable = errors.New("no worker available")

	// ErrFutureCanceled indicates the future was canceled
	ErrFutureCanceled = errors.New("future canceled")

	// ErrFutureTimeout indicates the future get operation timed out
	ErrFutureTimeout = errors.New("future get timed out")

	// ErrTaskPanic 表示任务执行期间发生 panic。
	ErrTaskPanic = errors.New("task panicked")
)

// newTaskPanicError 保留 panic 值，同时提供稳定的 errors.Is 判定入口。
func newTaskPanicError(value any) error {
	return fmt.Errorf("%w: %v", ErrTaskPanic, value)
}

// invalidArgumentError 为公开入口生成一致的参数错误。
func invalidArgumentError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidArg, message)
}

// invalidConfigurationError 为构造器生成一致的配置错误。
func invalidConfigurationError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, message)
}
