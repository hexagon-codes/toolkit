package redis

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"reflect"
	"time"
)

var (
	// ErrNotFound 表示负缓存命中（数据确实不存在），用于防穿透。
	ErrNotFound = errors.New("cache: not found")

	// ErrInvalidDest 表示调用方传入的 dest 不是非 nil 指针。
	ErrInvalidDest = errors.New("cache: dest must be a non-nil pointer")

	// ErrInvalidKey 表示缓存 key 为空。
	ErrInvalidKey = errors.New("cache: key is empty")

	// ErrInvalidLoader 表示 loader 为空。
	ErrInvalidLoader = errors.New("cache: loader is nil")

	// ErrInvalidContext 表示调用方传入了 nil context。
	ErrInvalidContext = errors.New("cache: context must not be nil")

	// ErrInvalidClient 表示 Redis 客户端为 nil 或 typed nil。
	ErrInvalidClient = errors.New("cache: Redis client must not be nil")

	// ErrInvalidValue 表示 loader 在成功路径返回了 nil 或 typed nil。
	ErrInvalidValue = errors.New("cache: loader returned a nil value")

	// ErrCorrupt 表示缓存内容损坏，例如 value 被其他系统写坏。
	ErrCorrupt = errors.New("cache: corrupt payload")
)

// Cache 业务代码依赖的抽象（Service/Repo 只依赖它，不依赖 Redis 实现）
type Cache interface {
	GetOrLoad(
		ctx context.Context,
		key string,
		ttl time.Duration,
		dest any,
		loader func(ctx context.Context) (any, error),
	) error

	Del(ctx context.Context, keys ...string) error
}

// Codec 用于序列化 / 反序列化缓存数据（默认 JSON）
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSONCodec 使用 encoding/json 编解码缓存值。
type JSONCodec struct{}

// Marshal 将值编码为 JSON。
func (JSONCodec) Marshal(v any) ([]byte, error) { return json.Marshal(v) }

// Unmarshal 将 JSON 解码到目标值。
func (JSONCodec) Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

const (
	// defaultLoaderTimeout 限制共享 loader 的最长执行时间。
	defaultLoaderTimeout = 10 * time.Second

	// DefaultMaxTTL 默认最大 TTL（用于 UnstableCache）
	DefaultMaxTTL = 15 * time.Minute
	// StableCacheTTL 稳定 Key TTL（单条记录查询）
	StableCacheTTL = 60 * time.Minute

	// UnstableCacheTTLShort 是 JOIN 查询等短周期不稳定 key 的 TTL。
	UnstableCacheTTLShort = 5 * time.Minute
	// UnstableCacheTTLMedium 是聚合查询等中周期不稳定 key 的 TTL。
	UnstableCacheTTLMedium = 10 * time.Minute
	// UnstableCacheTTLLong 是低频变化聚合查询的 TTL。
	UnstableCacheTTLLong = 15 * time.Minute
)

// Options 控制缓存行为（Redis/Local 共用）
type Options struct {
	// Prefix 会加到所有 key 前面：prefix:key
	Prefix string

	// Codec 序列化方式（默认 JSON）
	Codec Codec

	// Jitter 用于 TTL 抖动比例（0~1），例如 0.1 表示在 ttl 上最多 +10% 随机抖动
	Jitter float64

	// NegativeTTL 负缓存 TTL（用于防穿透：NotFound 也缓存一段时间）
	NegativeTTL time.Duration

	// MaxTTL 最大 TTL 上限（主要用于 UnstableCache 限制聚合数据缓存时间）
	// 0 表示不限制，默认 15 分钟
	MaxTTL time.Duration

	// ReadTimeout/WriteTimeout 对 Redis 操作的超时（LocalCache 不使用）
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// IsNotFound 用于识别 loader 返回的"未找到"错误，决定是否写负缓存
	// 默认：errors.Is(err, cache.ErrNotFound)
	//
	// 建议业务里把 gorm.ErrRecordNotFound 映射进来，例如：
	//   cache.WithIsNotFound(func(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, cache.ErrNotFound) })
	IsNotFound func(err error) bool

	// OnError 缓存层内部错误回调（比如 Redis get/set 出错、payload 损坏），用于打点/日志
	OnError func(ctx context.Context, op string, key string, err error)

	// Now 便于测试（默认 time.Now）
	Now func() time.Time
}

