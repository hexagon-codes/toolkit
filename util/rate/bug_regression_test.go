// Package rate 的 bug 回归测试。
package rate

import (
	"testing"
	"time"
)

// Bug1: SlidingWindow.Record must never panic for any capacity, including small
// capacities where the anti-growth trim path builds a new slice. Previously the
// trim used make([]time.Time, len-removeCount, capacity) where len-removeCount
// could exceed capacity, panicking with "makeslice: cap out of range".
func TestBug1_SlidingWindowRecord_SmallCapacityNoPanic(t *testing.T) {
	// Window large enough that cleanup never expires entries during the test,
	// so Record's trim branch is forced once len reaches maxSize (=100 here).
	sw := NewSlidingWindow(3, time.Hour)
	for i := 0; i < 500; i++ {
		sw.Record()
	}
	// Reaching here without panic is the assertion; also sanity-check bounding.
	if got := sw.Count(); got == 0 || got > 500 {
		t.Fatalf("Count = %d, want bounded in (0,500]", got)
	}
}

// Bug4: NewLeakyBucket with a non-positive rate must not cause an integer
// divide-by-zero panic in Allow/leak. A zero rate is treated as no throttle.
func TestBug4_LeakyBucket_ZeroRateNoDivideByZero(t *testing.T) {
	lb := NewLeakyBucket(2, 0)
	for i := 0; i < 10; i++ {
		if !lb.Allow() {
			t.Fatalf("Allow() = false at i=%d, want true (zero rate = no throttle)", i)
		}
	}
}
