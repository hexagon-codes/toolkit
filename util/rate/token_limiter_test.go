package rate

import (
	"context"
	"testing"
	"time"
)

func TestTokenRateLimiter_Allow(t *testing.T) {
	limiter := NewTokenRateLimiter(100, 10) // 100 TPM, 10 RPM

	// 应该允许前几个请求
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}
}

func TestTokenRateLimiter_AllowN(t *testing.T) {
	limiter := NewTokenRateLimiter(100, 10) // 100 TPM, 10 RPM

	// 请求 50 个 token
	if !limiter.AllowN(50) {
		t.Error("should allow 50 tokens")
	}

	// 再请求 50 个 token
	if !limiter.AllowN(50) {
		t.Error("should allow another 50 tokens")
	}

	// 再请求 50 个应该失败（超过 100 TPM）
	if limiter.AllowN(50) {
		t.Error("should not allow exceeding TPM limit")
	}
}

func TestTokenRateLimiter_WaitN(t *testing.T) {
	limiter := NewTokenRateLimiter(100, 100) // 100 TPM, 100 RPM

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 应该立即通过
	err := limiter.WaitN(ctx, 10)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTokenRateLimiter_WaitN_Timeout(t *testing.T) {
	limiter := NewTokenRateLimiter(10, 10) // 很低的限制

	// 先消耗所有 token
	limiter.AllowN(10)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// 应该超时
	err := limiter.WaitN(ctx, 10)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestTokenRateLimiter_Stats(t *testing.T) {
	limiter := NewTokenRateLimiter(100, 10)

	stats := limiter.Stats()
	if stats.TokensPerMinute != 100 {
		t.Errorf("expected TPM 100, got %d", stats.TokensPerMinute)
	}
	if stats.RequestsPerMinute != 10 {
		t.Errorf("expected RPM 10, got %d", stats.RequestsPerMinute)
	}
}

func TestTokenBucketV2_AllowN(t *testing.T) {
	bucket := NewTokenBucketV2(100, 10) // 100 容量，每秒 10 个

	// 应该允许 50 个
	if !bucket.AllowN(50) {
		t.Error("should allow 50 tokens")
	}

	// 应该允许另外 50 个
	if !bucket.AllowN(50) {
		t.Error("should allow another 50 tokens")
	}

	// 应该拒绝更多
	if bucket.AllowN(1) {
		t.Error("should not allow when empty")
	}
}

func TestTokenBucketV2_WaitN(t *testing.T) {
	bucket := NewTokenBucketV2(10, 100) // 10 容量，每秒 100 个

	ctx := context.Background()

	// 消耗所有
	bucket.AllowN(10)

	// 等待补充
	start := time.Now()
	err := bucket.WaitN(ctx, 1)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 应该等待约 10ms（1 token / 100 per second）
	if elapsed < 5*time.Millisecond {
		t.Logf("waited %v (might be faster due to timing)", elapsed)
	}
}

func TestTokenBucketV2_Available(t *testing.T) {
	bucket := NewTokenBucketV2(100, 10)

	available := bucket.Available()
	if available != 100 {
		t.Errorf("expected 100 available, got %d", available)
	}

	bucket.AllowN(30)
	available = bucket.Available()
	if available != 70 {
		t.Errorf("expected 70 available, got %d", available)
	}
}

func TestNewOpenAIGPT4Limiter(t *testing.T) {
	limiter := NewOpenAIGPT4Limiter()
	stats := limiter.Stats()

	if stats.TokensPerMinute != 10000 {
		t.Errorf("expected TPM 10000, got %d", stats.TokensPerMinute)
	}
	if stats.RequestsPerMinute != 500 {
		t.Errorf("expected RPM 500, got %d", stats.RequestsPerMinute)
	}
}

func TestMultiDimensionLimiter(t *testing.T) {
	// 用户级限制
	userLimiter := NewTokenBucketV2(100, 10)
	// 全局限制
	globalLimiter := NewTokenBucketV2(1000, 100)

	multi := NewMultiDimensionLimiter(userLimiter, globalLimiter)

	// 应该允许
	if !multi.AllowN(50) {
		t.Error("should allow within both limits")
	}

	// 再请求，超过用户限制
	if !multi.AllowN(50) {
		t.Error("should allow second request")
	}

	// 用户限制耗尽
	if multi.AllowN(50) {
		t.Error("should not allow when user limit exceeded")
	}
}
