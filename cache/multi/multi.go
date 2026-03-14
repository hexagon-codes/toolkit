package multi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

var (
	// ErrNotFound 数据不存在（所有层都未命中且 loader 返回 NotFound）
	ErrNotFound = errors.New("multi-cache: not found")

	// ErrInvalidDest dest 参数无效
	ErrInvalidDest = errors.New("multi-cache: dest must be a non-nil pointer")

	// ErrInvalidKey key 为空
	ErrInvalidKey = errors.New("multi-cache: key is empty")

	// ErrInvalidLoader loader 为空
	ErrInvalidLoader = errors.New("multi-cache: loader is nil")

	// ErrNoLayers 没有配置任何缓存层
	ErrNoLayers = errors.New("multi-cache: no cache layers configured")

	// ErrNilLayer 缓存层实例为 nil
	ErrNilLayer = errors.New("multi-cache: layer instance is nil")
)

// Layer 缓存层接口（本地缓存和 Redis 缓存都实现了这个接口）
type Layer interface {
	// GetOrLoad 获取或加载数据
	GetOrLoad(ctx context.Context, key string, ttl time.Duration, dest any, loader func(ctx context.Context) (any, error)) error
	// Del 删除缓存
	Del(ctx context.Context, keys ...string) error
}

// LayerConfig 缓存层配置
type LayerConfig struct {
	Layer Layer         // 缓存层实例
	TTL   time.Duration // 该层的 TTL
	Name  string        // 层名称（用于日志/监控）
}

// Cache 多层缓存
//
// 工作原理：
// 1. GetOrLoad 时从第一层开始查询，命中则返回
// 2. 未命中则查询下一层，找到后回填到前面的层
// 3. 所有层都未命中，调用 loader 从数据源加载
// 4. Del 时删除所有层的缓存
//
// 示例：
//
//	// 创建多层缓存
//	cache := multi.NewCache(
//	    multi.LayerConfig{Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
//	    multi.LayerConfig{Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
//	)
//
//	// 使用（自动处理三层：local -> redis -> db）
//	var user User
//	err := cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
//	    return db.FindUserByID(ctx, 123)
//	})
type Cache struct {
	layers []LayerConfig
	opts   Options
}

// Options 多层缓存配置
type Options struct {
	// IsNotFound 判断 loader 返回的错误是否表示"数据不存在"
	IsNotFound func(err error) bool

	// OnError 错误回调（用于日志/监控）
	OnError func(ctx context.Context, layer string, op string, key string, err error)

	// SkipBackfill 是否跳过回填（默认 false，即会回填）
	// 设置为 true 可以减少写入次数，但会降低缓存命中率
	SkipBackfill bool
}

type Option func(*Options)

func defaultOptions() Options {
	return Options{
		IsNotFound: func(err error) bool {
			return errors.Is(err, ErrNotFound)
		},
		OnError:      nil,
		SkipBackfill: false,
	}
}

func applyOptions(opts ...Option) Options {
	o := defaultOptions()
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	if o.IsNotFound == nil {
		o.IsNotFound = func(err error) bool { return errors.Is(err, ErrNotFound) }
	}
	return o
}

// WithIsNotFound 设置 NotFound 判断函数
func WithIsNotFound(fn func(err error) bool) Option {
	return func(o *Options) { o.IsNotFound = fn }
}

// WithOnError 设置错误回调
func WithOnError(fn func(ctx context.Context, layer string, op string, key string, err error)) Option {
	return func(o *Options) { o.OnError = fn }
}

// WithSkipBackfill 跳过回填（减少写入次数，但降低缓存命中率）
func WithSkipBackfill(skip bool) Option {
	return func(o *Options) { o.SkipBackfill = skip }
}

// NewCache 创建多层缓存
//
// 参数：
//   - layers: 缓存层配置（按优先级从高到低排列，如 local -> redis）
//   - opts: 可选配置
//
// 如果 layers 中包含 nil Layer，会 panic。
//
// 示例：
//
//	cache := multi.NewCache(
//	    multi.LayerConfig{Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
//	    multi.LayerConfig{Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
//	)
func NewCache(layers []LayerConfig, opts ...Option) *Cache {
	// 校验 layers 中是否包含 nil Layer
	for i, l := range layers {
		if l.Layer == nil {
			panic(fmt.Sprintf("multi-cache: layer[%d] (%s) has nil Layer instance", i, l.Name))
		}
	}
	return &Cache{
		layers: layers,
		opts:   applyOptions(opts...),
	}
}

