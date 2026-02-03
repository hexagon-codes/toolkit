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

type JSONCodec struct{}

func (JSONCodec) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (JSONCodec) Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

const (
	// DefaultMaxTTL 默认最大 TTL（用于 UnstableCache）
	DefaultMaxTTL = 15 * time.Minute
	// StableCacheTTL 稳定 Key TTL（单条记录查询）
	StableCacheTTL = 60 * time.Minute

	// 不稳定 Key TTL（聚合/JOIN/列表查询）
	UnstableCacheTTLShort  = 5 * time.Minute  // JOIN 查询
	UnstableCacheTTLMedium = 10 * time.Minute // 聚合查询
	UnstableCacheTTLLong   = 15 * time.Minute // 不常变的聚合
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

// ApplyOptions 导出的配置应用函数（供外部使用）
func ApplyOptions(opts ...Option) Options {
	return applyOptions(opts...)
}

// -------- Option helpers --------

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

// WithMaxTTL 设置最大 TTL 上限（主要用于 UnstableCache）
// 传入 0 表示不限制
func WithMaxTTL(ttl time.Duration) Option {
	return func(o *Options) { o.MaxTTL = ttl }
}

func WithRedisTimeout(readTimeout, writeTimeout time.Duration) Option {
	return func(o *Options) {
		o.ReadTimeout = readTimeout
		o.WriteTimeout = writeTimeout
	}
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

// WithTimeout：如果 parent 的 deadline 已经更紧，就不再额外包一层更短超时（导出供外部使用）
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
	// 使用 math/rand/v2，在 Go 1.22+ 是线程安全的
	delta := time.Duration(rand.Int64N(int64(maxDelta) + 1))
	return ttl + delta
}

// JitterTTL 导出的 TTL 抖动函数（防止缓存雪崩）
func JitterTTL(ttl time.Duration, jitter float64) time.Duration {
	return jitterTTL(ttl, jitter)
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
	if packed[0] == 0 {
		return false, nil, nil
	}
	return true, packed[1:], nil
}
