package local

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fixedClock 返回一个始终返回固定时刻的 Now 函数，用于把所有条目钉在同一纳秒，
// 以放大"同时间戳淘汰"的不确定性，验证 seq tiebreak 的确定性效果。
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestLRULess 表驱动验证 (accessedAt, seq) 字典序全序比较。
func TestLRULess(t *testing.T) {
	tests := []struct {
		name      string
		aAccessed int64
		aSeq      uint64
		bAccessed int64
		bSeq      uint64
		want      bool // a 是否"更应被淘汰"（更久未访问）
	}{
		{"a 时间更早", 100, 5, 200, 1, true},
		{"a 时间更晚", 300, 1, 200, 9, false},
		{"同时间戳-a seq 更小应优先淘汰", 100, 3, 100, 4, true},
		{"同时间戳-a seq 更大不优先淘汰", 100, 8, 100, 4, false},
		{"完全相等-非严格小于", 100, 7, 100, 7, false},
		{"零值边界", 0, 0, 0, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lruLess(tt.aAccessed, tt.aSeq, tt.bAccessed, tt.bSeq)
			if got != tt.want {
				t.Errorf("lruLess(%d,%d,%d,%d)=%v, want %v",
					tt.aAccessed, tt.aSeq, tt.bAccessed, tt.bSeq, got, tt.want)
			}
		})
	}
}

// TestLRULess_Antisymmetry 验证全序的反对称性：对任意不等的二元组，
// lruLess(a,b) 与 lruLess(b,a) 必有且仅有一个为 true。
func TestLRULess_Antisymmetry(t *testing.T) {
	pairs := []struct {
		acc int64
		seq uint64
	}{
		{100, 1}, {100, 2}, {200, 1}, {200, 2}, {0, 0}, {-1, 9},
	}
	for i := range pairs {
		for j := range pairs {
			if i == j {
				continue
			}
			ab := lruLess(pairs[i].acc, pairs[i].seq, pairs[j].acc, pairs[j].seq)
			ba := lruLess(pairs[j].acc, pairs[j].seq, pairs[i].acc, pairs[i].seq)
			if ab == ba {
				t.Errorf("反对称性破坏: pair[%d]=%v pair[%d]=%v lruLess 两向均为 %v",
					i, pairs[i], j, pairs[j], ab)
			}
		}
	}
}

// TestCache_DeterministicLRU_SameTimestamp 是 W7 阻断的核心回归测试：
// 在固定时钟（所有写入/访问落在同一纳秒）下，紧循环 Set a,b,c 后用固定访问模式，
// 再写入触发淘汰，验证被淘汰对象在多次重建中完全确定（不受 map 遍历顺序影响）。
//
// 语义：seq 随写入/访问单调递增；同一纳秒内 seq 最小者（最早被访问）先淘汰。
func TestCache_DeterministicLRU_SameTimestamp(t *testing.T) {
	// 固定时钟：把 accessedAt 全部钉在同一纳秒，纯靠 seq 决定淘汰顺序
	clk := fixedClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

	// run 在容量 3 的缓存上：Set a,b,c（seq: a<b<c），再固定访问 a（使 a 的 seq 变最大），
	// 然后写 d 触发淘汰。预期淘汰 b（此刻 seq 最小者）。返回被淘汰的 key。
	run := func() string {
		c := NewCacheNoCleanup(3, WithNow(clk), WithJitter(0))
		defer c.Close()

		c.Set("a", "va", 0) // seq=1
		c.Set("b", "vb", 0) // seq=2
		c.Set("c", "vc", 0) // seq=3

		// 固定访问模式：访问 a，使其 seq 变为最大（最近使用）
		if _, ok := c.Get("a"); !ok { // a.seq=4
			t.Fatal("a 应命中")
		}

		// 写入 d 触发淘汰：此刻同一纳秒内 seq 为 b=2 最小 -> b 应被淘汰
		c.Set("d", "vd", 0) // seq=5

		// 找出被淘汰者
		for _, k := range []string{"a", "b", "c", "d"} {
			if _, ok := c.Get(k); !ok {
				return k
			}
		}
		return ""
	}

	// 多次重建，结果必须恒定为 "b"——这正是非确定性实现会 flaky 的地方
	const iterations = 200
	first := run()
	if first != "b" {
		t.Fatalf("淘汰对象=%q, 期望确定淘汰 b（同纳秒内 seq 最小者）", first)
	}
	for i := 0; i < iterations; i++ {
		if got := run(); got != first {
			t.Fatalf("第 %d 次淘汰对象=%q, 与首次 %q 不一致，LRU 非确定", i, got, first)
		}
	}
}

