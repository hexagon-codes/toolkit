package rate

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

func TestConstructorsRejectInvalidConfiguration(t *testing.T) {
	t.Run("token bucket capacity", func(t *testing.T) {
		limiter, err := NewTokenBucket(0, 1)
		if limiter != nil || !errors.Is(err, ErrInvalidCapacity) {
			t.Fatalf("NewTokenBucket = (%v, %v), want nil ErrInvalidCapacity", limiter, err)
		}
	})
	t.Run("token bucket rate", func(t *testing.T) {
		limiter, err := NewTokenBucket(1, math.NaN())
		if limiter != nil || !errors.Is(err, ErrInvalidRate) {
			t.Fatalf("NewTokenBucket = (%v, %v), want nil ErrInvalidRate", limiter, err)
		}
	})
	t.Run("sliding window capacity", func(t *testing.T) {
		limiter, err := NewSlidingWindow(-1, time.Second)
		if limiter != nil || !errors.Is(err, ErrInvalidCapacity) {
			t.Fatalf("NewSlidingWindow = (%v, %v), want nil ErrInvalidCapacity", limiter, err)
		}
	})
	t.Run("sliding window duration", func(t *testing.T) {
		limiter, err := NewSlidingWindow(1, 0)
		if limiter != nil || !errors.Is(err, ErrInvalidWindow) {
			t.Fatalf("NewSlidingWindow = (%v, %v), want nil ErrInvalidWindow", limiter, err)
		}
	})
	t.Run("token limiter capacity", func(t *testing.T) {
		limiter, err := NewTokenRateLimiter(1, 0)
		if limiter != nil || !errors.Is(err, ErrInvalidCapacity) {
			t.Fatalf("NewTokenRateLimiter = (%v, %v), want nil ErrInvalidCapacity", limiter, err)
		}
	})
	t.Run("enhanced token bucket rate", func(t *testing.T) {
		limiter, err := NewTokenBucketV2(1, math.Inf(1))
		if limiter != nil || !errors.Is(err, ErrInvalidRate) {
			t.Fatalf("NewTokenBucketV2 = (%v, %v), want nil ErrInvalidRate", limiter, err)
		}
	})
}

type nonTransactionalLimiter struct{}

func (nonTransactionalLimiter) Allow() bool { return true }

func (nonTransactionalLimiter) Wait() time.Duration { return 0 }

func (nonTransactionalLimiter) AllowN(int) bool { return true }

func (nonTransactionalLimiter) WaitN(context.Context, int) error { return nil }

func (nonTransactionalLimiter) Available() int { return 1 }

func TestMultiDimensionLimiterRejectsNonAtomicConfiguration(t *testing.T) {
	if limiter, err := NewMultiDimensionLimiter(); limiter != nil || !errors.Is(err, ErrNoLimiters) {
		t.Fatalf("empty constructor = (%v, %v), want nil ErrNoLimiters", limiter, err)
	}

	var nilBucket *TokenBucketV2
	if limiter, err := NewMultiDimensionLimiter(nilBucket); limiter != nil || !errors.Is(err, ErrNilLimiter) {
		t.Fatalf("typed nil constructor = (%v, %v), want nil ErrNilLimiter", limiter, err)
	}

	if limiter, err := NewMultiDimensionLimiter(nonTransactionalLimiter{}); limiter != nil || !errors.Is(err, ErrUnsupportedLimiter) {
		t.Fatalf("unsupported constructor = (%v, %v), want nil ErrUnsupportedLimiter", limiter, err)
	}

	bucket := mustTokenBucketV2(t, 1, 1)
	if limiter, err := NewMultiDimensionLimiter(bucket, bucket); limiter != nil || !errors.Is(err, ErrDuplicateLimiter) {
		t.Fatalf("duplicate constructor = (%v, %v), want nil ErrDuplicateLimiter", limiter, err)
	}

	var zeroBucket TokenBucketV2
	if limiter, err := NewMultiDimensionLimiter(&zeroBucket); limiter != nil || !errors.Is(err, ErrUninitializedLimiter) {
		t.Fatalf("zero-value constructor = (%v, %v), want nil ErrUninitializedLimiter", limiter, err)
	}
}

func TestMultiDimensionLimiterUsesStableLockOrdering(t *testing.T) {
	first := mustTokenBucketV2(t, 2000, 0.000001)
	second := mustTokenBucketV2(t, 2000, 0.000001)
	forward := mustMultiDimensionLimiter(t, first, second)
	reverse := mustMultiDimensionLimiter(t, second, first)

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for _, limiter := range []*MultiDimensionLimiter{forward, reverse} {
		limiter := limiter
		go func() {
			defer waitGroup.Done()
			for range 500 {
				limiter.Allow()
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("opposite limiter order deadlocked")
	}
}
