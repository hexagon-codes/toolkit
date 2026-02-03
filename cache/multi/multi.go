package multi

import (
	"context"
	"encoding/json"
	"errors"
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
// 示例：
//
//	cache := multi.NewCache(
//	    multi.LayerConfig{Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
//	    multi.LayerConfig{Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
//	)
func NewCache(layers []LayerConfig, opts ...Option) *Cache {
	return &Cache{
		layers: layers,
		opts:   applyOptions(opts...),
	}
}

// GetOrLoad 获取或加载数据（自动处理多层缓存）
//
// 工作流程：
// 1. 从第一层（如 local）开始查询
// 2. 命中则直接返回
// 3. 未命中则查询下一层（如 redis）
// 4. 找到数据后回填到前面的层
// 5. 所有层都未命中，调用 loader 从数据源加载
// 6. 加载成功后写入所有层
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
	if len(c.layers) == 0 {
		return ErrNoLayers
	}

	// 逐层查询
	for i, layer := range c.layers {
		err := layer.Layer.GetOrLoad(ctx, key, layer.TTL, dest, func(ctx context.Context) (any, error) {
			// 这一层未命中，尝试下一层
			if i == len(c.layers)-1 {
				// 最后一层了，调用 loader
				return loader(ctx)
			}

			// 查询下一层
			nextLayer := c.layers[i+1]
			var temp any = dest // 用于接收下一层的数据
			err := nextLayer.Layer.GetOrLoad(ctx, key, nextLayer.TTL, dest, func(ctx context.Context) (any, error) {
				// 递归查询更深层或 loader
				return c.loadFromNextLayers(ctx, key, dest, i+1, loader)
			})

			if err != nil {
				return nil, err
			}
			return temp, nil
		})

		if err == nil {
			// 命中，返回结果
			return nil
		}

		// 判断是否是 NotFound 错误
		if c.isNotFound(err) {
			return ErrNotFound
		}

		// 其他错误，记录并继续尝试下一层
		c.onError(ctx, layer.Name, "get", key, err)
	}

	// 所有层都失败，直接调用 loader
	val, err := loader(ctx)
	if err != nil {
		if c.isNotFound(err) {
			return ErrNotFound
		}
		return err
	}

	// 回填到所有层
	if !c.opts.SkipBackfill {
		c.backfillAll(ctx, key, val)
	}

	// 将结果复制到 dest
	return copyValue(val, dest)
}

// loadFromNextLayers 从指定层开始加载（内部递归辅助函数）
func (c *Cache) loadFromNextLayers(
	ctx context.Context,
	key string,
	dest any,
	startIndex int,
	loader func(ctx context.Context) (any, error),
) (any, error) {
	// 遍历剩余层
	for i := startIndex; i < len(c.layers); i++ {
		layer := c.layers[i]
		var temp any = dest
		err := layer.Layer.GetOrLoad(ctx, key, layer.TTL, dest, func(ctx context.Context) (any, error) {
			// 最后一层了，调用 loader
			if i == len(c.layers)-1 {
				return loader(ctx)
			}
			// 继续下一层
			return c.loadFromNextLayers(ctx, key, dest, i+1, loader)
		})

		if err == nil {
			// 找到数据，回填到前面的层
			if !c.opts.SkipBackfill && i > startIndex {
				c.backfillRange(ctx, key, temp, startIndex, i)
			}
			return temp, nil
		}

		// NotFound 直接返回
		if c.isNotFound(err) {
			return nil, err
		}

		// 其他错误，记录并继续
		c.onError(ctx, layer.Name, "get", key, err)
	}

	// 所有层都失败，调用 loader
	return loader(ctx)
}

// backfillTimeout 回填操作的超时时间
const backfillTimeout = 5 * time.Second

// backfillAll 回填到所有层
func (c *Cache) backfillAll(ctx context.Context, key string, value any) {
	var wg sync.WaitGroup
	// 创建带超时的 context，继承原始 ctx 的取消信号
	// 使用较短的超时：原始 ctx 剩余时间 vs backfillTimeout
	backfillCtx, cancel := context.WithTimeout(ctx, backfillTimeout)

	for _, layer := range c.layers {
		wg.Add(1)
		// 使用 goroutine 异步回填，避免阻塞主流程
		go func(l LayerConfig) {
			defer wg.Done()
			// 创建一个临时变量接收数据（避免并发问题）
			var temp any
			err := l.Layer.GetOrLoad(backfillCtx, key, l.TTL, &temp, func(ctx context.Context) (any, error) {
				return value, nil
			})
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				c.onError(ctx, l.Name, "backfill", key, err)
			}
		}(layer)
	}

	// 在新 goroutine 中等待完成并取消 context
	go func() {
		wg.Wait()
		cancel()
	}()
}

// backfillRange 回填到指定范围的层
func (c *Cache) backfillRange(ctx context.Context, key string, value any, start, end int) {
	var wg sync.WaitGroup
	// 继承原始 ctx 的取消信号
	backfillCtx, cancel := context.WithTimeout(ctx, backfillTimeout)

	for i := start; i < end; i++ {
		layer := c.layers[i]
		wg.Add(1)
		go func(l LayerConfig) {
			defer wg.Done()
			var temp any
			err := l.Layer.GetOrLoad(backfillCtx, key, l.TTL, &temp, func(ctx context.Context) (any, error) {
				return value, nil
			})
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				c.onError(ctx, l.Name, "backfill", key, err)
			}
		}(layer)
	}

	go func() {
		wg.Wait()
		cancel()
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
