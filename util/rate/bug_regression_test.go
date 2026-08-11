// Package rate 的 bug 回归测试。
package rate

import (
	"errors"
	"testing"
	"time"
)

// Record 对任意合法容量都不能 panic，并且必须保留窗口内的全部记录。
func TestBug1_SlidingWindowRecord_SmallCapacityNoPanic(t *testing.T) {
	sw := mustSlidingWindow(t, 3, time.Hour)
	for i := 0; i < 500; i++ {
		sw.Record()
	}
	if got := sw.Count(); got != 500 {
		t.Fatalf("Count = %d, want 500", got)
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
