package rate

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Coverage for previously-uncovered SlidingWindow accessors and helpers.
func TestSlidingWindow_AccessorsAndRecord(t *testing.T) {
	sw := mustSlidingWindow(t, 3, time.Minute)

	if sw.Capacity() != 3 {
		t.Errorf("Capacity = %d, want 3", sw.Capacity())
	}
	if sw.Window() != time.Minute {
		t.Errorf("Window = %v, want 1m", sw.Window())
	}
	if sw.Count() != 0 {
		t.Errorf("initial Count = %d, want 0", sw.Count())
	}

	// TryAllow until full
	if ok, c := sw.TryAllow(); !ok || c != 1 {
		t.Errorf("TryAllow#1 = (%v,%d), want (true,1)", ok, c)
	}
	if ok, _ := sw.TryAllow(); !ok {
		t.Errorf("TryAllow#2 should be allowed")
	}
	if ok, _ := sw.TryAllow(); !ok {
		t.Errorf("TryAllow#3 should be allowed")
	}
	if ok, c := sw.TryAllow(); ok || c != 3 {
		t.Errorf("TryAllow#4 = (%v,%d), want (false,3) — over capacity", ok, c)
	}
	if sw.Count() != 3 {
		t.Errorf("Count = %d, want 3", sw.Count())
	}

	// Reset clears the window
	sw.Reset()
	if sw.Count() != 0 {
		t.Errorf("Count after Reset = %d, want 0", sw.Count())
	}

	// Record does not enforce capacity
	for i := 0; i < 10; i++ {
		sw.Record()
	}
	if sw.Count() != 10 {
		t.Errorf("Count after 10 Record = %d, want 10 (Record ignores capacity)", sw.Count())
	}
}

// Record 的有界增长路径：容量不小于 50 时，裁剪分支创建的切片长度不会超过容量。
func TestSlidingWindow_RecordBoundsMemory(t *testing.T) {
	sw := mustSlidingWindow(t, 60, time.Hour) // 容量不小于 50 时裁剪长度不会超过容量。
	for i := 0; i < 250; i++ {
		sw.Record()
	}
	// 最大长度为 120；达到上限后先删除最旧的一半再追加。
	if sw.Count() > 200 {
		t.Errorf("Count = %d, expected bounded well under 250 (memory cap)", sw.Count())
	}
	if sw.Count() == 0 {
		t.Errorf("Count = 0, expected some records retained")
	}
}

// Coverage for TokenRateLimiter public API + presets.
func TestTokenRateLimiter_API(t *testing.T) {
	l := mustTokenRateLimiter(t, 1000, 100) // 1000 TPM, 100 RPM

	if l.Available() <= 0 {
		t.Errorf("Available = %d, want >0 at start", l.Available())
	}
	if !l.Allow() {
		t.Errorf("first Allow should succeed")
	}
	if !l.AllowN(10) {
		t.Errorf("AllowN(10) should succeed with full bucket")
	}

	// AllowN 必须在一次原子操作中完成检查与消费。
	before := l.Available()
	if !l.AllowN(5) {
		t.Errorf("AllowN(5) should be true")
	}
	if l.Available() >= before {
		t.Errorf("AllowN(5) should consume availability: before=%d after=%d", before, l.Available())
	}

	// Stats sanity
	st := l.Stats()
	if st.TokensPerMinute != 1000 || st.RequestsPerMinute != 100 {
		t.Errorf("Stats TPM/RPM = %d/%d, want 1000/100", st.TokensPerMinute, st.RequestsPerMinute)
	}

	// 永远无法满足的请求应立即失败，不能永久等待。
	l2 := mustTokenRateLimiter(t, 10, 10)
	if err := l2.WaitN(context.Background(), 1000); !errors.Is(err, ErrInsufficientTokens) {
		t.Errorf("WaitN over capacity = %v, want ErrInsufficientTokens", err)
	}

	// WaitN succeeds immediately when tokens available.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.WaitN(ctx, 1); err != nil {
		t.Errorf("WaitN(1) = %v, want nil", err)
	}

	// Presets construct non-nil limiters.
	for name, ctor := range map[string]func() *TokenRateLimiter{
		"gpt4":      NewOpenAIGPT4Limiter,
		"gpt4o":     NewOpenAIGPT4oLimiter,
		"gpt4omini": NewOpenAIGPT4oMiniLimiter,
		"sonnet":    NewClaudeSonnetLimiter,
		"haiku":     NewClaudeHaikuLimiter,
		"deepseek":  NewDeepSeekLimiter,
		"qwen":      NewQwenLimiter,
	} {
		if ctor() == nil {
			t.Errorf("preset %s returned nil", name)
		}
	}
}

// TokenBucketV2 的 Available 与 Wait 覆盖。
func TestTokenBucketV2_AvailableAndWait(t *testing.T) {
	tb := mustTokenBucketV2(t, 5, 5) // 5 cap, 5/s
	if tb.Available() != 5 {
		t.Errorf("Available = %d, want 5", tb.Available())
	}
	if !tb.AllowN(5) {
		t.Fatal("AllowN(5) should consume the initial capacity")
	}
	// 等待一个令牌；补充速率为每秒 5 个时应快速返回。
	d := tb.Wait()
	if d > 2*time.Second {
		t.Errorf("Wait took too long: %v", d)
	}
}

// MultiDimensionLimiter 要求所有维度都允许请求。
func TestMultiDimensionLimiter_TightestBinds(t *testing.T) {
	small := mustTokenRateLimiter(t, 3, 3) // 最严格的维度。
	big := mustTokenRateLimiter(t, 1000, 1000)
	m := mustMultiDimensionLimiter(t, small, big)

	// 前几个请求由较小的每分钟请求数和令牌数限制。
	allowed := 0
	for i := 0; i < 10; i++ {
		if m.Allow() {
			allowed++
		}
	}
	if allowed == 0 {
		t.Errorf("expected some requests allowed")
	}
	if allowed > 3 {
		t.Errorf("allowed=%d, expected <=3 (bounded by tightest dimension)", allowed)
	}
}
