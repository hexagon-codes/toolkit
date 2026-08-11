package multi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
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

	// errCacheMiss 内部标记，用于 GetOrLoad 逐层探测，不应被底层 IsNotFound 识别
	errCacheMiss = errors.New("multi-cache: internal cache miss")

	// ErrNilLayer 缓存层实例为 nil
	ErrNilLayer = errors.New("multi-cache: layer instance is nil")

	// ErrBackfillSaturated 表示异步回填并发槽已满，本次非关键回填被跳过。
	ErrBackfillSaturated = errors.New("multi-cache: backfill concurrency is saturated")

	// ErrInvalidOption 表示缓存选项无效。
	ErrInvalidOption = errors.New("multi-cache: invalid option")

	// ErrNilBuilder 表示构建器为 nil。
	ErrNilBuilder = errors.New("multi-cache: builder must not be nil")

	// ErrInvalidContext 表示调用方传入了 nil context。
	ErrInvalidContext = errors.New("multi-cache: context must not be nil")

	// ErrInvalidValue 表示 loader 在成功路径返回了 nil 或 typed nil。
	ErrInvalidValue = errors.New("multi-cache: loader returned a nil value")

	// ErrClosed 表示多层缓存已经关闭。
	ErrClosed = errors.New("multi-cache: cache is closed")
)

// Layer 缓存层接口（本地缓存和 Redis 缓存都实现了这个接口）
type Layer interface {
	// GetOrLoad 获取或加载数据
	GetOrLoad(ctx context.Context, key string, ttl time.Duration, dest any, loader func(ctx context.Context) (any, error)) error
	// Del 删除缓存
	Del(ctx context.Context, keys ...string) error
}

