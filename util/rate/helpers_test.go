package rate

import (
	"testing"
	"time"
)

func mustTokenBucket(t *testing.T, capacity int, refillRate float64) *TokenBucket {
	t.Helper()
	limiter, err := NewTokenBucket(capacity, refillRate)
	if err != nil {
		t.Fatalf("NewTokenBucket: %v", err)
	}
	return limiter
}

func mustLeakyBucket(t *testing.T, capacity int, interval time.Duration) *LeakyBucket {
	t.Helper()
	limiter, err := NewLeakyBucket(capacity, interval)
	if err != nil {
		t.Fatalf("NewLeakyBucket: %v", err)
	}
	return limiter
}

func mustSlidingWindow(t *testing.T, capacity int, window time.Duration) *SlidingWindow {
	t.Helper()
	limiter, err := NewSlidingWindow(capacity, window)
	if err != nil {
		t.Fatalf("NewSlidingWindow: %v", err)
	}
	return limiter
}

func mustTokenRateLimiter(t *testing.T, tokensPerMinute, requestsPerMinute int) *TokenRateLimiter {
	t.Helper()
	limiter, err := NewTokenRateLimiter(tokensPerMinute, requestsPerMinute)
	if err != nil {
		t.Fatalf("NewTokenRateLimiter: %v", err)
	}
	return limiter
}

func mustTokenBucketV2(t *testing.T, capacity int, refillRate float64) *TokenBucketV2 {
	t.Helper()
	limiter, err := NewTokenBucketV2(capacity, refillRate)
	if err != nil {
		t.Fatalf("NewTokenBucketV2: %v", err)
	}
	return limiter
}

func mustMultiDimensionLimiter(t *testing.T, limiters ...LimiterV2) *MultiDimensionLimiter {
	t.Helper()
	limiter, err := NewMultiDimensionLimiter(limiters...)
	if err != nil {
		t.Fatalf("NewMultiDimensionLimiter: %v", err)
	}
	return limiter
}