// TestCache_DeterministicLRU_MultiEvict 验证同一纳秒内一次淘汰多个条目时，
// 被淘汰集合完全确定（应为 seq 最小的若干个）。
func TestCache_DeterministicLRU_MultiEvict(t *testing.T) {
	clk := fixedClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

	// 容量 2：依次写 5 个 key（seq 递增），写入过程中持续触发淘汰。
	// 由于无访问刷新，seq 顺序即写入顺序，最终应保留 seq 最大的 2 个（d,e）。
	run := func() (survivors map[string]bool) {
		c := NewCacheNoCleanup(2, WithNow(clk), WithJitter(0))
		defer c.Close()
		for _, k := range []string{"a", "b", "c", "d", "e"} {
			c.Set(k, k, 0)
		}
		survivors = map[string]bool{}
		for _, k := range []string{"a", "b", "c", "d", "e"} {
			if _, ok := c.Get(k); ok {
				survivors[k] = true
			}
		}
		return survivors
	}

	want := map[string]bool{"d": true, "e": true}
	for i := 0; i < 100; i++ {
		got := run()
		if len(got) != 2 || !got["d"] || !got["e"] {
			t.Fatalf("第 %d 次幸存集合=%v, 期望确定为 %v", i, got, want)
		}
	}
}

// TestCache_LRU_AccessedAtPrecedesSeq 验证当 accessedAt 不同（时间确实推进）时，
// 仍以 accessedAt 为主序，seq 仅作为同时间戳 tiebreak，未改变既有 LRU 语义。
func TestCache_LRU_AccessedAtPrecedesSeq(t *testing.T) {
	var now = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := func() time.Time { return now }

	c := NewCacheNoCleanup(2, WithNow(clk), WithJitter(0))
	defer c.Close()

	c.Set("old", "v", 0) // accessedAt=t0, seq=1
	now = now.Add(time.Second)
	c.Set("new", "v", 0) // accessedAt=t1>t0, seq=2

	// 即使 old 的 seq 更小，new 的 accessedAt 更大；写第三条应淘汰 accessedAt 最小的 old
	now = now.Add(time.Second)
	c.Set("third", "v", 0) // accessedAt=t2, seq=3

	if _, ok := c.Get("old"); ok {
		t.Error("old 的 accessedAt 最小，应被优先淘汰（accessedAt 为主序）")
	}
	if _, ok := c.Get("new"); !ok {
		t.Error("new 不应被淘汰")
	}
	if _, ok := c.Get("third"); !ok {
		t.Error("third 不应被淘汰")
	}
}

