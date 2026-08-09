package syncx

import (
	"sync"
	"testing"
)

func TestConcurrentMap_Basic(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	// Store and Load
	m.Store("a", 1)
	m.Store("b", 2)

	v, ok := m.Load("a")
	if !ok || v != 1 {
		t.Errorf("expected 1, got %v, ok=%v", v, ok)
	}

	v, ok = m.Load("c")
	if ok {
		t.Errorf("expected not found, got %v", v)
	}
}

func TestConcurrentMap_Delete(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)

	m.Delete("a")

	_, ok := m.Load("a")
	if ok {
		t.Error("expected key to be deleted")
	}
}

func TestConcurrentMap_LoadOrStore(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	// 新键
	v, loaded := m.LoadOrStore("a", 1)
	if loaded || v != 1 {
		t.Errorf("expected loaded=false, v=1, got loaded=%v, v=%v", loaded, v)
	}

	// 已存在的键
	v, loaded = m.LoadOrStore("a", 2)
	if !loaded || v != 1 {
		t.Errorf("expected loaded=true, v=1, got loaded=%v, v=%v", loaded, v)
	}
}

func TestConcurrentMap_LoadAndDelete(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)

	v, ok := m.LoadAndDelete("a")
	if !ok || v != 1 {
		t.Errorf("expected ok=true, v=1, got ok=%v, v=%v", ok, v)
	}

	_, ok = m.Load("a")
	if ok {
		t.Error("expected key to be deleted")
	}

	_, ok = m.LoadAndDelete("nonexistent")
	if ok {
		t.Error("expected ok=false for nonexistent key")
	}
}

func TestConcurrentMap_Swap(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)

	prev, loaded := m.Swap("a", 2)
	if !loaded || prev != 1 {
		t.Errorf("expected loaded=true, prev=1, got loaded=%v, prev=%v", loaded, prev)
	}

	v, _ := m.Load("a")
	if v != 2 {
		t.Errorf("expected 2, got %v", v)
	}

	_, loaded = m.Swap("b", 3)
	if loaded {
		t.Errorf("expected loaded=false for new key, got loaded=%v", loaded)
	}
}

func TestConcurrentMap_Range(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)
	m.Store("b", 2)
	m.Store("c", 3)

	sum := 0
	m.Range(func(k string, v int) bool {
		sum += v
		return true
	})

	if sum != 6 {
		t.Errorf("expected sum=6, got %d", sum)
	}

	// 测试提前停止
	count := 0
	m.Range(func(k string, v int) bool {
		count++
		return false // 停止
	})

	if count != 1 {
		t.Errorf("expected count=1 with early stop, got %d", count)
	}
}

func TestConcurrentMap_Len(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	if m.Len() != 0 {
		t.Errorf("expected len=0, got %d", m.Len())
	}

	m.Store("a", 1)
	m.Store("b", 2)

	if m.Len() != 2 {
		t.Errorf("expected len=2, got %d", m.Len())
	}
}

func TestConcurrentMap_IsEmpty(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	if !m.IsEmpty() {
		t.Error("expected empty")
	}

	m.Store("a", 1)

	if m.IsEmpty() {
		t.Error("expected not empty")
	}
}

func TestConcurrentMap_Keys(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)
	m.Store("b", 2)

	keys := m.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	// 检查所有键存在
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	if !keySet["a"] || !keySet["b"] {
		t.Error("missing expected keys")
	}
}

func TestConcurrentMap_Values(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)
	m.Store("b", 2)

	values := m.Values()
	if len(values) != 2 {
		t.Errorf("expected 2 values, got %d", len(values))
	}

	sum := 0
	for _, v := range values {
		sum += v
	}
	if sum != 3 {
		t.Errorf("expected sum=3, got %d", sum)
	}
}

func TestConcurrentMap_Clear(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)
	m.Store("b", 2)

	m.Clear()

	if !m.IsEmpty() {
		t.Error("expected empty after clear")
	}
}

func TestConcurrentMap_GetOrCompute(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	computeCount := 0
	compute := func() int {
		computeCount++
		return 42
	}

	// 第一次调用应该计算
	v := m.GetOrCompute("a", compute)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if computeCount != 1 {
		t.Errorf("expected computeCount=1, got %d", computeCount)
	}

	// 第二次调用应该返回缓存
	v = m.GetOrCompute("a", compute)
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if computeCount != 1 {
		t.Errorf("expected computeCount=1, got %d", computeCount)
	}
}

func TestConcurrentMap_Update(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("count", 1)

	ok := m.Update("count", func(v int) int { return v + 1 })
	if !ok {
		t.Error("expected ok=true")
	}

	v, _ := m.Load("count")
	if v != 2 {
		t.Errorf("expected 2, got %d", v)
	}

	// 不存在的键
	ok = m.Update("nonexistent", func(v int) int { return v + 1 })
	if ok {
		t.Error("expected ok=false for nonexistent key")
	}
}

func TestConcurrentMap_Has(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)

	if !m.Has("a") {
		t.Error("expected Has to return true for existing key")
	}

	if m.Has("b") {
		t.Error("expected Has to return false for nonexistent key")
	}
}

func TestConcurrentMap_SetIfAbsent(t *testing.T) {
	m := NewConcurrentMap[string, int]()

	// 设置新键
	if !m.SetIfAbsent("a", 1) {
		t.Error("expected SetIfAbsent to return true for new key")
	}

	// 已存在的键
	if m.SetIfAbsent("a", 2) {
		t.Error("expected SetIfAbsent to return false for existing key")
	}

	v, _ := m.Load("a")
	if v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestConcurrentMap_ToMap(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)
	m.Store("b", 2)

	regular := m.ToMap()

	if len(regular) != 2 {
		t.Errorf("expected len=2, got %d", len(regular))
	}
	if regular["a"] != 1 || regular["b"] != 2 {
		t.Error("map contents don't match")
	}
}

func TestConcurrentMap_Concurrent(t *testing.T) {
	m := NewConcurrentMap[int, int]()
	var wg sync.WaitGroup
	n := 1000

	// 并发写入
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.Store(i, i*2)
		}(i)
	}
	wg.Wait()

	if m.Len() != n {
		t.Errorf("expected len=%d, got %d", n, m.Len())
	}

	// 并发读取
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, ok := m.Load(i)
			if !ok || v != i*2 {
				t.Errorf("expected %d, got %v, ok=%v", i*2, v, ok)
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrentMap_ForEach(t *testing.T) {
	m := NewConcurrentMap[string, int]()
	m.Store("a", 1)
	m.Store("b", 2)

	sum := 0
	m.ForEach(func(k string, v int) {
		sum += v
	})

	if sum != 3 {
		t.Errorf("expected sum=3, got %d", sum)
	}
}
