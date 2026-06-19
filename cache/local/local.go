package local

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

var (
	// 负缓存命中（表示"确实不存在"），用于防穿透。
	ErrNotFound = errors.New("cache: not found")

	// 调用方传入的 dest 不合法（必须是非 nil 指针）
	ErrInvalidDest = errors.New("cache: dest must be a non-nil pointer")

	// key 不能为空
	ErrInvalidKey = errors.New("cache: key is empty")

	// loader 不能为空
	ErrInvalidLoader = errors.New("cache: loader is nil")

	// 缓存内容损坏（例如 value 被其他系统写坏）
	ErrCorrupt = errors.New("cache: corrupt payload")
)

// Codec 用于序列化 / 反序列化缓存数据（默认 JSON）
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

type JSONCodec struct{}

func (JSONCodec) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (JSONCodec) Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

const (
	// DefaultMaxTTL 默认最大 TTL
	DefaultMaxTTL = 15 * time.Minute
)

// Options 控制缓存行为
type Options struct {
	// Prefix 会加到所有 key 前面：prefix:key
	Prefix string

	// Codec 序列化方式（默认 JSON）
	Codec Codec

	// Jitter 用于 TTL 抖动比例（0~1），例如 0.1 表示在 ttl 上最多 +10% 随机抖动
	Jitter float64

	// NegativeTTL 负缓存 TTL（用于防穿透：NotFound 也缓存一段时间）
	NegativeTTL time.Duration

	// IsNotFound 用于识别 loader 返回的"未找到"错误，决定是否写负缓存
	IsNotFound func(err error) bool

	// OnError 缓存层内部错误回调（比如 payload 损坏），用于打点/日志
	OnError func(ctx context.Context, op string, key string, err error)

	// Now 便于测试（默认 time.Now）
	Now func() time.Time
}

type Option func(*Options)

func defaultOptions() Options {
	return Options{
		Prefix:      "",
		Codec:       JSONCodec{},
		Jitter:      0.10,
		NegativeTTL: 30 * time.Second,
		IsNotFound: func(err error) bool {
			return errors.Is(err, ErrNotFound)
		},
		OnError: nil,
		Now:     time.Now,
	}
}

func applyOptions(opts ...Option) Options {
	o := defaultOptions()
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	if o.Codec == nil {
		o.Codec = JSONCodec{}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	// Jitter clamp
	if o.Jitter < 0 {
		o.Jitter = 0
	}
	if o.Jitter > 1 {
		o.Jitter = 1
	}
	if o.IsNotFound == nil {
		o.IsNotFound = func(err error) bool { return errors.Is(err, ErrNotFound) }
	}
	return o
}

func WithPrefix(prefix string) Option {
	return func(o *Options) { o.Prefix = prefix }
}

func WithCodec(codec Codec) Option {
	return func(o *Options) { o.Codec = codec }
}

func WithJitter(j float64) Option {
	return func(o *Options) { o.Jitter = j }
}

func WithNegativeTTL(ttl time.Duration) Option {
	return func(o *Options) { o.NegativeTTL = ttl }
}

func WithIsNotFound(fn func(err error) bool) Option {
	return func(o *Options) { o.IsNotFound = fn }
}

func WithOnError(fn func(ctx context.Context, op string, key string, err error)) Option {
	return func(o *Options) { o.OnError = fn }
}

func WithNow(now func() time.Time) Option {
	return func(o *Options) { o.Now = now }
}

func joinPrefix(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + ":" + key
}

func jitterTTL(ttl time.Duration, jitter float64) time.Duration {
	if ttl <= 0 || jitter <= 0 {
		return ttl
	}

	maxDelta := time.Duration(float64(ttl) * jitter)
	if maxDelta <= 0 {
		return ttl
	}

	// 使用 math/rand/v2 全局函数生成随机数（线程安全）
	// 注意：rand.New() 创建的实例不是线程安全的，必须使用包级函数
	delta := time.Duration(rand.Int64N(int64(maxDelta) + 1))
	return ttl + delta
}

func packFound(data []byte) []byte {
	out := make([]byte, 1+len(data))
	out[0] = 1
	copy(out[1:], data)
	return out
}

func packNotFound() []byte { return []byte{0} }

func unpack(packed []byte) (found bool, data []byte, err error) {
	if len(packed) == 0 {
		return false, nil, ErrCorrupt
	}
	if packed[0] == 0 {
		return false, nil, nil
	}
	return true, packed[1:], nil
}

func ensureDestPtr(dest any) error {
	if dest == nil {
		return ErrInvalidDest
	}
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return ErrInvalidDest
	}
	return nil
}