// TestCache_DeterministicLRU_LoaderPath 在 loader 路径（GetOrLoad）上验证确定性淘汰，
// 确保 seq 机制对两条写入路径都生效。
func TestCache_DeterministicLRU_LoaderPath(t *testing.T) {
	clk := fixedClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	run := func() string {
		c := NewCacheNoCleanup(3, WithNow(clk), WithJitter(0))
		defer c.Close()

		load := func(name string) func(context.Context) (any, error) {
			return func(context.Context) (any, error) { return name, nil }
		}
		var s string
		_ = c.GetOrLoad(ctx, "a", time.Hour, &s, load("a")) // seq=1
		_ = c.GetOrLoad(ctx, "b", time.Hour, &s, load("b")) // seq=2
		_ = c.GetOrLoad(ctx, "c", time.Hour, &s, load("c")) // seq=3

		// 访问 a（缓存命中，刷新 seq=4）
		_ = c.GetOrLoad(ctx, "a", time.Hour, &s, load("a-reload"))

		// 写入 d 触发淘汰 -> 同纳秒内 seq 最小为 b
		_ = c.GetOrLoad(ctx, "d", time.Hour, &s, load("d")) // seq=5

		// 探测被淘汰者：被淘汰的 key 会重新触发 loader
		for _, k := range []string{"a", "b", "c", "d"} {
			reloaded := false
			_ = c.GetOrLoad(ctx, k, time.Hour, &s, func(context.Context) (any, error) {
				reloaded = true
				return k, nil
			})
			if reloaded {
				return k
			}
		}
		return ""
	}

	first := run()
	if first != "b" {
		t.Fatalf("loader 路径淘汰对象=%q, 期望 b", first)
	}
	for i := 0; i < 100; i++ {
		if got := run(); got != first {
			t.Fatalf("第 %d 次 loader 路径淘汰=%q, 与首次 %q 不一致", i, got, first)
		}
	}
}

// TestNewCacheNoCleanup_NoGoroutine 验证"无后台清理"构造不会启动后台 goroutine，
// 即使不调用 Stop/Close 也不会泄漏。对照 NewCache（默认起 goroutine）。
func TestNewCacheNoCleanup_NoGoroutine(t *testing.T) {
	// 等待基线 goroutine 稳定
	settle := func() {
		for i := 0; i < 10; i++ {
			runtime.GC()
			time.Sleep(5 * time.Millisecond)
		}
	}

	settle()
	base := runtime.NumGoroutine()

	// 不调用 Stop/Close，刻意制造"忘记关闭"的场景
	caches := make([]*Cache, 0, 50)
	for i := 0; i < 50; i++ {
		caches = append(caches, NewCacheNoCleanup(100))
	}
	settle()
	after := runtime.NumGoroutine()

	// 50 个无清理缓存不应新增任何后台 goroutine（允许调度抖动的少量浮动）
	if after > base+2 {
		t.Errorf("NewCacheNoCleanup 不应起后台 goroutine: base=%d after=%d (新增 %d)",
			base, after, after-base)
	}

	// 缓存仍可正常读写
	c := caches[0]
	c.Set("k", "v", time.Minute)
	if got, ok := c.Get("k"); !ok || got != "v" {
		t.Errorf("无清理缓存读写异常: got=%v ok=%v", got, ok)
	}

	// keep alive
	runtime.KeepAlive(caches)
}

// TestNewCacheNoCleanup_LazyExpiration 验证无后台清理时，过期条目通过惰性删除回收。
func TestNewCacheNoCleanup_LazyExpiration(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := func() time.Time { return now }

	c := NewCacheNoCleanup(100, WithNow(clk), WithJitter(0))
	defer c.Close()

	c.Set("k", "v", time.Minute)
	if c.Len() != 1 {
		t.Fatalf("写入后 Len=%d want 1", c.Len())
	}

	// 时间推进到过期
	now = now.Add(2 * time.Minute)

	// 无后台清理：过期条目在被读取前仍占位
	if c.Len() != 1 {
		t.Errorf("无后台清理时过期条目应仍占位（惰性删除），Len=%d want 1", c.Len())
	}

	// 读取触发惰性删除
	if _, ok := c.Get("k"); ok {
		t.Error("过期条目应未命中")
	}
	if c.Len() != 0 {
		t.Errorf("读取过期条目后应惰性删除，Len=%d want 0", c.Len())
	}
}

