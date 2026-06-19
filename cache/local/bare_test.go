package local

import (
	"context"
	"testing"
	"time"
)

// TestCache_BareSetGet_Basic 验证裸 Set/Get 的基本读写与类型保持。
func TestCache_BareSetGet_Basic(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{"字符串值", "k:str", "hello"},
		{"整数值", "k:int", 42},
		{"结构体值", "k:struct", User{ID: 1, Name: "Alice"}},
		{"指针值", "k:ptr", &User{ID: 2, Name: "Bob"}},
		{"切片值", "k:slice", []int{1, 2, 3}},
		{"map 值", "k:map", map[string]int{"a": 1}},
		{"nil 值", "k:nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCacheWithCleanup(100, -1)
			defer cache.Stop()

			cache.Set(tt.key, tt.value, 10*time.Minute)

			got, ok := cache.Get(tt.key)
			if !ok {
				t.Fatalf("Get(%q) ok=false, want true", tt.key)
			}
			// 直接比较：裸 API 返回原始 any，类型与值都应原样保持
			if !equalAny(got, tt.value) {
				t.Errorf("Get(%q)=%#v, want %#v", tt.key, got, tt.value)
			}
		})
	}
}

// TestCache_BareGet_Miss 验证未命中、空 key 的返回。
func TestCache_BareGet_Miss(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1)
	defer cache.Stop()

	tests := []struct {
		name string
		key  string
	}{
		{"不存在的 key", "no-such-key"},
		{"空 key", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cache.Get(tt.key)
			if ok {
				t.Errorf("Get(%q) ok=true, want false", tt.key)
			}
			if got != nil {
				t.Errorf("Get(%q)=%#v, want nil", tt.key, got)
			}
		})
	}
}

// TestCache_BareSet_EmptyKey 验证空 key 写入为安全空操作。
func TestCache_BareSet_EmptyKey(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1)
	defer cache.Stop()

	cache.Set("", "value", time.Minute)
	if cache.Len() != 0 {
		t.Errorf("空 key 写入后 Len=%d, want 0", cache.Len())
	}

	_, ok := cache.Get("")
	if ok {
		t.Error("空 key Get 应返回 ok=false")
	}
}

// TestCache_BareSet_Overwrite 验证同 key 覆盖写。
func TestCache_BareSet_Overwrite(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1)
	defer cache.Stop()

	cache.Set("k", "v1", time.Minute)
	cache.Set("k", "v2", time.Minute)

	got, ok := cache.Get("k")
	if !ok || got != "v2" {
		t.Errorf("覆盖写后 Get=%v ok=%v, want v2 true", got, ok)
	}
	if cache.Len() != 1 {
		t.Errorf("覆盖写不应新增条目, Len=%d want 1", cache.Len())
	}
}

// TestCache_BareSet_TTLExpiration 验证 ttl>0 时的过期行为（含惰性删除）。
func TestCache_BareSet_TTLExpiration(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }

	cache := NewCacheWithCleanup(100, -1, WithNow(mockNow), WithJitter(0))
	defer cache.Stop()

	cache.Set("k", "v", time.Minute)

	// 未过期
	now = now.Add(30 * time.Second)
	if got, ok := cache.Get("k"); !ok || got != "v" {
		t.Errorf("30s 未过期: Get=%v ok=%v, want v true", got, ok)
	}

	// 已过期
	now = now.Add(31 * time.Second)
	if got, ok := cache.Get("k"); ok {
		t.Errorf("61s 已过期: Get=%v ok=%v, want _ false", got, ok)
	}
	// 惰性删除：过期读后条目应被移除
	if cache.Len() != 0 {
		t.Errorf("过期读取后应惰性删除, Len=%d want 0", cache.Len())
	}
}

// TestCache_BareSet_NoExpiry 验证 ttl<=0 表示"永不过期"（仅受 LRU 约束）。
func TestCache_BareSet_NoExpiry(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }

	for _, ttl := range []time.Duration{0, -time.Minute} {
		cache := NewCacheWithCleanup(100, -1, WithNow(mockNow), WithJitter(0))

		cache.Set("k", "v", ttl)

		// 应写入成功（区别于 loader 路径 setItem 的 ttl<=0 丢弃写入）
		if cache.Len() != 1 {
			t.Errorf("ttl=%v 应写入, Len=%d want 1", ttl, cache.Len())
		}

		// 时间大幅推进后仍存活
		now = now.Add(1000 * time.Hour)
		if got, ok := cache.Get("k"); !ok || got != "v" {
			t.Errorf("ttl=%v 应永不过期: Get=%v ok=%v, want v true", ttl, got, ok)
		}

		now = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		cache.Stop()
	}
}