type localItem struct {
	packed     []byte
	expireAt   time.Time
	accessedAt atomic.Int64 // LRU: 最后访问时间（UnixNano），使用原子操作支持读锁下更新

	// seq 是单调递增的访问序列号，作为 accessedAt 相同时的 LRU 淘汰 tiebreak。
	//
	// 动机：accessedAt 取自 wall-clock（UnixNano），在高频写入/访问或低分辨率时钟
	// （部分平台 time.Now() 精度可达毫秒级）下，多个条目可能落在同一纳秒。此时仅按
	// accessedAt 排序无法区分先后，叠加 Go map 遍历顺序非确定性，淘汰对象会随机变化，
	// 导致下游确定性 LRU 测试 flaky。
	//
	// 每次写入与每次 LRU 访问刷新时，都会从 Cache.accessSeq 原子自增取一个全局唯一、
	// 单调递增的序列号写入本字段。于是淘汰时按 (accessedAt, seq) 字典序取最小者，
	// 即可在任意时钟分辨率下得到确定的淘汰对象，且与 accessedAt 的 LRU 语义一致
	// （seq 仅在 accessedAt 相同时生效，越小表示越早被访问，应优先淘汰）。
	//
	// 使用原子操作，支持读锁下更新（与 accessedAt 同步刷新）。
	seq atomic.Uint64

	// raw 用于裸 Get/Set API：直接存放调用方传入的原始 any 值，
	// 不经 Codec 序列化。仅当 isRaw 为 true 时有效。
	//
	// 与 packed 互斥：
	//   - loader 路径（GetOrLoad/GetOrLoadEx）写入 packed，isRaw=false
	//   - 裸 Set 路径写入 raw，isRaw=true，packed 为 nil
	//
	// 这样裸 API 与 loader API 共享同一份底层存储、过期、LRU 淘汰机制，
	// 但各自的读路径互不干扰。
	raw   any
	isRaw bool
}

// newLocalItem 创建新的 localItem
func newLocalItem(packed []byte, expireAt time.Time, accessedAt time.Time) *localItem {
	item := &localItem{
		packed:   packed,
		expireAt: expireAt,
	}
	item.accessedAt.Store(accessedAt.UnixNano())
	return item
}

// getAccessedAt 获取访问时间
func (i *localItem) getAccessedAt() time.Time {
	return time.Unix(0, i.accessedAt.Load())
}

// setAccessedAt 设置访问时间（原子操作）
func (i *localItem) setAccessedAt(t time.Time) {
	i.accessedAt.Store(t.UnixNano())
}

// getSeq 获取访问序列号（LRU 淘汰 tiebreak）
func (i *localItem) getSeq() uint64 {
	return i.seq.Load()
}

// setSeq 设置访问序列号（原子操作）
func (i *localItem) setSeq(s uint64) {
	i.seq.Store(s)
}

type Cache struct {
	mu         sync.RWMutex
	items      map[string]*localItem // 使用指针以支持读锁下原子更新 accessedAt
	sf         singleflight.Group
	opts       Options
	maxEntries int

	// 定期清理
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
	stopped         atomic.Bool // 防止双重关闭

	// 版本号：Clear() 时递增，用于防止 singleflight 竞态写入旧数据
	generation atomic.Uint64

	// accessSeq 是本缓存实例内单调递增的访问序列号发号器。
	// 每次写入或 LRU 访问刷新都会 Add(1) 取号并写入对应 localItem.seq，
	// 作为 accessedAt 相同时的确定性淘汰 tiebreak（详见 localItem.seq）。
	accessSeq atomic.Uint64
}

