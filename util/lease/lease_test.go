package lease

import (
	"context"
	"testing"
	"time"
)

// TestMemoryLease_MutualExclusion 验证持有期间他人无法获取同一 key。
func TestMemoryLease_MutualExclusion(t *testing.T) {
	l := NewMemoryLease()
	ctx := context.Background()

	tok, ok, err := l.Acquire(ctx, "session-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("首次获取应成功: ok=%v err=%v", ok, err)
	}
	if tok == 0 {
		t.Error("fencing token 应非零")
	}

	if _, ok2, _ := l.Acquire(ctx, "session-1", time.Minute); ok2 {
		t.Error("持有期间他人不应获取成功")
	}
}

// TestMemoryLease_FencingMonotonic 验证释放后再获取，fencing token 单调递增。
func TestMemoryLease_FencingMonotonic(t *testing.T) {
	l := NewMemoryLease()
	ctx := context.Background()

	tok1, _, _ := l.Acquire(ctx, "k", time.Minute)
	if err := l.Release(ctx, "k", tok1); err != nil {
		t.Fatalf("Release: %v", err)
	}
	tok2, ok, _ := l.Acquire(ctx, "k", time.Minute)
	if !ok {
		t.Fatal("释放后应可重新获取")
	}
	if tok2 <= tok1 {
		t.Errorf("fencing token 应单调递增: tok1=%d tok2=%d", tok1, tok2)
	}
}

// TestMemoryLease_ExpiryReacquire 验证 TTL 过期后可被重新获取（用注入时钟控制）。
func TestMemoryLease_ExpiryReacquire(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewMemoryLease(WithClock(func() time.Time { return now }))
	ctx := context.Background()

	if _, ok, _ := l.Acquire(ctx, "k", time.Second); !ok {
		t.Fatal("首次获取应成功")
	}
	// 推进时钟越过 TTL
	now = now.Add(2 * time.Second)
	if _, ok, _ := l.Acquire(ctx, "k", time.Second); !ok {
		t.Error("过期后应可重新获取")
	}
}

// TestMemoryLease_ReleaseWrongToken 验证用错误 token 释放不影响当前持有者。
func TestMemoryLease_ReleaseWrongToken(t *testing.T) {
	l := NewMemoryLease()
	ctx := context.Background()

	tok, _, _ := l.Acquire(ctx, "k", time.Minute)
	// 用错误 token 释放（应无副作用）
	_ = l.Release(ctx, "k", tok+999)
	if _, ok, _ := l.Acquire(ctx, "k", time.Minute); ok {
		t.Error("错误 token 释放不应释放当前持有者")
	}
}

// TestMemoryLease_RefreshWrongToken 验证非持有者续租返回 ErrNotHolder。
func TestMemoryLease_RefreshWrongToken(t *testing.T) {
	l := NewMemoryLease()
	ctx := context.Background()

	tok, _, _ := l.Acquire(ctx, "k", time.Minute)
	if err := l.Refresh(ctx, "k", tok, time.Minute); err != nil {
		t.Errorf("持有者续租应成功: %v", err)
	}
	if err := l.Refresh(ctx, "k", tok+1, time.Minute); err != ErrNotHolder {
		t.Errorf("非持有者续租应返回 ErrNotHolder, got %v", err)
	}
}