type notFoundMarker interface {
	NotFound() bool
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
//	cache, err := multi.NewCache([]multi.LayerConfig{
//	    multi.LayerConfig{Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
//	    multi.LayerConfig{Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
//	})
//	if err != nil {
//	    return err
//	}
//
//	// 使用（自动处理三层：local -> redis -> db）
//	var user User
//	err := cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
//	    return db.FindUserByID(ctx, 123)
//	})
type Cache struct {
	layers        []LayerConfig
	opts          Options
	backfillSlots chan struct{}
	sf            singleflight.Group
	keyGates      keyGateSet

	lifecycleMu     sync.Mutex
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	backfillWG      sync.WaitGroup
	closed          bool
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

	// BackfillConcurrency 限制单个 Cache 实例的异步回填并发数。
	BackfillConcurrency int
}

// Option 配置多级缓存行为。
type Option func(*Options) error

func defaultOptions() Options {
	return Options{
		IsNotFound: func(err error) bool {
			return errors.Is(err, ErrNotFound)
		},
		OnError:             nil,
		SkipBackfill:        false,
		BackfillConcurrency: defaultBackfillConcurrency,
	}
}

func applyOptions(opts ...Option) (Options, error) {
	o := defaultOptions()
	for index, fn := range opts {
		if fn == nil {
			return Options{}, fmt.Errorf("%w: option %d must not be nil", ErrInvalidOption, index)
		}
		if err := fn(&o); err != nil {
			return Options{}, fmt.Errorf("%w: option %d: %w", ErrInvalidOption, index, err)
		}
	}
	if o.IsNotFound == nil {
		return Options{}, fmt.Errorf("%w: not-found predicate must not be nil", ErrInvalidOption)
	}
	if o.BackfillConcurrency <= 0 {
		return Options{}, fmt.Errorf("%w: backfill concurrency must be greater than zero", ErrInvalidOption)
	}
	return o, nil
}

// WithIsNotFound 设置 NotFound 判断函数
func WithIsNotFound(fn func(err error) bool) Option {
	return func(o *Options) error {
		if fn == nil {
			return errors.New("not-found predicate must not be nil")
		}
		o.IsNotFound = fn
		return nil
	}
}

// WithOnError 设置错误回调
func WithOnError(fn func(ctx context.Context, layer string, op string, key string, err error)) Option {
	return func(o *Options) error {
		o.OnError = fn
		return nil
	}
}

// WithSkipBackfill 跳过回填（减少写入次数，但降低缓存命中率）
func WithSkipBackfill(skip bool) Option {
	return func(o *Options) error {
		o.SkipBackfill = skip
		return nil
	}
}

// WithBackfillConcurrency 设置单个多级缓存实例的异步回填并发上限。
func WithBackfillConcurrency(concurrency int) Option {
	return func(o *Options) error {
		if concurrency <= 0 {
			return errors.New("backfill concurrency must be greater than zero")
		}
		o.BackfillConcurrency = concurrency
		return nil
	}
}

// NewCache 创建多层缓存
//
// 参数：
//   - layers: 缓存层配置（按优先级从高到低排列，如 local -> redis）
//   - opts: 可选配置
//
// 示例：
//
//	cache, err := multi.NewCache([]multi.LayerConfig{
//	    multi.LayerConfig{Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
//	    multi.LayerConfig{Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
//	})
func NewCache(layers []LayerConfig, opts ...Option) (*Cache, error) {
	if len(layers) == 0 {
		return nil, ErrNoLayers
	}
	layerCopy := make([]LayerConfig, len(layers))
	for index, layer := range layers {
		if isNilLayer(layer.Layer) {
			return nil, fmt.Errorf("%w: layer %d (%q)", ErrNilLayer, index, layer.Name)
		}
		layerCopy[index] = layer
	}
	options, err := applyOptions(opts...)
	if err != nil {
		return nil, err
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &Cache{
		layers:          layerCopy,
		opts:            options,
		backfillSlots:   make(chan struct{}, options.BackfillConcurrency),
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
	}, nil
}

func isNilLayer(layer Layer) bool {
	if layer == nil {
		return true
	}
	value := reflect.ValueOf(layer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
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
	if ctx == nil {
		return ErrInvalidContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.isClosed() {
		return ErrClosed
	}
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
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return ErrInvalidDest
	}
	if len(c.layers) == 0 {
		return ErrNoLayers
	}

	// 1. 逐层查询（不嵌套 loader，使用 dummy loader 仅读取缓存）
	for i, layer := range c.layers {
		err := layer.Layer.GetOrLoad(ctx, key, layer.TTL, dest, func(ctx context.Context) (any, error) {
			return nil, errCacheMiss // 使用内部标记，避免被底层识别为 NotFound 而写入负缓存
		})
		if err == nil {
			// 命中，回填到前面的层
			if !c.opts.SkipBackfill && i > 0 {
				c.backfillRange(ctx, key, dest, 0, i)
			}
			return nil
		}
		// errCacheMiss 表示该层未命中，继续下一层
		if errors.Is(err, errCacheMiss) {
			continue
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		// ErrNotFound 来自负缓存命中，直接向外返回
		if c.isNotFound(err) {
			return ErrNotFound
		}
		// 其他错误记录日志，继续下一层
		c.onError(ctx, layer.Name, "get", key, err)
	}

	// 2. 所有层都未命中，调用 loader（只调用一次）
	resultChannel := c.sf.DoChan(key, func() (any, error) {
		loaderContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), sourceLoadTimeout)
		defer cancel()
		value, loadErr := loader(loaderContext)
		if loadErr != nil {
			return nil, loadErr
		}
		if isNilLayerValue(value) {
			return nil, ErrInvalidValue
		}
		if !c.opts.SkipBackfill {
			c.backfillAll(loaderContext, key, value)
		}
		return value, nil
	})

	var val any
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			if c.isNotFound(result.Err) {
				return ErrNotFound
			}
			return result.Err
		}
		val = result.Val
	}
	if isNilLayerValue(val) {
		return ErrInvalidValue
	}
	return copyValue(val, dest)
}

// backfillTimeout 回填操作的超时时间
const backfillTimeout = 5 * time.Second

const sourceLoadTimeout = 10 * time.Second

const defaultBackfillConcurrency = 4

// backfillAll 回填到所有层（异步执行，不阻塞主流程）
func (c *Cache) backfillAll(ctx context.Context, key string, value any) {
	c.scheduleBackfill(ctx, key, value, c.layers)
}

// backfillRange 回填到指定范围的层（异步执行，不阻塞主流程）
// 将 value 回填到 [start, end) 范围内的层
func (c *Cache) backfillRange(ctx context.Context, key string, value any, start, end int) {
	if start < 0 || end > len(c.layers) || start >= end {
		return
	}
	c.scheduleBackfill(ctx, key, value, c.layers[start:end])
}

