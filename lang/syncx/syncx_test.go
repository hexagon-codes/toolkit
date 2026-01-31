package syncx

import (
	"bytes"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSingleflightBasic 测试 Singleflight 基本功能
func TestSingleflightBasic(t *testing.T) {
	sf := NewSingleflight()
	var calls int32

	fn := func() (any, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(10 * time.Millisecond)
		return "result", nil
	}

	// 单个调用
	result, err := sf.Do("key1", fn)
	if err != nil {
		t.Errorf("Do() returned error: %v", err)
	}
	if result != "result" {
		t.Errorf("Do() = %v, want result", result)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("function called %d times, want 1", calls)
	}
}

// TestSingleflightDuplicate 测试并发重复调用
func TestSingleflightDuplicate(t *testing.T) {
	sf := NewSingleflight()
	var calls int32

	fn := func() (any, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		return "result", nil
	}

	// 并发调用相同的 key
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	results := make([]any, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			result, err := sf.Do("key1", fn)
			if err != nil {
				t.Errorf("Do() returned error: %v", err)
			}
			results[i] = result
		}()
	}

	wg.Wait()

	// 验证只调用了一次
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("function called %d times, want 1", calls)
	}

	// 验证所有结果相同
	for i, result := range results {
		if result != "result" {
			t.Errorf("result[%d] = %v, want result", i, result)
		}
	}
}

// TestSingleflightError 测试错误处理
func TestSingleflightError(t *testing.T) {
	sf := NewSingleflight()
	expectedErr := errors.New("test error")

	fn := func() (any, error) {
		return nil, expectedErr
	}

	result, err := sf.Do("key1", fn)
	if err != expectedErr {
		t.Errorf("Do() error = %v, want %v", err, expectedErr)
	}
	if result != nil {
		t.Errorf("Do() result = %v, want nil", result)
	}
}

// TestSingleflightForget 测试 Forget 功能
func TestSingleflightForget(t *testing.T) {
	sf := NewSingleflight()
	var calls int32

	fn := func() (any, error) {
		atomic.AddInt32(&calls, 1)
		return "result", nil
	}

	// 第一次调用
	sf.Do("key1", fn)

	// Forget 后再次调用
	sf.Forget("key1")
	sf.Do("key1", fn)

	// 应该调用了 2 次
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("function called %d times, want 2", calls)
	}
}

// TestSingleflightDifferentKeys 测试不同 key 的调用
func TestSingleflightDifferentKeys(t *testing.T) {
	sf := NewSingleflight()
	var calls int32

	fn := func() (any, error) {
		atomic.AddInt32(&calls, 1)
		return "result", nil
	}

	// 不同 key 应该分别执行
	sf.Do("key1", fn)
	sf.Do("key2", fn)

	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("function called %d times, want 2", calls)
	}
}

// TestPoolBasic 测试 Pool 基本功能
func TestPoolBasic(t *testing.T) {
	var creates int32
	pool := NewPool(func() any {
		atomic.AddInt32(&creates, 1)
		return &bytes.Buffer{}
	})

	// 获取对象
	buf1 := pool.Get().(*bytes.Buffer)
	if buf1 == nil {
		t.Fatal("Get() returned nil")
	}

	// 使用对象
	buf1.WriteString("hello")
	if buf1.String() != "hello" {
		t.Errorf("Buffer content = %s, want hello", buf1.String())
	}

	// 放回池中
	buf1.Reset()
	pool.Put(buf1)

	// 再次获取（可能是同一个对象）
	buf2 := pool.Get().(*bytes.Buffer)
	if buf2 == nil {
		t.Fatal("Get() returned nil after Put")
	}

	// 验证对象被重置
	if buf2.Len() != 0 {
		t.Errorf("Buffer length = %d, want 0 (should be reset)", buf2.Len())
	}
}

// TestTypedPoolBasic 测试 TypedPool 基本功能
func TestTypedPoolBasic(t *testing.T) {
	pool := NewTypedPool(func() *bytes.Buffer {
		return &bytes.Buffer{}
	})

	// 获取对象（无需类型断言）
	buf := pool.Get()
	if buf == nil {
		t.Fatal("Get() returned nil")
	}

	// 使用对象
	buf.WriteString("world")
	if buf.String() != "world" {
		t.Errorf("Buffer content = %s, want world", buf.String())
	}

	// 放回池中
	buf.Reset()
	pool.Put(buf)

	// 再次获取
	buf2 := pool.Get()
	if buf2 == nil {
		t.Fatal("Get() returned nil after Put")
	}
}

// TestTypedPoolConcurrent 测试 TypedPool 并发安全
func TestTypedPoolConcurrent(t *testing.T) {
	pool := NewTypedPool(func() *bytes.Buffer {
		return &bytes.Buffer{}
	})

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				buf := pool.Get()
				buf.WriteString("test")
				buf.Reset()
				pool.Put(buf)
			}
		}()
	}

	wg.Wait()
}

// BenchmarkSingleflight 基准测试
func BenchmarkSingleflight(b *testing.B) {
	sf := NewSingleflight()
	fn := func() (any, error) {
		return "result", nil
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sf.Do("key", fn)
		}
	})
}

// BenchmarkSingleflightUnique 基准测试（唯一 key）
func BenchmarkSingleflightUnique(b *testing.B) {
	sf := NewSingleflight()
	fn := func() (any, error) {
		return "result", nil
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sf.Do(string(rune(i)), fn)
			i++
		}
	})
}

// BenchmarkTypedPool 基准测试
func BenchmarkTypedPool(b *testing.B) {
	pool := NewTypedPool(func() *bytes.Buffer {
		return &bytes.Buffer{}
	})

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get()
			buf.WriteString("test")
			buf.Reset()
			pool.Put(buf)
		}
	})
}

// BenchmarkTypedPoolVsNew 对比基准测试
func BenchmarkTypedPoolVsNew(b *testing.B) {
	b.Run("Pool", func(b *testing.B) {
		pool := NewTypedPool(func() *bytes.Buffer {
			return &bytes.Buffer{}
		})
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := pool.Get()
				buf.WriteString("test")
				buf.Reset()
				pool.Put(buf)
			}
		})
	})

	b.Run("New", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := &bytes.Buffer{}
				buf.WriteString("test")
			}
		})
	})
}
