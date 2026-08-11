package lease

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

func TestMemoryLeaseHonorsCanceledContextWithoutMutation(t *testing.T) {
	lease := NewMemoryLease()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	if token, acquired, err := lease.Acquire(canceled, "key", time.Second); token != 0 || acquired || !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(canceled) = (%d, %v, %v), want zero false context.Canceled", token, acquired, err)
	}
	token, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if err != nil || !acquired || token != 1 {
		t.Fatalf("Acquire() after canceled call = (%d, %v, %v), want 1 true nil", token, acquired, err)
	}
	if err := lease.Release(canceled, "key", token); !errors.Is(err, context.Canceled) {
		t.Fatalf("Release(canceled) error = %v, want context.Canceled", err)
	}
	if _, acquired, err := lease.Acquire(context.Background(), "key", time.Second); err != nil || acquired {
		t.Fatalf("canceled Release changed ownership: acquired=%v err=%v", acquired, err)
	}
	if err := lease.Refresh(canceled, "key", token, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh(canceled) error = %v, want context.Canceled", err)
	}
}

func TestMemoryLeaseRejectsInvalidInput(t *testing.T) {
	lease := NewMemoryLease()
	ctx := context.Background()
	if _, _, err := lease.Acquire(nil, "key", time.Second); !errors.Is(err, ErrNilContext) {
		t.Fatalf("Acquire(nil) error = %v, want ErrNilContext", err)
	}
	if _, _, err := lease.Acquire(ctx, "", time.Second); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Acquire(empty key) error = %v, want ErrInvalidKey", err)
	}
	for _, ttl := range []time.Duration{0, -time.Nanosecond} {
		if _, _, err := lease.Acquire(ctx, "key", ttl); !errors.Is(err, ErrInvalidTTL) {
			t.Fatalf("Acquire(ttl=%s) error = %v, want ErrInvalidTTL", ttl, err)
		}
		if err := lease.Refresh(ctx, "key", 1, ttl); !errors.Is(err, ErrInvalidTTL) {
			t.Fatalf("Refresh(ttl=%s) error = %v, want ErrInvalidTTL", ttl, err)
		}
	}
	if err := lease.Release(nil, "key", 1); !errors.Is(err, ErrNilContext) {
		t.Fatalf("Release(nil) error = %v, want ErrNilContext", err)
	}
	if err := lease.Release(ctx, "", 1); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Release(empty key) error = %v, want ErrInvalidKey", err)
	}
}

func TestMemoryLeaseRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name      string
		construct func()
		want      error
	}{
		{name: "nil clock", construct: func() { _ = WithClock(nil) }, want: ErrInvalidClock},
		{name: "nil option", construct: func() { _ = NewMemoryLease(nil) }, want: ErrInvalidOption},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatalf("construct did not panic with %v", test.want)
				}
				err, ok := recovered.(error)
				if !ok || !errors.Is(err, test.want) {
					t.Fatalf("panic = %v, want %v", recovered, test.want)
				}
			}()
			test.construct()
		})
	}
}

func TestMemoryLeaseCannotRefreshExpiredGeneration(t *testing.T) {
	current := time.Unix(1000, 0)
	lease := NewMemoryLease(WithClock(func() time.Time { return current }))
	token, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if err != nil || !acquired {
		t.Fatalf("Acquire() = (%d, %v, %v)", token, acquired, err)
	}

	current = current.Add(time.Second)
	if err := lease.Refresh(context.Background(), "key", token, time.Second); !errors.Is(err, ErrNotHolder) {
		t.Fatalf("Refresh() at expiry error = %v, want ErrNotHolder", err)
	}
	next, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if err != nil || !acquired || next <= token {
		t.Fatalf("Acquire() after expiry = (%d, %v, %v), previous=%d", next, acquired, err, token)
	}
}

func TestMemoryLeaseStaleGenerationCannotReleaseNewHolder(t *testing.T) {
	current := time.Unix(1000, 0)
	lease := NewMemoryLease(WithClock(func() time.Time { return current }))
	stale, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if err != nil || !acquired {
		t.Fatalf("first Acquire() = (%d, %v, %v)", stale, acquired, err)
	}
	current = current.Add(time.Second)
	currentToken, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if err != nil || !acquired || currentToken <= stale {
		t.Fatalf("second Acquire() = (%d, %v, %v)", currentToken, acquired, err)
	}
	if err := lease.Release(context.Background(), "key", stale); err != nil {
		t.Fatalf("stale Release() error = %v", err)
	}
	if _, acquired, err := lease.Acquire(context.Background(), "key", time.Second); err != nil || acquired {
		t.Fatalf("stale generation released current holder: acquired=%v err=%v", acquired, err)
	}
}

func TestMemoryLeaseRejectsFencingTokenWraparound(t *testing.T) {
	lease := NewMemoryLease()
	lease.counter = FencingToken(math.MaxUint64)
	token, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if token != 0 || acquired || !errors.Is(err, ErrFencingTokenExhausted) {
		t.Fatalf("Acquire() at fencing limit = (%d, %v, %v), want zero false ErrFencingTokenExhausted", token, acquired, err)
	}
}

func TestMemoryLeaseZeroValueIsUsable(t *testing.T) {
	var lease MemoryLease
	token, acquired, err := lease.Acquire(context.Background(), "key", time.Second)
	if err != nil || !acquired || token != 1 {
		t.Fatalf("zero-value Acquire() = (%d, %v, %v), want 1 true nil", token, acquired, err)
	}
}

func TestMemoryLeaseConcurrentAcquireHasSingleWinner(t *testing.T) {
	lease := NewMemoryLease()
	const contenders = 64
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var waitGroup sync.WaitGroup
	waitGroup.Add(contenders)
	for range contenders {
		go func() {
			defer waitGroup.Done()
			<-start
			_, acquired, err := lease.Acquire(context.Background(), "key", time.Minute)
			if err != nil {
				t.Errorf("Acquire() error = %v", err)
			}
			results <- acquired
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	winners := 0
	for acquired := range results {
		if acquired {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("acquired count = %d, want 1", winners)
	}
}