func (c *Cache) scheduleBackfill(ctx context.Context, key string, value any, layers []LayerConfig) {
	// 深拷贝 value，防止异步回填与调用方竞争。
	data, err := json.Marshal(value)
	if err != nil {
		c.onError(ctx, "multi", "backfill_snapshot", key, err)
		return
	}
	var snapshot any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		c.onError(ctx, "multi", "backfill_snapshot", key, err)
		return
	}

	select {
	case c.backfillSlots <- struct{}{}:
	case <-ctx.Done():
		return
	default:
		c.onError(ctx, "multi", "backfill", key, ErrBackfillSaturated)
		return
	}
	if !c.registerBackfill() {
		<-c.backfillSlots
		c.onError(ctx, "multi", "backfill", key, ErrClosed)
		return
	}

	layers = append([]LayerConfig(nil), layers...)
	go func() {
		var callbackErrors []struct {
			layer string
			err   error
		}
		defer func() {
			<-c.backfillSlots
			c.backfillWG.Done()
			for _, callbackError := range callbackErrors {
				c.onError(context.WithoutCancel(ctx), callbackError.layer, "backfill", key, callbackError.err)
			}
		}()
		// 使用 WithoutCancel 脱离原始请求的取消信号，但保留 trace/value 等上下文信息
		backfillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backfillTimeout)
		stopLifecycleCancel := context.AfterFunc(c.lifecycleCtx, cancel)
		defer stopLifecycleCancel()
		defer cancel()

		unlock, lockErr := c.keyGates.acquire(backfillCtx, key)
		if lockErr != nil {
			return
		}
		for _, layer := range layers {
			var temp any
			err := layer.Layer.GetOrLoad(backfillCtx, key, layer.TTL, &temp, func(context.Context) (any, error) {
				return snapshot, nil
			})
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				callbackErrors = append(callbackErrors, struct {
					layer string
					err   error
				}{layer: layer.Name, err: err})
			}
			if backfillCtx.Err() != nil {
				break
			}
		}
		unlock()
	}()
}

// Del 删除缓存（删除所有层）
//
// 示例：
//
//	cache.Del(ctx, "user:123", "user:456")
func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if ctx == nil {
		return ErrInvalidContext
	}
	if len(keys) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.isClosed() {
		return ErrClosed
	}

	unique := make(map[string]struct{}, len(keys))
	orderedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		orderedKeys = append(orderedKeys, key)
	}
	if len(orderedKeys) == 0 {
		return nil
	}
	sort.Strings(orderedKeys)
	unlocks := make([]func(), 0, len(orderedKeys))
	for _, key := range orderedKeys {
		unlock, err := c.keyGates.acquire(ctx, key)
		if err != nil {
			for index := len(unlocks) - 1; index >= 0; index-- {
				unlocks[index]()
			}
			return err
		}
		unlocks = append(unlocks, unlock)
	}

	var deleteErr error
	type callbackError struct {
		layer string
		err   error
	}
	callbackErrors := make([]callbackError, 0)
	for _, layer := range c.layers {
		err := layer.Layer.Del(ctx, orderedKeys...)
		if err != nil {
			callbackErrors = append(callbackErrors, callbackError{layer: layer.Name, err: err})
			deleteErr = errors.Join(deleteErr, err)
		}
	}
	for index := len(unlocks) - 1; index >= 0; index-- {
		unlocks[index]()
	}
	for _, callback := range callbackErrors {
		c.onError(ctx, callback.layer, "del", orderedKeys[0], callback.err)
	}
	return deleteErr
}

// LayerCount 返回缓存层数
func (c *Cache) LayerCount() int {
	return len(c.layers)
}

// Close 取消并等待实例拥有的异步回填；底层 Layer 的生命周期仍由调用方管理。
func (c *Cache) Close() error {
	if c == nil {
		return nil
	}
	c.lifecycleMu.Lock()
	if !c.closed {
		c.closed = true
		c.lifecycleCancel()
	}
	c.lifecycleMu.Unlock()
	c.backfillWG.Wait()
	return nil
}

func (c *Cache) registerBackfill() bool {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.closed {
		return false
	}
	c.backfillWG.Add(1)
	return true
}

func (c *Cache) isClosed() bool {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	return c.closed
}

// isNotFound 判断是否是 NotFound 错误
func (c *Cache) isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if c.opts.IsNotFound != nil && c.opts.IsNotFound(err) {
		return true
	}
	var marker notFoundMarker
	if errors.As(err, &marker) && marker.NotFound() {
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
	if isNilLayerValue(src) {
		return ErrInvalidValue
	}

	// 使用 reflect 进行类型检查和赋值
	srcVal := reflect.ValueOf(src)
	dstVal := reflect.ValueOf(dst)

	// dst 必须是指针
	if dstVal.Kind() != reflect.Pointer || dstVal.IsNil() {
		return ErrInvalidDest
	}

	dstElem := dstVal.Elem()

	// 如果 src 是指针，获取其元素
	if srcVal.Kind() == reflect.Pointer {
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

func isNilLayerValue(value any) bool {
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