// Option 配置 Redis 缓存行为。
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		Prefix:       "",
		Codec:        JSONCodec{},
		Jitter:       0.10,
		NegativeTTL:  30 * time.Second,
		MaxTTL:       DefaultMaxTTL,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond, // SCAN 操作需要更长超时
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
	if isNilInterface(o.Codec) {
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

// ApplyOptions 导出的配置应用函数（供外部使用）
func ApplyOptions(opts ...Option) Options {
	return applyOptions(opts...)
}

// -------- Option helpers --------

// WithPrefix 设置所有缓存 key 的前缀。
func WithPrefix(prefix string) Option {
	return func(o *Options) { o.Prefix = prefix }
}

// WithCodec 设置缓存值编解码器。
func WithCodec(codec Codec) Option {
	return func(o *Options) { o.Codec = codec }
}

// WithJitter 设置 TTL 的随机抖动比例。
func WithJitter(j float64) Option {
	return func(o *Options) { o.Jitter = j }
}

// WithNegativeTTL 设置负缓存的 TTL。
func WithNegativeTTL(ttl time.Duration) Option {
	return func(o *Options) { o.NegativeTTL = ttl }
}

// WithMaxTTL 设置最大 TTL 上限（主要用于 UnstableCache）
// 传入 0 表示不限制
func WithMaxTTL(ttl time.Duration) Option {
	return func(o *Options) { o.MaxTTL = ttl }
}

// WithRedisTimeout 设置 Redis 读写操作的超时。
func WithRedisTimeout(readTimeout, writeTimeout time.Duration) Option {
	return func(o *Options) {
		o.ReadTimeout = readTimeout
		o.WriteTimeout = writeTimeout
	}
}

// WithIsNotFound 设置识别未找到错误的函数。
func WithIsNotFound(fn func(err error) bool) Option {
	return func(o *Options) { o.IsNotFound = fn }
}

// WithOnError 设置缓存内部错误回调。
func WithOnError(fn func(ctx context.Context, op string, key string, err error)) Option {
	return func(o *Options) { o.OnError = fn }
}

// WithNow 设置缓存使用的时钟函数。
func WithNow(now func() time.Time) Option {
	return func(o *Options) { o.Now = now }
}

func ensureDestPtr(dest any) error {
	if dest == nil {
		return ErrInvalidDest
	}
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return ErrInvalidDest
	}
	return nil
}

func newDestinationLike(destination any) any {
	value := reflect.ValueOf(destination)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return nil
	}
	return reflect.New(value.Elem().Type()).Interface()
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// WithTimeout 创建超时 context；若 parent 的 deadline 更早，则直接复用 parent。
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return parent, func() {}
	}
	if deadline, ok := parent.Deadline(); ok {
		if time.Until(deadline) <= d {
			return parent, func() {}
		}
	}
	return context.WithTimeout(parent, d)
}

// withTimeout 内部使用的别名
func withTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return WithTimeout(parent, d)
}

func joinPrefix(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + ":" + key
}

// JoinPrefix 导出的 key 前缀拼接函数
func JoinPrefix(prefix, key string) string {
	return joinPrefix(prefix, key)
}

func jitterTTL(ttl time.Duration, jitter float64) time.Duration {
	if ttl <= 0 || jitter <= 0 {
		return ttl
	}

	maxDelta := time.Duration(float64(ttl) * jitter)
	if maxDelta <= 0 {
		return ttl
	}
	// delta in [0, maxDelta]
	// 包级随机函数并发安全；此处只用于缓存过期抖动，不承担安全随机职责。
	delta := time.Duration(rand.Int64N(int64(maxDelta) + 1)) // #nosec G404 -- 缓存过期抖动不用于安全随机数。
	return ttl + delta
}

// JitterTTL 导出的 TTL 抖动函数（防止缓存雪崩）
func JitterTTL(ttl time.Duration, jitter float64) time.Duration {
	return jitterTTL(ttl, jitter)
}

// loadAndFillCommon 降级加载并填充 dest 的公共逻辑（StableCache 和 UnstableCache 共用）
// 修复：检查 Marshal 错误，不再静默忽略
func loadAndFillCommon(ctx context.Context, codec Codec, loader func(ctx context.Context) (any, error), dest any) error {
	val, err := loader(ctx)
	if err != nil {
		return err
	}
	if isNilInterface(val) {
		return ErrInvalidValue
	}
	if dest != nil {
		raw, merr := codec.Marshal(val)
		if merr != nil {
			return merr
		}
		return codec.Unmarshal(raw, dest)
	}
	return nil
}

// isNotFoundCommon 判断错误是否为"未找到"的公共逻辑（StableCache 和 UnstableCache 共用）
func isNotFoundCommon(err error, isNotFound func(error) bool) bool {
	if err == nil {
		return false
	}
	if isNotFound != nil && isNotFound(err) {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

// 简单的二进制 envelope：
// packed[0] == 1 表示 Found=true，后面是 codec 的数据
// packed[0] == 0 表示 Found=false（负缓存）
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
	switch packed[0] {
	case 0:
		if len(packed) != 1 {
			return false, nil, ErrCorrupt
		}
		return false, nil, nil
	case 1:
		return true, packed[1:], nil
	default:
		return false, nil, ErrCorrupt
	}
}
