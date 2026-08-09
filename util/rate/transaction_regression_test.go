package rate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTokenBucketsRejectNonPositiveTokenCounts(t *testing.T) {
	bucket := mustTokenBucketV2(t, 2, 0.000001)
	before := bucket.Available()
	if bucket.AllowN(-1) {
		t.Fatal("AllowN accepted a negative token count")
	}
	if after := bucket.Available(); after != before {
		t.Fatalf("negative token count changed availability: before=%d after=%d", before, after)
	}

	limiter := mustTokenRateLimiter(t, 2, 2)
	statsBefore := limiter.Stats()
	if limiter.AllowN(-1) {
		t.Fatal("TokenRateLimiter accepted a negative token count")
	}
	statsAfter := limiter.Stats()
	if statsAfter.TokensAvailable != statsBefore.TokensAvailable || statsAfter.RequestsAvailable != statsBefore.RequestsAvailable {
		t.Fatalf("negative token count changed limiter state: before=%+v after=%+v", statsBefore, statsAfter)
	}
}

func TestTokenBucketWaitNRejectsNilContext(t *testing.T) {
	bucket := mustTokenBucketV2(t, 1, 1)
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("WaitN panicked for nil context: %v", recovered)
			}
		}()
		//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
		if err := bucket.WaitN(nil, 2); err == nil {
			t.Fatal("expected nil context error")
		}
	}()
}

func TestWaitNDoesNotConsumeWhenContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	bucket := mustTokenBucketV2(t, 2, 1)
	tokenLimiter := mustTokenRateLimiter(t, 2, 2)
	multiBucket := mustTokenBucketV2(t, 2, 1)
	multi := mustMultiDimensionLimiter(t, multiBucket)
	tests := []struct {
		name      string
		available func() int
		wait      func(context.Context) error
	}{
		{
			name:      "token bucket",
			available: bucket.Available,
			wait: func(ctx context.Context) error {
				return bucket.WaitN(ctx, 1)
			},
		},
		{
			name:      "token rate limiter",
			available: tokenLimiter.Available,
			wait: func(ctx context.Context) error {
				return tokenLimiter.WaitN(ctx, 1)
			},
		},
		{
			name:      "multi-dimension limiter",
			available: multi.Available,
			wait: func(ctx context.Context) error {
				return multi.WaitN(ctx, 1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.available()
			if err := test.wait(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("WaitN error = %v, want context.Canceled", err)
			}
			if after := test.available(); after != before {
				t.Fatalf("canceled WaitN consumed capacity: before=%d after=%d", before, after)
			}
		})
	}
}

type blockingAtomicLimiter struct {
	entered       chan struct{}
	release       chan struct{}
	once          sync.Once
	mu            sync.Mutex
	transactionID uint64
}

func (l *blockingAtomicLimiter) Allow() bool { return l.AllowN(1) }

func (l *blockingAtomicLimiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.canAllowNLocked(n) {
		return false
	}
	l.consumeNLocked(n)
	return true
}

func (l *blockingAtomicLimiter) Wait() time.Duration { return 0 }

func (l *blockingAtomicLimiter) WaitN(context.Context, int) error { return nil }

func (l *blockingAtomicLimiter) Available() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.availableLocked()
}

func (l *blockingAtomicLimiter) transactionKey() uint64 { return l.transactionID }

func (l *blockingAtomicLimiter) lockTransaction() { l.mu.Lock() }

func (l *blockingAtomicLimiter) unlockTransaction() { l.mu.Unlock() }

func (l *blockingAtomicLimiter) canAllowNLocked(int) bool {
	l.once.Do(func() { close(l.entered) })
	<-l.release
	return true
}

func (l *blockingAtomicLimiter) consumeNLocked(int) {}

func (l *blockingAtomicLimiter) availableLocked() int { return 1 }

func (l *blockingAtomicLimiter) maxTransactionN() int { return 1 }

func TestMultiDimensionLimiterCannotOverdrawConcurrentLimiter(t *testing.T) {
	bucket := mustTokenBucketV2(t, 1, 0.000001)
	gate := &blockingAtomicLimiter{
		entered:       make(chan struct{}),
		release:       make(chan struct{}),
		transactionID: limiterTransactionSequence.Add(1),
	}
	multi := mustMultiDimensionLimiter(t, bucket, gate)

	multiResult := make(chan bool, 1)
	go func() { multiResult <- multi.AllowN(1) }()
	<-gate.entered

	directResult := make(chan bool, 1)
	go func() { directResult <- bucket.AllowN(1) }()
	select {
	case <-directResult:
		t.Fatal("direct limiter bypassed the multi-dimensional transaction lock")
	case <-time.After(20 * time.Millisecond):
	}
	close(gate.release)
	if !<-multiResult {
		t.Fatal("multi-dimensional limiter should consume the available token")
	}
	if <-directResult {
		t.Fatal("direct limiter consumed a token already committed by the transaction")
	}
}
