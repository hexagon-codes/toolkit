package poolx

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Future[T] - Generic Result Pattern
// ============================================================================

// FutureState represents the state of a Future
type FutureState int32

const (
	// FutureStatePending indicates the task has not completed yet
	FutureStatePending FutureState = iota
	// FutureStateCompleted indicates the task completed successfully
	FutureStateCompleted
	// FutureStateFailed indicates the task failed with an error
	FutureStateFailed
	// FutureStateCanceled indicates the future was canceled
	FutureStateCanceled
)

// String returns the string representation of the state
func (s FutureState) String() string {
	switch s {
	case FutureStatePending:
		return "Pending"
	case FutureStateCompleted:
		return "Completed"
	case FutureStateFailed:
		return "Failed"
	case FutureStateCanceled:
		return "Canceled"
	default:
		return "Unknown"
	}
}

// Future represents an asynchronous computation result.
// It provides a way to retrieve the result of a task that runs in a goroutine pool.
type Future[T any] struct {
	state    atomic.Int32
	result   T
	err      error
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	cancelFn context.CancelFunc
}

// NewFuture creates a new Future in pending state
func NewFuture[T any]() *Future[T] {
	return &Future[T]{
		done: make(chan struct{}),
	}
}

// Complete sets the result and marks the future as completed.
// This can only be called once; subsequent calls are ignored.
func (f *Future[T]) Complete(result T) {
	f.once.Do(func() {
		f.mu.Lock()
		f.result = result
		f.state.Store(int32(FutureStateCompleted))
		f.mu.Unlock()
		close(f.done)
	})
}

// Fail marks the future as failed with the given error.
// This can only be called once; subsequent calls are ignored.
func (f *Future[T]) Fail(err error) {
	f.once.Do(func() {
		f.mu.Lock()
		f.err = err
		f.state.Store(int32(FutureStateFailed))
		f.mu.Unlock()
		close(f.done)
	})
}

// Cancel cancels the future.
// This can only be called once; subsequent calls are ignored.
func (f *Future[T]) Cancel() {
	f.once.Do(func() {
		f.mu.Lock()
		f.err = ErrFutureCanceled
		f.state.Store(int32(FutureStateCanceled))
		if f.cancelFn != nil {
			f.cancelFn()
		}
		f.mu.Unlock()
		close(f.done)
	})
}

// Get blocks until the future is complete and returns the result.
// Returns an error if the future failed or was canceled.
func (f *Future[T]) Get() (T, error) {
	<-f.done
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.result, f.err
}

// GetWithTimeout blocks until the future completes or times out.
// Returns ErrFutureTimeout if the timeout expires.
func (f *Future[T]) GetWithTimeout(timeout time.Duration) (T, error) {
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.result, f.err
	case <-time.After(timeout):
		var zero T
		return zero, ErrFutureTimeout
	}
}

// GetWithContext blocks until the future completes or context is canceled.
func (f *Future[T]) GetWithContext(ctx context.Context) (T, error) {
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.result, f.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// IsDone returns true if the future has completed (success, failure, or canceled).
func (f *Future[T]) IsDone() bool {
	return FutureState(f.state.Load()) != FutureStatePending
}

// IsCompleted returns true if the future completed successfully.
func (f *Future[T]) IsCompleted() bool {
	return FutureState(f.state.Load()) == FutureStateCompleted
}

// IsFailed returns true if the future failed with an error.
func (f *Future[T]) IsFailed() bool {
	return FutureState(f.state.Load()) == FutureStateFailed
}

// IsCanceled returns true if the future was canceled.
func (f *Future[T]) IsCanceled() bool {
	return FutureState(f.state.Load()) == FutureStateCanceled
}

// State returns the current state of the future.
func (f *Future[T]) State() FutureState {
	return FutureState(f.state.Load())
}

// Done returns a channel that is closed when the future completes.
func (f *Future[T]) Done() <-chan struct{} {
	return f.done
}

// ============================================================================
// Helper Functions for Creating Futures
// ============================================================================

// SubmitFunc submits a function that returns a result to the pool.
// Returns a Future that can be used to retrieve the result.
func SubmitFunc[T any](p *Pool, fn func() (T, error)) *Future[T] {
	future := NewFuture[T]()

	err := p.Submit(func() {
		result, err := fn()
		if err != nil {
			future.Fail(err)
		} else {
			future.Complete(result)
		}
	})

	if err != nil {
		future.Fail(err)
	}

	return future
}

// SubmitFuncCtx submits a function with context support.
// The context is passed to the function and can be used for cancellation.
func SubmitFuncCtx[T any](p *Pool, ctx context.Context, fn func(context.Context) (T, error)) *Future[T] {
	future := NewFuture[T]()

	// Create a child context that can be canceled
	childCtx, cancel := context.WithCancel(ctx)
	future.cancelFn = cancel

	err := p.SubmitWithContext(ctx, func() {
		// Check if already canceled
		select {
		case <-childCtx.Done():
			future.Fail(childCtx.Err())
			return
		default:
		}

		result, err := fn(childCtx)
		if err != nil {
			future.Fail(err)
		} else {
			future.Complete(result)
		}
	})

	if err != nil {
		cancel()
		future.Fail(err)
	}

	return future
}

// TrySubmitFunc attempts to submit a function without blocking.
// Returns nil if no worker is available.
func TrySubmitFunc[T any](p *Pool, fn func() (T, error)) *Future[T] {
	future := NewFuture[T]()

	ok := p.TrySubmit(func() {
		result, err := fn()
		if err != nil {
			future.Fail(err)
		} else {
			future.Complete(result)
		}
	})

	if !ok {
		return nil
	}

	return future
}

// ============================================================================
// FutureGroup - Wait for Multiple Futures
// ============================================================================

// FutureGroup allows waiting for multiple futures to complete
type FutureGroup[T any] struct {
	futures []*Future[T]
	mu      sync.Mutex
}

// NewFutureGroup creates a new FutureGroup
func NewFutureGroup[T any]() *FutureGroup[T] {
	return &FutureGroup[T]{}
}

// Add adds a future to the group
func (g *FutureGroup[T]) Add(f *Future[T]) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.futures = append(g.futures, f)
}