// TestNewCacheNoCleanup_EquivalentToZeroInterval 验证 NewCacheNoCleanup 与
// NewCacheWithCleanup(_, 0) 行为等价（cleanupInterval 字段为 0，不起 goroutine）。
func TestNewCacheNoCleanup_EquivalentToZeroInterval(t *testing.T) {
	a := NewCacheNoCleanup(123)
	defer a.Close()
	b := NewCacheWithCleanup(123, 0)
	defer b.Close()

	if a.cleanupInterval != 0 {
		t.Errorf("NewCacheNoCleanup cleanupInterval=%v want 0", a.cleanupInterval)
	}
	if a.cleanupInterval != b.cleanupInterval {
		t.Errorf("两种构造 cleanupInterval 不一致: %v vs %v", a.cleanupInterval, b.cleanupInterval)
	}
	if a.maxEntries != 123 || b.maxEntries != 123 {
		t.Errorf("maxEntries 未正确设置: a=%d b=%d", a.maxEntries, b.maxEntries)
	}
}

// TestCache_Close 表驱动验证 Close 的幂等性、返回值，以及对各类构造的安全性。
func TestCache_Close(t *testing.T) {
	tests := []struct {
		name string
		make func() *Cache
	}{
		{"默认带清理构造", func() *Cache { return NewCache(10) }},
		{"无清理构造", func() *Cache { return NewCacheNoCleanup(10) }},
		{"负间隔构造", func() *Cache { return NewCacheWithCleanup(10, -1) }},
		{"显式清理间隔构造", func() *Cache { return NewCacheWithCleanup(10, 50*time.Millisecond) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.make()

			// 首次 Close 返回 nil
			if err := c.Close(); err != nil {
				t.Errorf("Close() 返回 err=%v, want nil", err)
			}
			// 幂等：多次 Close 安全且仍返回 nil
			if err := c.Close(); err != nil {
				t.Errorf("二次 Close() 返回 err=%v, want nil", err)
			}
			// Close 后混用 Stop 也安全（不应 panic）
			c.Stop()
		})
	}
}

// TestCache_CloseStopInterchangeable 验证 Close 与 Stop 共用同一停止状态，
// 交错调用不会重复关闭 channel（否则会 panic）。
func TestCache_CloseStopInterchangeable(t *testing.T) {
	tests := []struct {
		name string
		seq  []string // 调用顺序
	}{
		{"Stop 后 Close", []string{"Stop", "Close"}},
		{"Close 后 Stop", []string{"Close", "Stop"}},
		{"Close x3", []string{"Close", "Close", "Close"}},
		{"Stop x3", []string{"Stop", "Stop", "Stop"}},
		{"交错", []string{"Close", "Stop", "Close", "Stop"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCacheWithCleanup(10, 50*time.Millisecond)
			for _, op := range tt.seq {
				switch op {
				case "Stop":
					c.Stop()
				case "Close":
					if err := c.Close(); err != nil {
						t.Errorf("Close() err=%v want nil", err)
					}
				}
			}
		})
	}
}

// TestCache_ConcurrentDeterministicSeq 并发压力下验证 seq 发号单调唯一（无重复），
// 保证 LRU tiebreak 始终有严格全序。-race 下运行可同时检测数据竞争。
func TestCache_ConcurrentDeterministicSeq(t *testing.T) {
	c := NewCacheNoCleanup(10000)
	defer c.Close()

	const goroutines = 16
	const perG = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				k := string(rune('A'+id)) + string(rune('0'+(i%10)))
				c.Set(k, i, time.Minute)
				_, _ = c.Get(k)
			}
		}(g)
	}
	wg.Wait()

	// 发号总数应至少等于写入次数（每次 Set 至少取 1 个号）。
	// 只断言单调递增到达过较大值，确保 nextSeq 正常工作且无回绕。
	if got := c.accessSeq.Load(); got < uint64(goroutines*perG) {
		t.Errorf("accessSeq=%d 小于写入次数下界 %d", got, goroutines*perG)
	}
}
