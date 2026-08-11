package rate

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestLegacyLimitersWaitContextHonorsCancellation(t *testing.T) {
	tokenBucket := mustTokenBucket(t, 1, 1e-9)
	leakyBucket := mustLeakyBucket(t, 1, time.Hour)
	slidingWindow := mustSlidingWindow(t, 1, time.Hour)
	for name, limiter := range map[string]struct {
		consume func() bool
		wait    func(context.Context) (time.Duration, error)
	}{
		"token bucket":   {consume: tokenBucket.Allow, wait: tokenBucket.WaitContext},
		"leaky bucket":   {consume: leakyBucket.Allow, wait: leakyBucket.WaitContext},
		"sliding window": {consume: slidingWindow.Allow, wait: slidingWindow.WaitContext},
	} {
		t.Run(name, func(t *testing.T) {
			if !limiter.consume() {
				t.Fatal("initial capacity was not available")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if waited, err := limiter.wait(ctx); waited != 0 || !errors.Is(err, context.Canceled) {
				t.Fatalf("WaitContext(canceled) = (%s, %v), want zero context.Canceled", waited, err)
			}
			if limiter.consume() {
				t.Fatal("canceled wait consumed or created capacity")
			}
		})
	}
}

func TestLegacyLimitersRejectNilContextAndZeroValue(t *testing.T) {
	var tokenBucket TokenBucket
	var leakyBucket LeakyBucket
	var slidingWindow SlidingWindow
	for name, limiter := range map[string]struct {
		allow func() bool
		wait  func(context.Context) (time.Duration, error)
	}{
		"token bucket":   {allow: tokenBucket.Allow, wait: tokenBucket.WaitContext},
		"leaky bucket":   {allow: leakyBucket.Allow, wait: leakyBucket.WaitContext},
		"sliding window": {allow: slidingWindow.Allow, wait: slidingWindow.WaitContext},
	} {
		t.Run(name, func(t *testing.T) {
			if limiter.allow() {
				t.Fatal("zero-value limiter allowed a request")
			}
			if _, err := limiter.wait(nil); !errors.Is(err, ErrNilContext) {
				t.Fatalf("WaitContext(nil) error = %v, want ErrNilContext", err)
			}
			if _, err := limiter.wait(context.Background()); !errors.Is(err, ErrUninitializedLimiter) {
				t.Fatalf("zero-value WaitContext() error = %v, want ErrUninitializedLimiter", err)
			}
		})
	}
}

func TestV2LimitersZeroValueFailsClosed(t *testing.T) {
	var tokenBucket TokenBucketV2
	if tokenBucket.Allow() || tokenBucket.Available() != 0 {
		t.Fatal("zero-value TokenBucketV2 did not fail closed")
	}
	if err := tokenBucket.WaitN(context.Background(), 1); !errors.Is(err, ErrUninitializedLimiter) {
		t.Fatalf("zero-value TokenBucketV2 WaitN() error = %v, want ErrUninitializedLimiter", err)
	}

	var tokenLimiter TokenRateLimiter
	if tokenLimiter.Allow() || tokenLimiter.Available() != 0 {
		t.Fatal("zero-value TokenRateLimiter did not fail closed")
	}
	if err := tokenLimiter.WaitN(context.Background(), 1); !errors.Is(err, ErrUninitializedLimiter) {
		t.Fatalf("zero-value TokenRateLimiter WaitN() error = %v, want ErrUninitializedLimiter", err)
	}

	var multi MultiDimensionLimiter
	if multi.Allow() || multi.Available() != 0 {
		t.Fatal("zero-value MultiDimensionLimiter did not fail closed")
	}
	if err := multi.WaitN(context.Background(), 1); !errors.Is(err, ErrUninitializedLimiter) {
		t.Fatalf("zero-value MultiDimensionLimiter WaitN() error = %v, want ErrUninitializedLimiter", err)
	}
}

func TestTokenBucketsFailClosedOnClockRollback(t *testing.T) {
	tokenBucket := mustTokenBucket(t, 1, 1)
	if !tokenBucket.Allow() {
		t.Fatal("initial token was not available")
	}
	future := time.Now().Add(time.Hour)
	tokenBucket.mu.Lock()
	tokenBucket.lastTime = future
	tokenBucket.tokens = 0
	tokenBucket.mu.Unlock()
	if tokenBucket.Allow() {
		t.Fatal("TokenBucket allowed a request after clock rollback")
	}
	tokenBucket.mu.Lock()
	if tokenBucket.tokens < 0 || !tokenBucket.lastTime.Equal(future) {
		t.Fatalf("TokenBucket rollback state = tokens %f last %s, want non-negative and unchanged", tokenBucket.tokens, tokenBucket.lastTime)
	}
	tokenBucket.mu.Unlock()

	v2 := mustTokenBucketV2(t, 1, 1)
	if !v2.Allow() {
		t.Fatal("initial V2 token was not available")
	}
	v2.mu.Lock()
	v2.lastTime = future
	v2.tokens = 0
	v2.mu.Unlock()
	if v2.Allow() {
		t.Fatal("TokenBucketV2 allowed a request after clock rollback")
	}
	v2.mu.Lock()
	if v2.tokens < 0 || !v2.lastTime.Equal(future) {
		t.Fatalf("TokenBucketV2 rollback state = tokens %f last %s, want non-negative and unchanged", v2.tokens, v2.lastTime)
	}
	v2.mu.Unlock()
}

func TestLeakyAndSlidingWindowsFailClosedOnClockRollback(t *testing.T) {
	leakyBucket := mustLeakyBucket(t, 1, time.Second)
	if !leakyBucket.Allow() {
		t.Fatal("initial leaky-bucket capacity was not available")
	}
	future := time.Now().Add(time.Hour)
	leakyBucket.mu.Lock()
	leakyBucket.lastLeakTime = future
	leakyBucket.mu.Unlock()
	if leakyBucket.Allow() {
		t.Fatal("LeakyBucket allowed a request after clock rollback")
	}
	leakyBucket.mu.Lock()
	if !leakyBucket.lastLeakTime.Equal(future) || leakyBucket.water != 1 {
		t.Fatalf("LeakyBucket rollback state = time %s water %d, want unchanged", leakyBucket.lastLeakTime, leakyBucket.water)
	}
	leakyBucket.mu.Unlock()

	slidingWindow := mustSlidingWindow(t, 1, time.Second)
	slidingWindow.mu.Lock()
	slidingWindow.requests = []time.Time{future}
	slidingWindow.mu.Unlock()
	if slidingWindow.Allow() {
		t.Fatal("SlidingWindow allowed a request after clock rollback")
	}
	if count := slidingWindow.Count(); count != 1 {
		t.Fatalf("SlidingWindow rollback count = %d, want 1", count)
	}
}

func TestTokenBucketWaitContextCancelsExtremeDuration(t *testing.T) {
	tokenBucket := mustTokenBucket(t, 1, math.SmallestNonzeroFloat64)
	if !tokenBucket.Allow() {
		t.Fatal("initial token was not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := tokenBucket.WaitContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitContext() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("WaitContext() cancellation took %s", elapsed)
	}
}

func TestSlidingWindowRecordPreservesExactWindowCount(t *testing.T) {
	slidingWindow := mustSlidingWindow(t, 3, time.Hour)
	for range 500 {
		slidingWindow.Record()
	}
	if count := slidingWindow.Count(); count != 500 {
		t.Fatalf("Count() = %d, want all 500 in-window records", count)
	}
}