// nextSeq 取下一个单调递增的访问序列号（并发安全）。
func (c *Cache) nextSeq() uint64 {
	return c.accessSeq.Add(1)
}

// touchLRU 刷新条目的 LRU 访问信息：同步更新 accessedAt 与 seq。
//
// 两者必须成对更新——accessedAt 决定 LRU 主序，seq 在同一纳秒内决定先后，
// 共同保证淘汰对象在任意时钟分辨率下都是确定的。该方法使用原子操作，
// 可在读锁下安全调用（无需升级写锁）。
func (c *Cache) touchLRU(item *localItem, now time.Time) {
	item.setAccessedAt(now)
	item.setSeq(c.nextSeq())
}

const (
	// DefaultCleanupInterval 默认清理间隔
	DefaultCleanupInterval = time.Minute

	// DefaultMaxEntries 当 maxEntries <= 0 时的默认上限，防止 OOM
	DefaultMaxEntries = 10000
)

// NewCache 创建本地缓存
// cleanupInterval 为 0 时使用默认值（1分钟），传入负值则禁用定期清理
func NewCache(maxEntries int, opts ...Option) *Cache {
	return NewCacheWithCleanup(maxEntries, DefaultCleanupInterval, opts...)
}

// NewCacheWithCleanup 创建本地缓存（可指定清理间隔）
//
// 注意：maxEntries <= 0 时会使用默认上限（DefaultMaxEntries = 10000），防止 OOM。
// 如需更大容量，请显式传入正整数。
func NewCacheWithCleanup(maxEntries int, cleanupInterval time.Duration, opts ...Option) *Cache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	c := &Cache{
		items:           make(map[string]*localItem),
		opts:            applyOptions(opts...),
		maxEntries:      maxEntries,
		cleanupInterval: cleanupInterval,
		stopCleanup:     make(chan struct{}),
	}

	// 启动定期清理（cleanupInterval <= 0 时禁用）
	if cleanupInterval > 0 {
		go c.periodicCleanup()
	}

	return c
}

// NewCacheNoCleanup 创建"无后台清理"的本地缓存：不启动任何后台 goroutine，
// 仅依赖惰性过期（读取到过期条目时就地删除）+ 写入时的 LRU 容量淘汰。
//
// 适用场景：调用方将本缓存委托/嵌入到更大的生命周期中（如下游 ToolCache），
// 不希望被一个无法显式停止的后台 goroutine 拖累，从而避免 goroutine 泄漏。
// 这等价于 NewCacheWithCleanup(maxEntries, 0, opts...)，但语义更显式、更易自文档化。
//
// 与定期清理构造的区别：
//   - 过期条目只在被再次读取、或因容量超限触发 LRU 淘汰时才被回收；
//     在被读取前会一直占用 map 槽位（但已过期条目读取必然返回未命中）。
//   - 没有后台 goroutine，因此即使不调用 Stop/Close 也不会泄漏 goroutine。
//
// 仍可安全调用 Stop()/Close()（幂等空操作），便于与统一的关闭流程兼容。
//
// 注意：maxEntries <= 0 时同样会规整为 DefaultMaxEntries（10000），防止 OOM。
func NewCacheNoCleanup(maxEntries int, opts ...Option) *Cache {
	return NewCacheWithCleanup(maxEntries, 0, opts...)
}

