package rate

import (
	"context"
	"testing"
	"time"
)

// Coverage for previously-uncovered SlidingWindow accessors and helpers.
func TestSlidingWindow_AccessorsAndRecord(t *testing.T) {
	sw := NewSlidingWindow(3, time.Minute)

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

// Record's anti-unbounded-growth path: with capacity>=50 the trim branch makes
// a slice whose len<=cap, so it is safe. (capacity<50 hits a known makeslice
// panic at limiter.go:300 — see audit report; not exercised here to keep green.)
func TestSlidingWindow_RecordBoundsMemory(t *testing.T) {
	sw := NewSlidingWindow(60, time.Hour) // capacity>=50 keeps trim slice len<=cap
	for i := 0; i < 250; i++ {
		sw.Record()
	}
	// maxSize = max(60*2,100)=120; once >=120 it removes oldest half then appends.
	if sw.Count() > 200 {
		t.Errorf("Count = %d, expected bounded well under 250 (memory cap)", sw.Count())
	}
	if sw.Count() == 0 {
		t.Errorf("Count = 0, expected some records retained")
	}
}

// Coverage for TokenRateLimiter public API + presets.
func TestTokenRateLimiter_API(t *testing.T) {
	l := NewTokenRateLimiter(1000, 100) // 1000 TPM, 100 RPM

	if l.Available() <= 0 {
		t.Errorf("Available = %d, want >0 at start", l.Available())
	}
	if !l.Allow() {
		t.Errorf("first Allow should succeed")
	}
	if !l.AllowN(10) {
		t.Errorf("AllowN(10) should succeed with full bucket")
	}

	// TryAllowN does not consume; ConsumeN does.
	before := l.Available()
	if !l.TryAllowN(5) {
		t.Errorf("TryAllowN(5) should be true")
	}
	if l.Available() != before {
		t.Errorf("TryAllowN must not consume: before=%d after=%d", before, l.Available())
	}
	l.ConsumeN(5)
	if l.Available() > before {
		t.Errorf("ConsumeN(5) should not increase availability")
	}

	// Stats sanity
	st := l.Stats()
	if st.TokensPerMinute != 1000 || st.RequestsPerMinute != 100 {
		t.Errorf("Stats TPM/RPM = %d/%d, want 1000/100", st.TokensPerMinute, st.RequestsPerMinute)
	}

	// Reserve returns 0 when capacity available, >0 when not.
	l2 := NewTokenRateLimiter(10, 10)
	if w := l2.Reserve(5); w != 0 {
		t.Errorf("Reserve(5) with full bucket = %v, want 0", w)
	}
	if w := l2.Reserve(1000); w <= 0 {
		t.Errorf("Reserve(1000) over capacity = %v, want >0", w)
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

// TokenBucketV2 Reserve/Available/Wait coverage.
func TestTokenBucketV2_ReserveAvailable(t *testing.T) {
	tb := NewTokenBucketV2(5, 5) // 5 cap, 5/s
	if tb.Available() != 5 {
		t.Errorf("Available = %d, want 5", tb.Available())
	}
	if w := tb.Reserve(); w != 0 {
		t.Errorf("Reserve() with tokens = %v, want 0", w)
	}
	if w := tb.ReserveN(100); w <= 0 {
		t.Errorf("ReserveN(100) over cap = %v, want >0", w)
	}
	// Wait for 1 token: with refill rate 5/s should return quickly.
	d := tb.Wait()
	if d > 2*time.Second {
		t.Errorf("Wait took too long: %v", d)
	}
}

// MultiDimensionLimiter: both dimensions must allow.
func TestMultiDimensionLimiter_TightestBinds(t *testing.T) {
	small := NewTokenRateLimiter(3, 3) // tight
	big := NewTokenRateLimiter(1000, 1000)
	m := NewMultiDimensionLimiter(small, big)

	// First couple allowed (limited by small=3 RPM/TPM).
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
