// Package rate 的 bug 回归测试。
package rate

import (
	"errors"
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
	sw := mustSlidingWindow(t, 3, time.Hour)
	for i := 0; i < 500; i++ {
		sw.Record()
	}
	// Reaching here without panic is the assertion; also sanity-check bounding.
	if got := sw.Count(); got == 0 || got > 500 {
		t.Fatalf("Count = %d, want bounded in (0,500]", got)
	}
}

// 非正漏水间隔必须在构造阶段被拒绝，不能进入除零或忙等路径。
func TestLeakyBucketRejectsNonPositiveRate(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		if limiter, err := NewLeakyBucket(2, interval); !errors.Is(err, ErrInvalidRate) || limiter != nil {
			t.Fatalf("NewLeakyBucket(%s) = (%v, %v), want nil ErrInvalidRate", interval, limiter, err)
		}
	}
}
