package rate

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	// Create a bucket with capacity 5 and rate 10/s
	tb := NewTokenBucket(5, 10)

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied (no tokens left)
	if tb.Allow() {
		t.Error("6th request should be denied")
	}

	// Wait for token refill
	time.Sleep(200 * time.Millisecond)

	// Should allow again
	if !tb.Allow() {
		t.Error("Request after refill should be allowed")
	}
}

func TestTokenBucket_Wait(t *testing.T) {
	// Create a bucket with capacity 1 and rate 10/s
	tb := NewTokenBucket(1, 10)

	// First request should not wait
	waitTime := tb.Wait()
	if waitTime != 0 {
		t.Errorf("First request waitTime = %v, want 0", waitTime)
	}

	// Second request should wait
	waitTime = tb.Wait()
	if waitTime <= 0 {
		t.Error("Second request should have wait time > 0")
	}
}

func TestTokenBucket_Concurrent(t *testing.T) {
	tb := NewTokenBucket(100, 1000)

	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tb.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have allowed around 100 (initial capacity)
	if allowed > 110 || allowed < 90 {
		t.Errorf("Allowed %d requests, expected around 100", allowed)
	}
}

func TestLeakyBucket_Allow(t *testing.T) {
	// Create a bucket with capacity 5 and leak rate 50ms
	lb := NewLeakyBucket(5, 50*time.Millisecond)

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		if !lb.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if lb.Allow() {
		t.Error("6th request should be denied")
	}

	// Wait for water to leak
	time.Sleep(100 * time.Millisecond)

	// Should allow again (some water leaked)
	if !lb.Allow() {
		t.Error("Request after leak should be allowed")
	}
}

func TestLeakyBucket_Wait(t *testing.T) {
	lb := NewLeakyBucket(1, 50*time.Millisecond)

	// First request should not wait
	waitTime := lb.Wait()
	if waitTime != 0 {
		t.Errorf("First request waitTime = %v, want 0", waitTime)
	}

	// Second request should wait
	waitTime = lb.Wait()
	if waitTime < 50*time.Millisecond {
		t.Errorf("Second request waitTime = %v, want >= 50ms", waitTime)
	}
}

func TestLeakyBucket_Concurrent(t *testing.T) {
	lb := NewLeakyBucket(50, 10*time.Millisecond)

	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lb.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have allowed around 50 (capacity)
	if allowed > 60 || allowed < 40 {
		t.Errorf("Allowed %d requests, expected around 50", allowed)
	}
}

func TestSlidingWindow_Allow(t *testing.T) {
	// Create window with capacity 5 and window 200ms
	sw := NewSlidingWindow(5, 200*time.Millisecond)

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		if !sw.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if sw.Allow() {
		t.Error("6th request should be denied")
	}

	// Wait for window to slide (wait longer than window)
	time.Sleep(300 * time.Millisecond)

	// Should allow again after all old requests expired
	if !sw.Allow() {
		t.Error("Request after window slide should be allowed")
	}
}

func TestSlidingWindow_Wait(t *testing.T) {
	sw := NewSlidingWindow(1, 100*time.Millisecond)

	// First request should not wait
	waitTime := sw.Wait()
	if waitTime != 0 {
		t.Errorf("First request waitTime = %v, want 0", waitTime)
	}

	// Second request should wait
	start := time.Now()
	sw.Wait()
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("Second request should wait longer, elapsed = %v", elapsed)
	}
}

func TestSlidingWindow_Concurrent(t *testing.T) {
	sw := NewSlidingWindow(50, time.Second)

	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sw.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have allowed exactly 50
	if allowed != 50 {
		t.Errorf("Allowed %d requests, expected 50", allowed)
	}
}

func TestLimiterInterface(t *testing.T) {
	// Verify all limiters implement the Limiter interface
	var _ Limiter = &TokenBucket{}
	var _ Limiter = &LeakyBucket{}
	var _ Limiter = &SlidingWindow{}
}
