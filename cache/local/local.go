package local

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
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
	// 使用 crypto/rand 生成安全随机数
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return ttl // 失败时不抖动
	}
	n := int64(binary.LittleEndian.Uint64(buf[:]))
	if n < 0 {
		n = -n
	}
	delta := time.Duration(n % (int64(maxDelta) + 1))
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
	accessedAt time.Time // LRU: 最后访问时间
}

type Cache struct {
	mu         sync.RWMutex
	items      map[string]localItem
	sf         singleflight.Group
	opts       Options
	maxEntries int

	// 定期清理
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
	stopped         atomic.Bool // 防止双重关闭
}

// DefaultCleanupInterval 默认清理间隔
const DefaultCleanupInterval = time.Minute

// NewCache 创建本地缓存
// cleanupInterval 为 0 时使用默认值（1分钟），传入负值则禁用定期清理
func NewCache(maxEntries int, opts ...Option) *Cache {
	return NewCacheWithCleanup(maxEntries, DefaultCleanupInterval, opts...)
}

// NewCacheWithCleanup 创建本地缓存（可指定清理间隔）
func NewCacheWithCleanup(maxEntries int, cleanupInterval time.Duration, opts ...Option) *Cache {
	c := &Cache{
		items:           make(map[string]localItem),
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
				c.setItem(fullKey, packNotFound(), jitterTTL(negTTL, c.opts.Jitter))
			}
			return nil, lerr
		}

		raw, merr := c.opts.Codec.Marshal(val)
		if merr != nil {
			return nil, merr
		}
		packed3 := packFound(raw)

		if ttl > 0 {
			c.setItem(fullKey, packed3, jitterTTL(ttl, c.opts.Jitter))
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

	c.mu.RLock()
	item, ok := c.items[fullKey]
	c.mu.RUnlock()

	if !ok {
		return nil, false, nil
	}
	if !item.expireAt.IsZero() && now.After(item.expireAt) {
		// 过期则清理
		c.mu.Lock()
		delete(c.items, fullKey)
		c.mu.Unlock()
		return nil, false, nil
	}
	if len(item.packed) == 0 {
		return nil, false, ErrCorrupt
	}

	// LRU: 更新访问时间
	c.mu.Lock()
	if it, exists := c.items[fullKey]; exists {
		it.accessedAt = now
		c.items[fullKey] = it
	}
	c.mu.Unlock()

	// 返回副本，避免外部修改
	cp := make([]byte, len(item.packed))
	copy(cp, item.packed)
	return cp, true, nil
}

func (c *Cache) setItem(fullKey string, packed []byte, ttl time.Duration) {
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

	c.items[fullKey] = localItem{
		packed:     cp,
		expireAt:   exp,
		accessedAt: now, // LRU: 初始化访问时间
	}
	c.evictIfNeededLocked(now)
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

	// 2) LRU 驱逐：删除最久未访问的条目
	// 优化：一次性收集所有需要删除的 key，避免多次遍历
	needDel := len(c.items) - c.maxEntries
	if needDel <= 0 {
		return
	}

	// 收集所有条目的访问时间
	type keyTime struct {
		key  string
		time time.Time
	}
	candidates := make([]keyTime, 0, len(c.items))
	for k, it := range c.items {
		candidates = append(candidates, keyTime{k, it.accessedAt})
	}

	// 部分排序：只需要找到最小的 needDel 个元素
	// 使用简单的选择算法（对于小数量的删除更高效）
	for i := 0; i < needDel && i < len(candidates); i++ {
		minIdx := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].time.Before(candidates[minIdx].time) {
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

// Stop 停止定期清理（优雅关闭时调用）
func (c *Cache) Stop() {
	// 使用 atomic.Bool 确保只关闭一次
	if c.stopped.CompareAndSwap(false, true) {
		close(c.stopCleanup)
	}
}

// Len 返回当前缓存条目数（用于监控）
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}
