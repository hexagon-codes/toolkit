package contextx

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type auditAfterContext struct {
	context.Context
	stopOnce sync.Once
	stopped  chan struct{}
	done     chan struct{}
}

func (c *auditAfterContext) Done() <-chan struct{} { return c.done }

func (c *auditAfterContext) AfterFunc(func()) func() bool {
	return func() bool {
		stopped := false
		c.stopOnce.Do(func() {
			close(c.stopped)
			stopped = true
		})
		return stopped
	}
}

func TestMergePreservesCauseIgnoresNilAndCleansRegistrations(t *testing.T) {
	source, cancelSource := context.WithCancelCause(context.Background())
	tracked := &auditAfterContext{Context: context.Background(), stopped: make(chan struct{}), done: make(chan struct{})}
	merged, cancelMerged := Merge(nil, source, tracked)
	defer cancelMerged()

	cause := errors.New("source failed")
	cancelSource(cause)
	select {
	case <-merged.Done():
	case <-time.After(time.Second):
		t.Fatal("merged context did not observe source cancellation")
	}
	if !errors.Is(context.Cause(merged), cause) {
		t.Fatalf("merged cause = %v, want %v", context.Cause(merged), cause)
	}
	select {
	case <-tracked.stopped:
	case <-time.After(time.Second):
		t.Fatal("merged context retained another source registration after cancellation")
	}
}

func TestMergeExposesEarliestDeadline(t *testing.T) {
	source, cancelSource := context.WithTimeout(context.Background(), time.Hour)
	defer cancelSource()
	merged, cancelMerged := Merge(context.Background(), source)
	defer cancelMerged()
	want, _ := source.Deadline()
	got, ok := merged.Deadline()
	if !ok || !got.Equal(want) {
		t.Fatalf("merged deadline = (%v, %v), want (%v, true)", got, ok, want)
	}
}

func TestRunPreservesTaskAndCancellationErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	taskErr := errors.New("task failed")
	err := Run(ctx, func(context.Context) error {
		cancel()
		return taskErr
	})
	if !errors.Is(err, taskErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want both task and cancellation errors", err)
	}
}

func TestAsyncAPIsRejectNilTasks(t *testing.T) {
	if err := Go(context.Background(), nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("Go(nil) error = %v, want %v", err, ErrNilTask)
	}
	group := NewWaitGroupContext(context.Background())
	if err := group.Go(nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("WaitGroupContext.Go(nil) error = %v, want %v", err, ErrNilTask)
	}
	pool, err := NewPool(context.Background(), 1)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	defer pool.Close()
	if err := pool.Go(nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("Pool.Go(nil) error = %v, want %v", err, ErrNilTask)
	}
}

func TestPoolClosesSubmissionAtWaitBoundary(t *testing.T) {
	pool, err := NewPool(context.Background(), 1)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	if err := pool.Go(func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Pool.Go() error = %v", err)
	}
	if err := pool.Wait(); err != nil {
		t.Fatalf("Pool.Wait() error = %v", err)
	}
	if err := pool.Go(func(context.Context) error { return nil }); err == nil || err.Error() != "contextx: pool is closed" {
		t.Fatalf("Pool.Go() after Wait error = %v, want closed error", err)
	}
}

func TestPoolWaitPreservesTaskAndCancellationErrors(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	pool, err := NewPool(parent, 1)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	taskErr := errors.New("pool task failed")
	if submitErr := pool.Go(func(context.Context) error {
		cancel()
		return taskErr
	}); submitErr != nil {
		t.Fatalf("Pool.Go() error = %v", submitErr)
	}
	err = pool.Wait()
	if !errors.Is(err, taskErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Pool.Wait() error = %v, want task and cancellation errors", err)
	}
}