// GetOrLoad 获取或加载数据（自动处理多层缓存）
//
// 工作流程：
// 1. 逐层查询缓存，命中则直接返回并回填前面的层
// 2. 所有层都未命中，调用 loader 从数据源加载（只调用一次）
// 3. 加载成功后回填到所有层
//
// 参数：
//   - ctx: 上下文
//   - key: 缓存 key
//   - dest: 结果指针（必须是非 nil 的指针）
//   - loader: 数据加载函数（从 DB 或其他数据源加载）
//
// 示例：
//
//	var user User
//	err := cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
//	    return db.FindUserByID(ctx, 123)
//	})
func (c *Cache) GetOrLoad(
	ctx context.Context,
	key string,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {
	if key == "" {
		return ErrInvalidKey
	}
	if loader == nil {
		return ErrInvalidLoader
	}
	if dest == nil {
		return ErrInvalidDest
	}
	// dest 必须是非 nil 的指针
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return ErrInvalidDest
	}
	if len(c.layers) == 0 {
		return ErrNoLayers
	}

	// 1. 逐层查询（不嵌套 loader，使用 dummy loader 仅读取缓存）
	for i, layer := range c.layers {
		err := layer.Layer.GetOrLoad(ctx, key, layer.TTL, dest, func(ctx context.Context) (any, error) {
			return nil, ErrNotFound // 不真正加载，只查缓存
		})
		if err == nil {
			// 命中，回填到前面的层
			if !c.opts.SkipBackfill && i > 0 {
				c.backfillRange(ctx, key, dest, 0, i)
			}
			return nil
		}
		// 非 NotFound 错误记录日志，继续下一层
		if !c.isNotFound(err) {
			c.onError(ctx, layer.Name, "get", key, err)
		}
	}

	// 2. 所有层都未命中，调用 loader（只调用一次）
	val, err := loader(ctx)
	if err != nil {
		if c.isNotFound(err) {
			return ErrNotFound
		}
		return err
	}

	// 3. 将结果复制到 dest
	if err := copyValue(val, dest); err != nil {
		return err
	}

	// 4. 回填到所有层
	if !c.opts.SkipBackfill {
		c.backfillAll(ctx, key, val)
	}

	return nil
}

// backfillTimeout 回填操作的超时时间
const backfillTimeout = 5 * time.Second

// backfillAll 回填到所有层（异步执行，不阻塞主流程）
func (c *Cache) backfillAll(ctx context.Context, key string, value any) {
	// 异步执行回填，不阻塞主流程
	go func() {
		// 使用 WithoutCancel 脱离原始请求的取消信号，但保留 trace/value 等上下文信息
		// 这样即使原始请求被取消，回填操作仍能完成，且链路追踪不会丢失
		backfillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backfillTimeout)
		defer cancel() // 确保 cancel 总是被调用

		var wg sync.WaitGroup
		for _, layer := range c.layers {
			wg.Add(1)
			// 使用 goroutine 异步回填
			go func(l LayerConfig) {
				defer wg.Done()
				// 创建一个临时变量接收数据（避免并发问题）
				var temp any
				err := l.Layer.GetOrLoad(backfillCtx, key, l.TTL, &temp, func(ctx context.Context) (any, error) {
					return value, nil
				})
				if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
					c.onError(backfillCtx, l.Name, "backfill", key, err)
				}
			}(layer)
		}

		wg.Wait()
	}()
}

// backfillRange 回填到指定范围的层（异步执行，不阻塞主流程）
// 将 value 回填到 [start, end) 范围内的层
func (c *Cache) backfillRange(ctx context.Context, key string, value any, start, end int) {
	// 异步执行回填，不阻塞主流程
	go func() {
		// 使用 WithoutCancel 脱离原始请求的取消信号，但保留 trace/value 等上下文信息
		backfillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backfillTimeout)
		defer cancel() // 确保 cancel 总是被调用

		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			layer := c.layers[i]
			wg.Add(1)
			go func(l LayerConfig) {
				defer wg.Done()
				var temp any
				err := l.Layer.GetOrLoad(backfillCtx, key, l.TTL, &temp, func(ctx context.Context) (any, error) {
					return value, nil
				})
				if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
					c.onError(backfillCtx, l.Name, "backfill", key, err)
				}
			}(layer)
		}

		wg.Wait()
	}()
}

// Del 删除缓存（删除所有层）
//
// 示例：
//
//	cache.Del(ctx, "user:123", "user:456")
func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	var lastErr error
	for _, layer := range c.layers {
		err := layer.Layer.Del(ctx, keys...)
		if err != nil {
			c.onError(ctx, layer.Name, "del", keys[0], err)
			lastErr = err
		}
	}
	return lastErr
}

// LayerCount 返回缓存层数
func (c *Cache) LayerCount() int {
	return len(c.layers)
}

// isNotFound 判断是否是 NotFound 错误
func (c *Cache) isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if c.opts.IsNotFound != nil && c.opts.IsNotFound(err) {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

// onError 错误回调
func (c *Cache) onError(ctx context.Context, layer, op, key string, err error) {
	if c.opts.OnError != nil {
		c.opts.OnError(ctx, layer, op, key, err)
	}
}

// copyValue 将 src 的值复制到 dst
// 使用 JSON 序列化/反序列化确保深拷贝
func copyValue(src, dst any) error {
	if src == nil {
		return nil
	}

	// 使用 reflect 进行类型检查和赋值
	srcVal := reflect.ValueOf(src)
	dstVal := reflect.ValueOf(dst)

	// dst 必须是指针
	if dstVal.Kind() != reflect.Ptr || dstVal.IsNil() {
		return ErrInvalidDest
	}

	dstElem := dstVal.Elem()

	// 如果 src 是指针，获取其元素
	if srcVal.Kind() == reflect.Ptr {
		if srcVal.IsNil() {
			return nil
		}
		srcVal = srcVal.Elem()
	}

	// 类型兼容性检查
	if srcVal.Type().AssignableTo(dstElem.Type()) {
		dstElem.Set(srcVal)
		return nil
	}

	// 如果类型不直接兼容，尝试通过 JSON 序列化
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
