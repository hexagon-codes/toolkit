package syncx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Semaphore tests
func TestSemaphore_Basic(t *testing.T) {
	sem := NewSemaphore(3)

	if sem.Capacity() != 3 {
		t.Errorf("expected capacity=3, got %d", sem.Capacity())
	}

	if sem.Available() != 3 {
		t.Errorf("expected available=3, got %d", sem.Available())
	}

	if sem.Held() != 0 {
		t.Errorf("expected held=0, got %d", sem.Held())
	}
}

func TestSemaphore_Acquire(t *testing.T) {
	sem := NewSemaphore(2)

	sem.Acquire()
	if sem.Available() != 1 {
		t.Errorf("expected available=1, got %d", sem.Available())
	}

	sem.Acquire()
	if sem.Available() != 0 {
		t.Errorf("expected available=0, got %d", sem.Available())
	}

	sem.Release()
	if sem.Available() != 1 {
		t.Errorf("expected available=1 after release, got %d", sem.Available())
	}

	sem.Release()
	if sem.Available() != 2 {
		t.Errorf("expected available=2 after release, got %d", sem.Available())
	}
}

func TestSemaphore_TryAcquire(t *testing.T) {
	sem := NewSemaphore(1)

	if !sem.TryAcquire() {
		t.Error("expected TryAcquire to succeed")
	}

	if sem.TryAcquire() {
		t.Error("expected TryAcquire to fail when full")
	}

	sem.Release()

	if !sem.TryAcquire() {
		t.Error("expected TryAcquire to succeed after release")
	}
}

func TestSemaphore_AcquireContext(t *testing.T) {
	sem := NewSemaphore(1)
	sem.Acquire()

	// 测试超时
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := sem.AcquireContext(ctx)
	if err == nil {
		t.Error("expected timeout error")
	}

	sem.Release()

	// 测试成功获取
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()

	err = sem.AcquireContext(ctx2)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSemaphore_Concurrent(t *testing.T) {
	sem := NewSemaphore(3)
	var maxConcurrent atomic.Int32
	var current atomic.Int32
	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem.Acquire()
			defer sem.Release()

			current.Add(1)
			if c := current.Load(); c > maxConcurrent.Load() {
				maxConcurrent.Store(c)
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
		}()
	}

	wg.Wait()

	if maxConcurrent.Load() > 3 {
		t.Errorf("expected maxConcurrent<=3, got %d", maxConcurrent.Load())
	}
}

func TestSemaphore_ZeroCapacity(t *testing.T) {
	sem := NewSemaphore(0)
	if sem.Capacity() != 1 {
		t.Errorf("expected capacity=1 for zero input, got %d", sem.Capacity())
	}
}