// Wait blocks until all futures complete and returns all results.
// Returns the first error encountered, if any.
func (g *FutureGroup[T]) Wait() ([]T, error) {
	g.mu.Lock()
	futures := make([]*Future[T], len(g.futures))
	copy(futures, g.futures)
	g.mu.Unlock()

	results := make([]T, len(futures))
	var firstErr error

	for i, f := range futures {
		result, err := f.Get()
		results[i] = result
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return results, firstErr
}

// WaitWithTimeout waits for all futures with a timeout.
func (g *FutureGroup[T]) WaitWithTimeout(timeout time.Duration) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return g.WaitWithContext(ctx)
}

// WaitWithContext waits for all futures with context cancellation.
func (g *FutureGroup[T]) WaitWithContext(ctx context.Context) ([]T, error) {
	g.mu.Lock()
	futures := make([]*Future[T], len(g.futures))
	copy(futures, g.futures)
	g.mu.Unlock()

	results := make([]T, len(futures))
	var firstErr error

	for i, f := range futures {
		result, err := f.GetWithContext(ctx)
		results[i] = result
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return results, firstErr
}

// AllCompleted returns true if all futures have completed.
func (g *FutureGroup[T]) AllCompleted() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, f := range g.futures {
		if !f.IsDone() {
			return false
		}
	}
	return true
}

// AnyFailed returns true if any future has failed.
func (g *FutureGroup[T]) AnyFailed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, f := range g.futures {
		if f.IsFailed() {
			return true
		}
	}
	return false
}

// Count returns the number of futures in the group.
func (g *FutureGroup[T]) Count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.futures)
}

// ============================================================================
// Promise - Writable side of a Future
// ============================================================================

// Promise provides the writable side of a Future.
// Use this when you need to separate the read and write sides.
type Promise[T any] struct {
	future *Future[T]
}

// NewPromise creates a new Promise with its associated Future.
func NewPromise[T any]() (*Promise[T], *Future[T]) {
	future := NewFuture[T]()
	promise := &Promise[T]{future: future}
	return promise, future
}

// Complete sets the successful result.
func (p *Promise[T]) Complete(result T) {
	p.future.Complete(result)
}

// Fail sets the error result.
func (p *Promise[T]) Fail(err error) {
	p.future.Fail(err)
}

// Future returns the associated Future.
func (p *Promise[T]) Future() *Future[T] {
	return p.future
}

// ============================================================================
// Convenience Functions
// ============================================================================

// Async executes a function asynchronously and returns a Future.
// Uses the default pool.
func Async[T any](fn func() (T, error)) *Future[T] {
	initDefaultPool()
	return SubmitFunc(defaultPool, fn)
}

// AsyncCtx executes a function asynchronously with context.
// Uses the default pool.
func AsyncCtx[T any](ctx context.Context, fn func(context.Context) (T, error)) *Future[T] {
	initDefaultPool()
	return SubmitFuncCtx(defaultPool, ctx, fn)
}

// Await waits for a future and returns its result.
// This is a convenience function equivalent to f.Get().
func Await[T any](f *Future[T]) (T, error) {
	return f.Get()
}

// AwaitAll waits for multiple futures and returns all results.
func AwaitAll[T any](futures ...*Future[T]) ([]T, error) {
	results := make([]T, len(futures))
	var firstErr error

	for i, f := range futures {
		result, err := f.Get()
		results[i] = result
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return results, firstErr
}

// AwaitFirst waits for the first future to complete and returns its result.
func AwaitFirst[T any](futures ...*Future[T]) (T, int, error) {
	if len(futures) == 0 {
		var zero T
		return zero, -1, ErrInvalidArg
	}

	// Create a channel to receive the first result
	type result struct {
		value T
		index int
		err   error
	}

	resultCh := make(chan result, 1)

	for i, f := range futures {
		go func(idx int, future *Future[T]) {
			val, err := future.Get()
			select {
			case resultCh <- result{value: val, index: idx, err: err}:
			default:
			}
		}(i, f)
	}

	r := <-resultCh
	return r.value, r.index, r.err
}

// AwaitAny waits for any future to complete successfully.
// Returns the first successful result, or the last error if all fail.
func AwaitAny[T any](futures ...*Future[T]) (T, int, error) {
	if len(futures) == 0 {
		var zero T
		return zero, -1, ErrInvalidArg
	}

	type result struct {
		value T
		index int
		err   error
	}

	resultCh := make(chan result, len(futures))

	for i, f := range futures {
		go func(idx int, future *Future[T]) {
			val, err := future.Get()
			resultCh <- result{value: val, index: idx, err: err}
		}(i, f)
	}

	var lastErr error
	for i := 0; i < len(futures); i++ {
		r := <-resultCh
		if r.err == nil {
			return r.value, r.index, nil
		}
		lastErr = r.err
	}

	var zero T
	return zero, -1, lastErr
}
