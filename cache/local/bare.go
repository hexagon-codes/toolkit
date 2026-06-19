package local

import "time"

// 本文件提供裸（loader-free）的键值读写 API：Get / Set / SetWithTTL。
//
// 设计动机：
//
// 既有的 GetOrLoad / GetOrLoadEx 是"读穿透 + 回填"语义，必须传入 loader，
// 适合"缓存数据库查询结果"这类场景。但部分下游（如实现 llm.Cache 这类
// 裸 Get(key)/Set(key, val) 接口）只需要直接读写，并不需要 loader。
//
// 为此新增一组裸 API，直接以 any 形式存取调用方的值（不经 Codec 序列化），
// 复用底层同一份 map 存储、TTL 过期与 LRU 淘汰机制。
//
// 与 loader 路径的关系：
//   - 两条路径写入同一个 items map，但条目用 localItem.isRaw 区分。
//   - 裸 Get 只读取 isRaw=true 的条目；GetOrLoad 系列只读取 isRaw=false 的条目。
//   - 两者共享 maxEntries 容量上限与 LRU 淘汰，互不破坏既有行为。
//
// 线程安全：所有方法均为并发安全。

// Get 直接读取 key 对应的值（不经 loader）。
//
// 返回值：
//   - value: 命中时为之前通过 Set/SetWithTTL 写入的原始 any 值（保持原始类型，
//     不做任何序列化/反序列化）；未命中时为 nil。
//   - ok: true 表示命中且未过期；false 表示未命中、已过期或 key 为空。
//
// 行为说明：
//   - 命中时会刷新该条目的 LRU 访问时间（最近使用），延缓被淘汰。
//   - 过期条目视为未命中，且会被惰性删除。
//   - 仅能读到由裸 Set/SetWithTTL 写入的条目；通过 GetOrLoad 系列写入的条目
//     不会被本方法返回（两条路径的存储语义不同，互不干扰）。
//   - key 为空时直接返回 (nil, false)，不会 panic。
//
// 注意：返回的是写入时的同一个 any 值（引用），调用方若写入的是指针或可变类型
// （slice/map 等），请勿在缓存外部就地修改，以免影响后续读取者。
func (c *Cache) Get(key string) (value any, ok bool) {
	if key == "" {
		return nil, false
	}
	fullKey := joinPrefix(c.opts.Prefix, key)
	return c.getRawItem(fullKey)
}

// Set 写入 key=value，使用指定 ttl（不经 loader）。
//
// 这是 SetWithTTL 的同名别名，方法语义完全一致，提供更贴近裸 KV 习惯的命名。
//
// ttl 语义见 SetWithTTL。
func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.SetWithTTL(key, value, ttl)
}

// SetWithTTL 写入 key=value，使用指定 ttl（不经 loader）。
//
// value 以原始 any 形式存放，不经 Codec 序列化，因此 Get 可原样取回。
//
// ttl 语义（已明确定义）：
//   - ttl > 0：条目在 now+ttl 后过期。若启用了 Jitter（默认 0.1），实际过期时间
//     会在 [ttl, ttl*(1+jitter)] 区间内随机抖动，以缓解缓存雪崩。
//   - ttl <= 0：表示"永不按 TTL 过期"。条目会被写入并长期保留，仅在容量超过
//     maxEntries 时被 LRU 淘汰。注意这与 loader 路径中 setItem 的"ttl<=0 不写入"
//     行为不同：裸 Set 的语义是显式的"无过期写入"，而非"丢弃写入"。
//
// maxEntries 语义（已明确定义）：
//   - 构造缓存时若传入 maxEntries <= 0，会被规整为 DefaultMaxEntries（10000），
//     因此 LRU 淘汰始终生效，不存在"无上限"的退化场景，可防 OOM。
//   - 写入后若条目总数超过 maxEntries，会触发 LRU 淘汰：优先清理已过期条目，
//     仍超限则淘汰最久未访问（accessedAt 最小）的条目，直到不超过上限。
//     裸条目与 loader 条目共用同一容量上限与同一淘汰队列。
//
// key 为空时为安全空操作（直接返回，不写入、不 panic）。
//
// 并发与一致性：写入会持有写锁，并参与 Clear() 的 generation 失效保护——
// 即在写入过程中若发生 Clear()，此次写入会被丢弃，避免写回陈旧数据。
func (c *Cache) SetWithTTL(key string, value any, ttl time.Duration) {
	if key == "" {
		return
	}
	fullKey := joinPrefix(c.opts.Prefix, key)
	gen := c.getGeneration()
	c.setRawItemWithGen(fullKey, value, jitterTTL(ttl, c.opts.Jitter), gen)
}

// getRawItem 读取裸条目，逻辑与 getItem 对齐（过期惰性删除 + LRU 访问时间刷新），
// 但只对 isRaw=true 的条目生效，并返回原始 any 值而非 packed 字节。
func (c *Cache) getRawItem(fullKey string) (any, bool) {
	now := c.opts.Now()

	// 读锁读取；accessedAt 通过原子操作更新，无需升级写锁
	c.mu.RLock()
	item, ok := c.items[fullKey]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}

	// 非裸条目（loader 路径写入）不通过裸 Get 返回
	if !item.isRaw {
		c.mu.RUnlock()
		return nil, false
	}

	// 过期检查：需要写锁删除，升级锁
	if !item.expireAt.IsZero() && now.After(item.expireAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		// 双重检查：获取写锁期间可能已被其他 goroutine 删除/替换
		if existing, exists := c.items[fullKey]; exists &&
			!existing.expireAt.IsZero() && now.After(existing.expireAt) {
			delete(c.items, fullKey)
		}
		c.mu.Unlock()
		return nil, false
	}

	// LRU：原子刷新访问时间与序列号（无需写锁）
	c.touchLRU(item, now)

	val := item.raw
	c.mu.RUnlock()
	return val, true
}

// setRawItemWithGen 写入裸条目，复用 setItemWithGen 的版本号保护与 LRU 淘汰逻辑，
// 但 ttl<=0 时语义为"无过期写入"（expireAt 置零），而非丢弃写入。
func (c *Cache) setRawItemWithGen(fullKey string, value any, ttl time.Duration, expectedGen uint64) {
	now := c.opts.Now()

	// ttl<=0 表示永不过期：expireAt 置零值，过期检查会跳过它
	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 版本号检查：若 Clear() 在本次写入前后被调用，放弃写入，避免写回陈旧数据
	if c.generation.Load() != expectedGen {
		return
	}

	item := &localItem{
		expireAt: exp,
		raw:      value,
		isRaw:    true,
	}
	item.setAccessedAt(now)
	// 写入即视为一次访问：赋予最新序列号，作为同时间戳 LRU 淘汰的确定性 tiebreak。
	item.setSeq(c.nextSeq())
	c.items[fullKey] = item

	// 复用既有 LRU 淘汰逻辑（裸条目与 loader 条目共用容量上限）
	c.evictIfNeededLocked(now)
}