// Once tests
func TestOnce_Do(t *testing.T) {
	var o Once[int]
	count := 0

	v1 := o.Do(func() int {
		count++
		return 42
	})

	v2 := o.Do(func() int {
		count++
		return 100
	})

	if v1 != 42 || v2 != 42 {
		t.Errorf("expected 42, got v1=%d, v2=%d", v1, v2)
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestOnce_Value(t *testing.T) {
	var o Once[string]

	val, ok := o.Value()
	if ok {
		t.Error("expected not initialized before Do")
	}
	if val != "" {
		t.Error("expected empty string before initialization")
	}

	o.Do(func() string { return "hello" })

	val, ok = o.Value()
	if !ok {
		t.Error("expected initialized after Do")
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %s", val)
	}
}

func TestOnceValue(t *testing.T) {
	count := 0
	fn := OnceValue(func() int {
		count++
		return 42
	})

	v1 := fn()
	v2 := fn()
	v3 := fn()

	if v1 != 42 || v2 != 42 || v3 != 42 {
		t.Error("expected all values to be 42")
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestOnceValueErr(t *testing.T) {
	count := 0
	fn := OnceValueErr(func() (int, error) {
		count++
		if count == 1 {
			return 42, nil
		}
		return 0, errors.New("should not happen")
	})

	v1, err1 := fn()
	v2, err2 := fn()

	if v1 != 42 || err1 != nil {
		t.Errorf("expected 42, nil; got %d, %v", v1, err1)
	}

	if v2 != 42 || err2 != nil {
		t.Errorf("expected cached 42, nil; got %d, %v", v2, err2)
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestOnceFunc(t *testing.T) {
	count := 0
	fn := OnceFunc(func() {
		count++
	})

	fn()
	fn()
	fn()

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestOnceErr_Do(t *testing.T) {
	var o OnceErr[int]

	v1, err1 := o.Do(func() (int, error) {
		return 42, nil
	})

	v2, err2 := o.Do(func() (int, error) {
		return 100, errors.New("should not happen")
	})

	if v1 != 42 || err1 != nil {
		t.Errorf("expected 42, nil; got %d, %v", v1, err1)
	}

	if v2 != 42 || err2 != nil {
		t.Errorf("expected cached 42, nil; got %d, %v", v2, err2)
	}
}

func TestOnceErr_WithError(t *testing.T) {
	var o OnceErr[int]
	expectedErr := errors.New("init error")

	v, err := o.Do(func() (int, error) {
		return 0, expectedErr
	})

	if v != 0 || err != expectedErr {
		t.Errorf("expected 0, error; got %d, %v", v, err)
	}

	v, err, ok := o.Value()
	if !ok {
		t.Error("expected initialized after Do")
	}
	if v != 0 || err != expectedErr {
		t.Errorf("expected cached 0, error; got %d, %v", v, err)
	}
}

// Lazy tests
func TestLazy_Get(t *testing.T) {
	count := 0
	lazy := NewLazy(func() int {
		count++
		return 42
	})

	v1 := lazy.Get()
	v2 := lazy.Get()
	v3 := lazy.Get()

	if v1 != 42 || v2 != 42 || v3 != 42 {
		t.Error("expected all values to be 42")
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestLazy_IsInitialized(t *testing.T) {
	lazy := NewLazy(func() int { return 42 })

	// IsInitialized 不应该触发初始化，只是检查状态
	if lazy.IsInitialized() {
		t.Error("expected not initialized before Get")
	}

	// 调用 Get 后仍然未初始化（因为 IsInitialized 不触发初始化）
	if lazy.IsInitialized() {
		t.Error("expected still not initialized because IsInitialized doesn't trigger init")
	}

	// 调用 Get() 触发初始化
	_ = lazy.Get()

	// 现在应该已初始化
	if !lazy.IsInitialized() {
		t.Error("expected initialized after Get")
	}
}

func TestLazy_Concurrent(t *testing.T) {
	var count atomic.Int32
	lazy := NewLazy(func() int {
		count.Add(1)
		return 42
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := lazy.Get()
			if v != 42 {
				t.Errorf("expected 42, got %d", v)
			}
		}()
	}

	wg.Wait()

	if count.Load() != 1 {
		t.Errorf("expected count=1, got %d", count.Load())
	}
}

func TestLazyErr_Get(t *testing.T) {
	count := 0
	lazy := NewLazyErr(func() (int, error) {
		count++
		return 42, nil
	})

	v1, err1 := lazy.Get()
	v2, err2 := lazy.Get()

	if v1 != 42 || err1 != nil {
		t.Errorf("expected 42, nil; got %d, %v", v1, err1)
	}

	if v2 != 42 || err2 != nil {
		t.Errorf("expected 42, nil; got %d, %v", v2, err2)
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestLazyErr_WithError(t *testing.T) {
	expectedErr := errors.New("init error")
	lazy := NewLazyErr(func() (int, error) {
		return 0, expectedErr
	})

	v, err := lazy.Get()
	if v != 0 || err != expectedErr {
		t.Errorf("expected 0, error; got %d, %v", v, err)
	}

	if lazy.Err() != expectedErr {
		t.Error("expected Err() to return the error")
	}
}

func TestLazyErr_MustGet(t *testing.T) {
	lazy := NewLazyErr(func() (int, error) {
		return 42, nil
	})

	v := lazy.MustGet()
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestLazyErr_MustGet_Panic(t *testing.T) {
	lazy := NewLazyErr(func() (int, error) {
		return 0, errors.New("error")
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()

	lazy.MustGet()
}

func TestLazyValue(t *testing.T) {
	count := 0
	fn := LazyValue(func() int {
		count++
		return 42
	})

	v1 := fn()
	v2 := fn()

	if v1 != 42 || v2 != 42 {
		t.Error("expected 42")
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}

func TestLazyValueErr(t *testing.T) {
	count := 0
	fn := LazyValueErr(func() (int, error) {
		count++
		return 42, nil
	})

	v1, err1 := fn()
	v2, err2 := fn()

	if v1 != 42 || err1 != nil || v2 != 42 || err2 != nil {
		t.Error("unexpected values")
	}

	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}