func (c *Cache) GetOrLoad(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {
	if key == "" {
		return ErrInvalidKey
	}
	if loader == nil {
		return ErrInvalidLoader
	}
	if err := ensureDestPtr(dest); err != nil {
		return err
	}

	fullKey := joinPrefix(c.opts.Prefix, key)

	// 1) 先读本地缓存
	if packed, ok, err := c.getItem(fullKey); err == nil && ok {
		return c.unmarshalPacked(packed, dest)
	} else if err != nil {
		c.onError(ctx, "local_get", fullKey, err)
	}

	// 记录当前版本号，用于防止 Clear() 竞态
	gen := c.getGeneration()

	// 2) singleflight 防击穿
	v, err, _ := c.sf.Do(fullKey, func() (any, error) {
		// double check
		if packed2, ok2, _ := c.getItem(fullKey); ok2 {
			return packed2, nil
		}

		val, lerr := loader(ctx)
		if lerr != nil {
			if c.isNotFound(lerr) {
				negTTL := c.negativeTTL()
				c.setItemWithGen(fullKey, packNotFound(), jitterTTL(negTTL, c.opts.Jitter), gen, true)
			}
			return nil, lerr
		}

		raw, merr := c.opts.Codec.Marshal(val)
		if merr != nil {
			return nil, merr
		}
		packed3 := packFound(raw)

		if ttl > 0 {
			c.setItemWithGen(fullKey, packed3, jitterTTL(ttl, c.opts.Jitter), gen, true)
		}
		return packed3, nil
	})
	if err != nil {
		return err
	}

	packed, ok := v.([]byte)
	if !ok {
		return ErrCorrupt
	}
	return c.unmarshalPacked(packed, dest)
}

func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, k := range keys {
		if k == "" {
			continue
		}
		fullKey := joinPrefix(c.opts.Prefix, k)
		delete(c.items, fullKey)
	}
	return nil
}

// --- internal ---

func (c *Cache) getItem(fullKey string) ([]byte, bool, error) {
	now := c.opts.Now()

	// 使用读锁进行读取操作
	// accessedAt 使用原子操作更新，无需写锁
	c.mu.RLock()
	item, ok := c.items[fullKey]
	if !ok {
		c.mu.RUnlock()
		return nil, false, nil
	}

	// 检查过期（需要写锁删除，升级锁）
	if !item.expireAt.IsZero() && now.After(item.expireAt) {
		c.mu.RUnlock()
		// 升级到写锁进行删除
		c.mu.Lock()
		// 双重检查：在获取写锁期间可能已被其他 goroutine 删除
		if existingItem, exists := c.items[fullKey]; exists && now.After(existingItem.expireAt) {
			delete(c.items, fullKey)
		}
		c.mu.Unlock()
		return nil, false, nil
	}

	if len(item.packed) == 0 {
		c.mu.RUnlock()
		return nil, false, ErrCorrupt
	}

	// LRU: 原子更新访问时间与序列号（无需写锁）
	c.touchLRU(item, now)

	// 返回副本，避免外部修改
	cp := make([]byte, len(item.packed))
	copy(cp, item.packed)
	c.mu.RUnlock()
	return cp, true, nil
}

func (c *Cache) setItem(fullKey string, packed []byte, ttl time.Duration) {
	c.setItemWithGen(fullKey, packed, ttl, 0, false)
}

// setItemWithGen 带版本号检查的写入方法
// checkGen=true 时，只有 generation 匹配才写入（用于防止 Clear() 竞态）
func (c *Cache) setItemWithGen(fullKey string, packed []byte, ttl time.Duration, expectedGen uint64, checkGen bool) {
	if ttl <= 0 {
		return
	}
	now := c.opts.Now()
	exp := now.Add(ttl)

	// copy
	cp := make([]byte, len(packed))
	copy(cp, packed)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 版本号检查：如果 Clear() 在 singleflight 期间被调用，放弃写入
	if checkGen && c.generation.Load() != expectedGen {
		return
	}

	item := newLocalItem(cp, exp, now)
	// 写入即视为一次访问：赋予最新序列号，保证它在同一纳秒内是"最近使用"的，
	// 不会被同时间戳的旧条目反超而误淘汰。
	item.setSeq(c.nextSeq())
	c.items[fullKey] = item
	c.evictIfNeededLocked(now)
}

// getGeneration 获取当前版本号（用于 singleflight 竞态保护）
func (c *Cache) getGeneration() uint64 {
	return c.generation.Load()
}