// TestCache_BareGet_RefreshesLRU 验证裸 Get 命中会刷新 LRU 访问时间。
func TestCache_BareGet_RefreshesLRU(t *testing.T) {
	cache := NewCacheWithCleanup(3, -1) // 容量 3，无过期写入
	defer cache.Stop()

	cache.Set("a", "va", 0)
	time.Sleep(5 * time.Millisecond)
	cache.Set("b", "vb", 0)
	time.Sleep(5 * time.Millisecond)
	cache.Set("c", "vc", 0)

	// 访问 a，使其成为最近使用，b 变为最久未使用
	time.Sleep(5 * time.Millisecond)
	if _, ok := cache.Get("a"); !ok {
		t.Fatal("a 应命中")
	}

	// 写入第 4 条，触发淘汰，最久未访问的 b 应被淘汰
	time.Sleep(5 * time.Millisecond)
	cache.Set("d", "vd", 0)

	if cache.Len() != 3 {
		t.Fatalf("淘汰后 Len=%d want 3", cache.Len())
	}
	if _, ok := cache.Get("a"); !ok {
		t.Error("a 最近被访问，不应被淘汰")
	}
	if _, ok := cache.Get("b"); ok {
		t.Error("b 最久未访问，应被淘汰")
	}
	if _, ok := cache.Get("d"); !ok {
		t.Error("d 刚写入，应存在")
	}
}

// TestCache_BareSet_MaxEntriesEviction 验证容量上限触发 LRU 淘汰，
// 并验证 maxEntries<=0 被规整为默认上限（构造层既有行为）。
func TestCache_BareSet_MaxEntriesEviction(t *testing.T) {
	// maxEntries<=0 -> DefaultMaxEntries，确保始终有上限（防 OOM）
	cache := NewCacheWithCleanup(0, -1)
	defer cache.Stop()
	if cache.maxEntries != DefaultMaxEntries {
		t.Fatalf("maxEntries<=0 应规整为 %d, got %d", DefaultMaxEntries, cache.maxEntries)
	}

	// 小容量验证淘汰
	small := NewCacheWithCleanup(2, -1)
	defer small.Stop()
	small.Set("a", 1, 0)
	time.Sleep(2 * time.Millisecond)
	small.Set("b", 2, 0)
	time.Sleep(2 * time.Millisecond)
	small.Set("c", 3, 0) // 触发淘汰 a

	if small.Len() != 2 {
		t.Errorf("容量 2 写入 3 条后 Len=%d want 2", small.Len())
	}
	if _, ok := small.Get("a"); ok {
		t.Error("a 应被淘汰")
	}
}

// TestCache_BarePrefix 验证裸 API 同样应用 Prefix。
func TestCache_BarePrefix(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1, WithPrefix("p"))
	defer cache.Stop()

	cache.Set("k", "v", time.Minute)

	// 通过 Get 读回（内部应用同一前缀）
	if got, ok := cache.Get("k"); !ok || got != "v" {
		t.Errorf("带前缀 Get=%v ok=%v, want v true", got, ok)
	}

	// 底层存储 key 应带前缀
	cache.mu.RLock()
	_, exists := cache.items["p:k"]
	cache.mu.RUnlock()
	if !exists {
		t.Error("底层 key 应为 p:k")
	}
}

// TestCache_BareDel 验证 Del 可删除裸条目。
func TestCache_BareDel(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1)
	defer cache.Stop()

	cache.Set("k", "v", time.Minute)
	if err := cache.Del(context.Background(), "k"); err != nil {
		t.Fatalf("Del 失败: %v", err)
	}
	if _, ok := cache.Get("k"); ok {
		t.Error("Del 后应未命中")
	}
}

// TestCache_BareClear 验证 Clear 清空裸条目。
func TestCache_BareClear(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1)
	defer cache.Stop()

	cache.Set("k1", "v1", time.Minute)
	cache.Set("k2", "v2", time.Minute)
	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Clear 后 Len=%d want 0", cache.Len())
	}
	if _, ok := cache.Get("k1"); ok {
		t.Error("Clear 后 k1 应未命中")
	}
}

// TestCache_BareAndLoaderIsolation 验证裸路径与 loader 路径互不串读。
func TestCache_BareAndLoaderIsolation(t *testing.T) {
	cache := NewCacheWithCleanup(100, -1)
	defer cache.Stop()
	ctx := context.Background()

	// loader 路径写入 "shared"
	var u User
	if err := cache.GetOrLoad(ctx, "shared", time.Minute, &u, func(ctx context.Context) (any, error) {
		return User{ID: 7, Name: "Loaded"}, nil
	}); err != nil {
		t.Fatalf("GetOrLoad 失败: %v", err)
	}

	// 裸 Get 不应读到 loader 路径写入的条目
	if got, ok := cache.Get("shared"); ok {
		t.Errorf("裸 Get 不应读到 loader 条目, got=%#v", got)
	}

	// 裸 Set 写入不同 key
	cache.Set("bare", "bv", time.Minute)

	// loader 路径的 double-check 读取（getItem）不应读到裸条目，会重新调用 loader
	loaded := false
	var s string
	if err := cache.GetOrLoad(ctx, "bare", time.Minute, &s, func(ctx context.Context) (any, error) {
		loaded = true
		return "from-loader", nil
	}); err != nil {
		t.Fatalf("GetOrLoad(bare) 失败: %v", err)
	}
	if !loaded {
		t.Error("loader 路径不应命中裸条目，应触发 loader")
	}
}

// equalAny 用于测试中比较 any 值（覆盖切片/map 等不可直接 == 的类型）。
func equalAny(a, b any) bool {
	switch av := a.(type) {
	case []int:
		bv, ok := b.([]int)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
		return true
	case map[string]int:
		bv, ok := b.(map[string]int)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if bv[k] != v {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
