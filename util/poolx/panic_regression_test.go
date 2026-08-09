package poolx

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSubmitFuncPanicCompletesFuture(t *testing.T) {
	p := New("future-panic", WithMaxWorkers(1), WithAutoScale(false), WithPanicHandler(func(any) {}))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		panic("future panic")
	})

	select {
	case <-future.Done():
	case <-time.After(time.Second):
		t.Fatal("future remained pending after task panic")
	}
	if _, err := future.Get(); !errors.Is(err, ErrTaskPanic) {
		t.Fatalf("expected ErrTaskPanic, got %v", err)
	}
}

func TestSubmitFuncCtxPanicCompletesFuture(t *testing.T) {
	p := New("future-context-panic", WithMaxWorkers(1), WithAutoScale(false), WithPanicHandler(func(any) {}))
	defer p.Release()

	future := SubmitFuncCtx(context.Background(), p, func(context.Context) (int, error) {
		panic("context future panic")
	})

	select {
	case <-future.Done():
	case <-time.After(time.Second):
		t.Fatal("context future remained pending after task panic")
	}
	if _, err := future.Get(); !errors.Is(err, ErrTaskPanic) {
		t.Fatalf("expected ErrTaskPanic, got %v", err)
	}
}

func TestTrySubmitFuncPanicCompletesFuture(t *testing.T) {
	p := New("try-future-panic", WithMaxWorkers(1), WithAutoScale(false), WithPanicHandler(func(any) {}))
	defer p.Release()

	future := TrySubmitFunc(p, func() (int, error) {
		panic("try future panic")
	})
	if future == nil {
		t.Fatal("expected task submission to succeed")
	}

	select {
	case <-future.Done():
	case <-time.After(time.Second):
		t.Fatal("try future remained pending after task panic")
	}
	if _, err := future.Get(); !errors.Is(err, ErrTaskPanic) {
		t.Fatalf("expected ErrTaskPanic, got %v", err)
	}
}

func TestSubmitWaitReturnsTaskPanic(t *testing.T) {
	p := New("wait-panic", WithMaxWorkers(1), WithAutoScale(false), WithPanicHandler(func(any) {}))
	defer p.Release()

	if err := p.SubmitWait(func() { panic("wait panic") }); !errors.Is(err, ErrTaskPanic) {
		t.Fatalf("expected ErrTaskPanic, got %v", err)
	}
}

func TestParallelReturnsTaskPanic(t *testing.T) {
	SetPanicHandler(func(any) {})
	result := make(chan error, 1)
	go func() {
		result <- Parallel(func() { panic("parallel panic") })
	}()

	select {
	case err := <-result:
		if !errors.Is(err, ErrTaskPanic) {
			t.Fatalf("expected ErrTaskPanic, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Parallel remained blocked after task panic")
	}
}

func TestSetDefaultPoolRejectsNil(t *testing.T) {
	original := DefaultPool()
	if err := SetDefaultPool(nil); !errors.Is(err, ErrInvalidArg) {
		t.Fatalf("expected ErrInvalidArg, got %v", err)
	}
	if DefaultPool() != original {
		t.Fatal("nil replacement changed the default pool")
	}
}

func TestDefaultPoolConcurrentReplacement(t *testing.T) {
	original := DefaultPool()
	first := New("default-replacement-first", WithMaxWorkers(1), WithAutoScale(false))
	second := New("default-replacement-second", WithMaxWorkers(1), WithAutoScale(false))
	defer first.Release()
	defer second.Release()
	defer func() {
		if err := SetDefaultPool(original); err != nil {
			t.Errorf("restore default pool: %v", err)
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			if err := SetDefaultPool(first); err != nil {
				t.Errorf("set first default pool: %v", err)
				return
			}
			if err := SetDefaultPool(second); err != nil {
				t.Errorf("set second default pool: %v", err)
				return
			}
		}
	}()
	for range 2000 {
		if DefaultPool() == nil {
			t.Fatal("default pool became nil")
		}
	}
	<-done
}

func TestSubmitFuncCtxRejectsNilContext(t *testing.T) {
	p := New("nil-context", WithMaxWorkers(1), WithAutoScale(false))
	defer p.Release()

	var future *Future[int]
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("SubmitFuncCtx panicked for nil context: %v", recovered)
			}
		}()
		//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
		future = SubmitFuncCtx(nil, p, func(context.Context) (int, error) { return 1, nil })
	}()

	if _, err := future.GetWithTimeout(time.Second); !errors.Is(err, ErrInvalidArg) {
		t.Fatalf("expected ErrInvalidArg, got %v", err)
	}
}