func (c *Cache) evictIfNeededLocked(now time.Time) {
	if c.maxEntries <= 0 {
		return
	}
	if len(c.items) <= c.maxEntries {
		return
	}

	// 1) 先收集过期的 key，再删除（避免遍历时删除）
	var expiredKeys []string
	for k, it := range c.items {
		if !it.expireAt.IsZero() && now.After(it.expireAt) {
			expiredKeys = append(expiredKeys, k)
		}
	}
	for _, k := range expiredKeys {
		delete(c.items, k)
	}
	if len(c.items) <= c.maxEntries {
		return
	}

	// 2) LRU 驱逐：删除最久未访问的条目（确定性淘汰）
	// 性能特征：使用选择排序找最小的 needDel 个元素，时间复杂度 O(n*needDel)。
	// 当 maxEntries 较大（>10万）且频繁触发驱逐时性能可能下降，
	// 可考虑引入 container/heap 或双向链表优化为 O(n*log(n))。
	// 对于常见的万级缓存场景，当前实现足够高效。
	//
	// 确定性保证：排序键为 (accessedAt, seq) 二元组。accessedAt 为 LRU 主序，
	// seq 为同一纳秒内的单调 tiebreak。任意两个条目都有严格全序，因此即便
	// Go map 遍历顺序不确定，选择排序选出的淘汰对象也完全确定。
	needDel := len(c.items) - c.maxEntries
	if needDel <= 0 {
		return
	}

	// 收集所有条目的访问时间与序列号
	type keyMeta struct {
		key        string
		accessedAt int64  // UnixNano
		seq        uint64 // 同时间戳 tiebreak
	}
	candidates := make([]keyMeta, 0, len(c.items))
	for k, it := range c.items {
		candidates = append(candidates, keyMeta{
			key:        k,
			accessedAt: it.accessedAt.Load(),
			seq:        it.getSeq(),
		})
	}

	// 部分排序：只需要找到最小的 needDel 个元素
	// 使用简单的选择算法（对于小数量的删除更高效）
	for i := 0; i < needDel && i < len(candidates); i++ {
		minIdx := i
		for j := i + 1; j < len(candidates); j++ {
			if lruLess(candidates[j].accessedAt, candidates[j].seq,
				candidates[minIdx].accessedAt, candidates[minIdx].seq) {
				minIdx = j
			}
		}
		if minIdx != i {
			candidates[i], candidates[minIdx] = candidates[minIdx], candidates[i]
		}
		// 删除第 i 个最旧的条目
		delete(c.items, candidates[i].key)
	}
}

// lruLess 定义 LRU 淘汰的严格全序：按 (accessedAt, seq) 字典序比较。
//
// 返回 true 表示左侧条目"更应被淘汰"（更久未访问）。
//   - accessedAt 较小者优先淘汰（最久未访问）。
//   - accessedAt 相同时，seq 较小者优先淘汰（同一纳秒内更早被访问）。
//
// 由于 seq 在本缓存实例内单调唯一，(accessedAt, seq) 构成严格全序，
// 保证淘汰对象在任意时钟分辨率下都是确定的，不受 map 遍历顺序影响。
func lruLess(aAccessed int64, aSeq uint64, bAccessed int64, bSeq uint64) bool {
	if aAccessed != bAccessed {
		return aAccessed < bAccessed
	}
	return aSeq < bSeq
}

func (c *Cache) unmarshalPacked(packed []byte, dest any) error {
	found, data, err := unpack(packed)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return c.opts.Codec.Unmarshal(data, dest)
}

func (c *Cache) negativeTTL() time.Duration {
	if c.opts.NegativeTTL > 0 {
		return c.opts.NegativeTTL
	}
	return 30 * time.Second
}

