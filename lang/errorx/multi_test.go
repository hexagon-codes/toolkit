package errorx

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMultiError_Basic(t *testing.T) {
	me := NewMultiError()

	if me.Len() != 0 {
		t.Error("new MultiError should have 0 errors")
	}

	if me.HasErrors() {
		t.Error("new MultiError should not have errors")
	}

	if me.ErrorOrNil() != nil {
		t.Error("ErrorOrNil should return nil for empty MultiError")
	}
}

func TestMultiError_Append(t *testing.T) {
	me := NewMultiError()
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	me.Append(err1)
	me.Append(nil) // nil 应该被忽略
	me.Append(err2)

	if me.Len() != 2 {
		t.Errorf("expected 2 errors, got %d", me.Len())
	}

	if !me.HasErrors() {
		t.Error("HasErrors should return true")
	}

	if me.ErrorOrNil() == nil {
		t.Error("ErrorOrNil should return error")
	}
}

func TestMultiError_AppendMultiple(t *testing.T) {
	me := NewMultiError()
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	me.Append(err1, nil, err2)

	if me.Len() != 2 {
		t.Errorf("expected 2 errors, got %d", me.Len())
	}
}

func TestMultiError_Chain(t *testing.T) {
	me := NewMultiError()
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	me.Append(err1, err2)

	if me.Len() != 2 {
		t.Errorf("expected 2 errors, got %d", me.Len())
	}
}

func TestMultiError_Errors(t *testing.T) {
	me := NewMultiError()
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	me.Append(err1, err2)

	errs := me.Errors()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}

	// 验证返回的是副本
	errs[0] = errors.New("modified")
	if me.First().Error() == "modified" {
		t.Error("Errors should return a copy")
	}
}

func TestMultiError_FirstLast(t *testing.T) {
	me := NewMultiError()

	if me.First() != nil {
		t.Error("First should return nil for empty MultiError")
	}
	if me.Last() != nil {
		t.Error("Last should return nil for empty MultiError")
	}

	err1 := errors.New("first")
	err2 := errors.New("middle")
	err3 := errors.New("last")
	me.Append(err1, err2, err3)

	if me.First().Error() != "first" {
		t.Errorf("First should return first error, got %v", me.First())
	}
	if me.Last().Error() != "last" {
		t.Errorf("Last should return last error, got %v", me.Last())
	}
}

func TestMultiError_Error(t *testing.T) {
	me := NewMultiError()

	// 空 MultiError
	if me.Error() != "" {
		t.Errorf("empty MultiError should return empty string, got %q", me.Error())
	}

	// 单个错误
	me.Append(errors.New("only error"))
	if me.Error() != "only error" {
		t.Errorf("single error should return just the error, got %q", me.Error())
	}

	// 多个错误
	me.Append(errors.New("second error"))
	errStr := me.Error()
	if errStr == "" {
		t.Error("Error should not return empty string")
	}
	if !contains(errStr, "only error") || !contains(errStr, "second error") {
		t.Errorf("Error should contain all errors, got %q", errStr)
	}
}

func TestMultiError_Unwrap(t *testing.T) {
	me := NewMultiError()
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	me.Append(err1, err2)

	unwrapped := me.Unwrap()
	if len(unwrapped) != 2 {
		t.Errorf("expected 2 errors, got %d", len(unwrapped))
	}
}

func TestMultiError_Concurrent(t *testing.T) {
	me := NewMultiError()
	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			me.Append(fmt.Errorf("error %d", i))
		}(i)
	}

	wg.Wait()

	if me.Len() != n {
		t.Errorf("expected %d errors, got %d", n, me.Len())
	}
}

func TestGo(t *testing.T) {
	var counter atomic.Int32

	err := Go(
		func() error {
			counter.Add(1)
			return nil
		},
		func() error {
			counter.Add(1)
			return errors.New("error 1")
		},
		func() error {
			counter.Add(1)
			return errors.New("error 2")
		},
	)

	if counter.Load() != 3 {
		t.Errorf("expected all 3 functions to run, ran %d", counter.Load())
	}

	var me *MultiError
	if !errors.As(err, &me) {
		t.Fatalf("Go error type = %T, want *MultiError", err)
	}
	if me.Len() != 2 {
		t.Errorf("expected 2 errors, got %d", me.Len())
	}
}

func TestGo_AllSuccess(t *testing.T) {
	err := Go(
		func() error { return nil },
		func() error { return nil },
		func() error { return nil },
	)

	if err != nil {
		t.Errorf("Go should return nil when all succeed, got %v", err)
	}
}

