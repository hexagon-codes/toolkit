package contextx

import (
	"context"
	"errors"
	"testing"
)

// Bug3: Pool.Go accepts func(ctx) error but the return value was discarded, and
// Wait() returned only ctx.Err(), silently dropping task errors. Wait() must
// surface the first/joined task error when context was not canceled.
func TestBug3_Pool_WaitSurfacesTaskError(t *testing.T) {
	sentinel := errors.New("task failed")

	p := NewPool(context.Background(), 2)
	p.Go(func(ctx context.Context) error { return nil })
	p.Go(func(ctx context.Context) error { return sentinel })

	err := p.Wait()
	if err == nil {
		t.Fatalf("Wait() = nil, want the task error to be surfaced")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Wait() = %v, want it to wrap %v", err, sentinel)
	}
}

// All-nil tasks must still report success.
func TestBug3_Pool_WaitNilWhenNoTaskError(t *testing.T) {
	p := NewPool(context.Background(), 2)
	p.Go(func(ctx context.Context) error { return nil })
	p.Go(func(ctx context.Context) error { return nil })
	if err := p.Wait(); err != nil {
		t.Fatalf("Wait() = %v, want nil", err)
	}
}