func (c *Cache) isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if c.opts.IsNotFound != nil && c.opts.IsNotFound(err) {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

func (c *Cache) onError(ctx context.Context, op, key string, err error) {
	if c.opts.OnError != nil {
		c.opts.OnError(ctx, op, key, err)
	}
}

// periodicCleanup 定期清理过期条目
func (c *Cache) periodicCleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanExpired()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanExpired 清理所有过期条目
func (c *Cache) cleanExpired() {
	now := c.opts.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, item := range c.items {
		if !item.expireAt.IsZero() && now.After(item.expireAt) {
			delete(c.items, k)
		}
	}
}

// Stop 停止定期清理（优雅关闭时调用）。
//
// 幂等：多次调用安全，只会真正关闭一次。对"无后台清理"构造
// （NewCacheNoCleanup / cleanupInterval<=0）的缓存调用也安全——
// 此时没有后台 goroutine，调用仅是把停止信号 channel 关闭，不产生副作用。
func (c *Cache) Stop() {
	// 使用 atomic.Bool 确保只关闭一次
	if c.stopped.CompareAndSwap(false, true) {
		close(c.stopCleanup)
	}
}

// Close 是 Stop 的同名别名，停止后台清理并释放相关资源。
//
// 提供该方法是为契合 Go 生态中"资源持有者实现 Close() error"的惯例
// （形似 io.Closer），便于下游用统一的关闭流程（如 defer c.Close()）管理缓存生命周期。
// 始终返回 nil；与 Stop 一样幂等，多次调用安全。
func (c *Cache) Close() error {
	c.Stop()
	return nil
}

// Len 返回当前缓存条目数（用于监控）
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Clear 清空所有缓存条目（不停止后台清理 goroutine）
// 同时递增版本号，使正在进行的 singleflight 请求不会写入旧数据
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*localItem)
	c.generation.Add(1) // 递增版本号，使进行中的 singleflight 写入失效
}

// loadResult 用于 singleflight 返回值，携带缓存命中信息
type loadResult struct {
	packed    []byte
	fromCache bool // true 表示来自缓存（double check 命中），false 表示来自 loader
}

// GetOrLoadEx 与 GetOrLoad 相同，但额外返回是否命中本地缓存
// cacheHit=true 表示数据来自本地缓存，cacheHit=false 表示数据来自 loader
// 注意：singleflight 场景下，所有等待者都会获得相同的 cacheHit 值
func (c *Cache) GetOrLoadEx(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) (cacheHit bool, err error) {
	if key == "" {
		return false, ErrInvalidKey
	}
	if loader == nil {
		return false, ErrInvalidLoader
	}
	if err := ensureDestPtr(dest); err != nil {
		return false, err
	}

	fullKey := joinPrefix(c.opts.Prefix, key)

	// 1) 先读本地缓存
	if packed, ok, err := c.getItem(fullKey); err == nil && ok {
		return true, c.unmarshalPacked(packed, dest)
	} else if err != nil {
		c.onError(ctx, "local_get", fullKey, err)
	}

	// 记录当前版本号，用于防止 Clear() 竞态
	gen := c.getGeneration()

	// 2) singleflight 防击穿，返回值携带来源信息
	v, err, _ := c.sf.Do(fullKey, func() (any, error) {
		// double check
		if packed2, ok2, _ := c.getItem(fullKey); ok2 {
			return loadResult{packed: packed2, fromCache: true}, nil
		}

		val, lerr := loader(ctx)
		if lerr != nil {
			if c.isNotFound(lerr) {
				negTTL := c.negativeTTL()
				c.setItemWithGen(fullKey, packNotFound(), jitterTTL(negTTL, c.opts.Jitter), gen, true)
			}
			return nil, lerr
		}

		raw, merr := c.opts.Codec.Marshal(val)
		if merr != nil {
			return nil, merr
		}
		packed3 := packFound(raw)

		if ttl > 0 {
			c.setItemWithGen(fullKey, packed3, jitterTTL(ttl, c.opts.Jitter), gen, true)
		}
		return loadResult{packed: packed3, fromCache: false}, nil
	})
	if err != nil {
		return false, err
	}

	// 解析 singleflight 返回值
	result, ok := v.(loadResult)
	if !ok {
		return false, ErrCorrupt
	}
	return result.fromCache, c.unmarshalPacked(result.packed, dest)
}