func TestGoWithLimit(t *testing.T) {
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	err := GoWithLimit(2,
		func() error {
			current.Add(1)
			if c := current.Load(); c > maxConcurrent.Load() {
				maxConcurrent.Store(c)
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
			return nil
		},
		func() error {
			current.Add(1)
			if c := current.Load(); c > maxConcurrent.Load() {
				maxConcurrent.Store(c)
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
			return nil
		},
		func() error {
			current.Add(1)
			if c := current.Load(); c > maxConcurrent.Load() {
				maxConcurrent.Store(c)
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
			return nil
		},
		func() error {
			current.Add(1)
			if c := current.Load(); c > maxConcurrent.Load() {
				maxConcurrent.Store(c)
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
			return errors.New("error")
		},
	)

	if maxConcurrent.Load() > 2 {
		t.Errorf("max concurrent should be <= 2, got %d", maxConcurrent.Load())
	}

	var me *MultiError
	if !errors.As(err, &me) {
		t.Fatalf("GoWithLimit error type = %T, want *MultiError", err)
	}
	if me.Len() != 1 {
		t.Errorf("expected 1 error, got %d", me.Len())
	}
}

func TestGoWithLimit_ZeroLimit(t *testing.T) {
	err := GoWithLimit(0,
		func() error { return nil },
		func() error { return errors.New("error") },
	)

	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("GoWithLimit error = %v, want ErrInvalidLimit", err)
	}
}

func TestWalk(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	wrapped := fmt.Errorf("wrapped: %w", err1)
	joined := errors.Join(err1, err2)

	// 测试单个错误
	var visited []error
	Walk(err1, func(e error) bool {
		visited = append(visited, e)
		return true
	})
	if len(visited) != 1 {
		t.Errorf("expected 1 visited error, got %d", len(visited))
	}

	// 测试包装错误
	visited = nil
	Walk(wrapped, func(e error) bool {
		visited = append(visited, e)
		return true
	})
	if len(visited) != 2 {
		t.Errorf("expected 2 visited errors, got %d", len(visited))
	}

	// 测试 joined 错误
	visited = nil
	Walk(joined, func(e error) bool {
		visited = append(visited, e)
		return true
	})
	if len(visited) != 3 { // joined + err1 + err2
		t.Errorf("expected 3 visited errors, got %d", len(visited))
	}

	// 测试提前停止
	visited = nil
	Walk(joined, func(e error) bool {
		visited = append(visited, e)
		return false // 停止
	})
	if len(visited) != 1 {
		t.Errorf("expected 1 visited error with early stop, got %d", len(visited))
	}

	// 测试 nil
	visited = nil
	Walk(nil, func(e error) bool {
		visited = append(visited, e)
		return true
	})
	if len(visited) != 0 {
		t.Errorf("expected 0 visited errors for nil, got %d", len(visited))
	}
}

func TestWalk_MultiError(t *testing.T) {
	me := NewMultiError()
	me.Append(errors.New("error 1"))
	me.Append(errors.New("error 2"))

	var visited []string
	Walk(me, func(e error) bool {
		visited = append(visited, e.Error())
		return true
	})

	// MultiError 自身 + 2 个子错误
	if len(visited) != 3 {
		t.Errorf("expected 3 visited, got %d: %v", len(visited), visited)
	}
}

func TestCollectErrors(t *testing.T) {
	// 全部成功
	err := CollectErrors(
		func() error { return nil },
		func() error { return nil },
	)
	if err != nil {
		t.Error("CollectErrors should return nil when all succeed")
	}

	// 部分失败
	err = CollectErrors(
		func() error { return nil },
		func() error { return errors.New("error 1") },
		func() error { return errors.New("error 2") },
	)
	if err == nil {
		t.Error("CollectErrors should return error")
	}
	me, ok := err.(*MultiError)
	if !ok {
		t.Error("CollectErrors should return *MultiError")
	}
	if me.Len() != 2 {
		t.Errorf("expected 2 errors, got %d", me.Len())
	}
}

func TestCombineErrors(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	// 全部 nil
	err := CombineErrors(nil, nil)
	if err != nil {
		t.Error("CombineErrors should return nil for all nil")
	}

	// 有非 nil
	err = CombineErrors(err1, nil, err2)
	if err == nil {
		t.Error("CombineErrors should return error")
	}
	me, ok := err.(*MultiError)
	if !ok {
		t.Error("CombineErrors should return *MultiError")
	}
	if me.Len() != 2 {
		t.Errorf("expected 2 errors, got %d", me.Len())
	}
}

func TestAppendResult(t *testing.T) {
	me := NewMultiError()

	// 模拟函数返回值
	me.AppendResult("ignored", nil)
	me.AppendResult(123, errors.New("error"))

	if me.Len() != 1 {
		t.Errorf("expected 1 error, got %d", me.Len())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
