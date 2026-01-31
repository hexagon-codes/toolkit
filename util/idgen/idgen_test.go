package idgen

import (
	"strings"
	"sync"
	"testing"
)

// UUID 测试
func TestUUID(t *testing.T) {
	id := UUID()

	if len(id) != 36 {
		t.Errorf("expected length 36, got %d", len(id))
	}

	// UUID 格式：8-4-4-4-12
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 parts, got %d", len(parts))
	}

	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Errorf("invalid UUID format: %s", id)
	}
}

func TestUUID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		id := UUID()
		if ids[id] {
			t.Errorf("duplicate UUID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("expected %d unique UUIDs, got %d", count, len(ids))
	}
}

func TestUUIDWithoutHyphen(t *testing.T) {
	id := UUIDWithoutHyphen()

	if len(id) != 32 {
		t.Errorf("expected length 32, got %d", len(id))
	}

	if strings.Contains(id, "-") {
		t.Errorf("UUID should not contain hyphens: %s", id)
	}
}

func TestMustUUID(t *testing.T) {
	id := MustUUID()

	if len(id) != 36 {
		t.Errorf("expected length 36, got %d", len(id))
	}
}

// Snowflake 测试
func TestNewSnowflake(t *testing.T) {
	sf, err := NewSnowflake(1)
	if err != nil {
		t.Fatalf("failed to create snowflake: %v", err)
	}

	if sf == nil {
		t.Fatal("snowflake is nil")
	}
}

func TestNewSnowflake_InvalidWorkerID(t *testing.T) {
	// 负数
	_, err := NewSnowflake(-1)
	if err == nil {
		t.Error("expected error for negative worker ID")
	}

	// 超过最大值
	_, err = NewSnowflake(1024)
	if err == nil {
		t.Error("expected error for worker ID > 1023")
	}
}

func TestSnowflake_Generate(t *testing.T) {
	sf, err := NewSnowflake(1)
	if err != nil {
		t.Fatalf("failed to create snowflake: %v", err)
	}

	id := sf.Generate()
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestSnowflake_Uniqueness(t *testing.T) {
	sf, _ := NewSnowflake(1)
	ids := make(map[int64]bool)
	count := 10000

	for i := 0; i < count; i++ {
		id := sf.Generate()
		if ids[id] {
			t.Errorf("duplicate Snowflake ID: %d", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestSnowflake_Monotonic(t *testing.T) {
	sf, _ := NewSnowflake(1)
	prevID := sf.Generate()

	for i := 0; i < 100; i++ {
		id := sf.Generate()
		if id <= prevID {
			t.Errorf("IDs are not monotonically increasing: %d <= %d", id, prevID)
		}
		prevID = id
	}
}

func TestSnowflake_Concurrent(t *testing.T) {
	sf, _ := NewSnowflake(1)
	count := 1000
	ids := make(chan int64, count)
	var wg sync.WaitGroup

	// 并发生成
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < count/10; j++ {
				ids <- sf.Generate()
			}
		}()
	}

	wg.Wait()
	close(ids)

	// 检查唯一性
	idMap := make(map[int64]bool)
	for id := range ids {
		if idMap[id] {
			t.Errorf("duplicate ID in concurrent test: %d", id)
		}
		idMap[id] = true
	}

	if len(idMap) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(idMap))
	}
}

func TestInitSnowflake(t *testing.T) {
	err := InitSnowflake(5)
	if err != nil {
		t.Fatalf("failed to init snowflake: %v", err)
	}

	id := SnowflakeID()
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

// NanoID 测试
func TestNanoID(t *testing.T) {
	id := NanoID()

	if len(id) != DefaultSize {
		t.Errorf("expected length %d, got %d", DefaultSize, len(id))
	}
}

func TestNanoID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		id := NanoID()
		if ids[id] {
			t.Errorf("duplicate NanoID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestNanoIDSize(t *testing.T) {
	tests := []int{8, 10, 16, 21, 32}

	for _, size := range tests {
		id := NanoIDSize(size)
		if len(id) != size {
			t.Errorf("expected length %d, got %d", size, len(id))
		}
	}
}

func TestNanoIDSize_ZeroSize(t *testing.T) {
	id := NanoIDSize(0)
	// 应该返回默认长度
	if len(id) != DefaultSize {
		t.Errorf("expected default length %d, got %d", DefaultSize, len(id))
	}
}

func TestNanoIDCustom(t *testing.T) {
	alphabet := "0123456789"
	size := 10

	id := NanoIDCustom(alphabet, size)

	if len(id) != size {
		t.Errorf("expected length %d, got %d", size, len(id))
	}

	// 验证只包含指定字符
	for _, char := range id {
		if !strings.ContainsRune(alphabet, char) {
			t.Errorf("ID contains invalid character: %c", char)
		}
	}
}

func TestNanoIDCustom_EmptyAlphabet(t *testing.T) {
	id := NanoIDCustom("", 10)

	// 应该使用默认字符集
	if len(id) != 10 {
		t.Errorf("expected length 10, got %d", len(id))
	}
}

func TestShortID(t *testing.T) {
	id := ShortID()

	if len(id) != 8 {
		t.Errorf("expected length 8, got %d", len(id))
	}
}

func TestMediumID(t *testing.T) {
	id := MediumID()

	if len(id) != 16 {
		t.Errorf("expected length 16, got %d", len(id))
	}
}

// Benchmark 测试
func BenchmarkUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		UUID()
	}
}

func BenchmarkSnowflake(b *testing.B) {
	sf, _ := NewSnowflake(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sf.Generate()
	}
}

func BenchmarkNanoID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NanoID()
	}
}

func BenchmarkNanoIDSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NanoIDSize(16)
	}
}
