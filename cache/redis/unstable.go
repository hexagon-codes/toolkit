package redis

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// UnstableCache 不稳定 key 缓存（用于聚合查询、JOIN、列表等）
//
// 特点：
// 1. key 不确定（列表、聚合、分页）
// 2. 批量失效（版本号或 pattern 删除）
// 3. 短 TTL（避免数据过期）
//
// 使用场景：
// - GetGroupEnabledModels(group) - 聚合查询
// - GetAllEnableAbilityWithChannels() - JOIN 查询
// - GetUserList(page, size) - 列表查询
//
// 失效策略：
// - 方案 A（推荐）：版本号（key 包含版本，更新时版本递增）
// - 方案 B：批量删除（使用 pattern 删除所有相关 key）
// - 方案 C：短 TTL（1-2分钟自动过期）
type UnstableCache struct {
	client redis.UniversalClient
	sf     singleflight.Group
	opts   Options

	versionKey       string
	version          int64
	lastVersionCheck int64
	versionSf        singleflight.Group // 版本刷新专用 singleflight
}

// NewUnstableCache 创建不稳定 key 缓存
//
// versionKey: 版本号存储的 Redis key（如 "ability:version"）
func NewUnstableCache(client redis.UniversalClient, versionKey string, opts ...Option) *UnstableCache {
	c := &UnstableCache{
		client:     client,
		opts:       applyOptions(opts...),
		versionKey: versionKey,
	}
	c.loadVersion()
	return c
}

// GetOrLoad 获取或加载聚合数据（不稳定 key）
//
// 示例 1：带版本号
//
//	var models []string
//	err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models, func(ctx context.Context) (any, error) {
//	    return db.GetGroupEnabledModels(ctx, "chat")
//	})
//
// 示例 2：不带版本号（使用短 TTL）
//
//	var abilities []Ability
//	err := cache.GetOrLoadWithoutVersion(ctx, "ability:all:enabled", 2*time.Minute, &abilities, loader)
func (c *UnstableCache) GetOrLoad(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {

	c.refreshVersionIfNeeded(ctx)

	version := c.getVersion()
	versionedKey := fmt.Sprintf("%s:v%d", key, version)
	return c.getOrLoadInternal(ctx, versionedKey, ttl, dest, loader)
}

// GetOrLoadWithoutVersion 不使用版本号的加载（使用短 TTL + 批量删除）
func (c *UnstableCache) GetOrLoadWithoutVersion(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {
	return c.getOrLoadInternal(ctx, key, ttl, dest, loader)
}

// InvalidateVersion 递增版本号（使所有使用版本号的 key 失效）
//
// 使用场景：
// - 更新 Ability 后调用
// - 更新 Channel 后调用
//
// 示例：
//
//	cache.InvalidateVersion(ctx)
func (c *UnstableCache) InvalidateVersion(ctx context.Context) error {
	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	newVersion, err := c.client.Incr(writeCtx, c.versionKey).Result()
	if err != nil {
		c.onError(ctx, "unstable_incr_version", c.versionKey, err)
		return err
	}

	atomic.StoreInt64(&c.version, newVersion)
	return nil
}

// InvalidatePattern 批量删除匹配的 key（不使用版本号时）
//
// 注意：
// - 生产环境使用 SCAN 而不是 KEYS，避免阻塞
// - 集群直连模式会遍历所有 master 节点执行 SCAN
// - 代理/单点模式直接 SCAN
//
// 示例：
//
//	cache.InvalidatePattern(ctx, "ability:group:*")
func (c *UnstableCache) InvalidatePattern(ctx context.Context, pattern string) error {
	fullPattern := joinPrefix(c.opts.Prefix, pattern)

	// 检查是否为集群直连模式
	if clusterClient, ok := c.client.(*redis.ClusterClient); ok {
		// 集群直连模式：遍历所有 master 节点
		return clusterClient.ForEachMaster(ctx, func(ctx context.Context, node *redis.Client) error {
			return c.scanAndDeleteWithClient(ctx, node, fullPattern)
		})
	}

	// 代理/单点模式：直接 SCAN
	return c.scanAndDeleteUniversal(ctx, fullPattern)
}

// scanAndDeleteWithClient 使用指定客户端扫描并删除匹配的 key（用于集群节点）
func (c *UnstableCache) scanAndDeleteWithClient(ctx context.Context, client *redis.Client, pattern string) error {
	var cursor uint64
	for {
		writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
		keys, nextCursor, err := client.Scan(writeCtx, cursor, pattern, 100).Result()
		cancel()

		if err != nil {
			c.onError(ctx, "unstable_scan", pattern, err)
			return err
		}

		if len(keys) > 0 {
			writeCtx2, cancel2 := withTimeout(ctx, c.opts.WriteTimeout)
			_, err := client.Del(writeCtx2, keys...).Result()
			cancel2()

			if err != nil {
				c.onError(ctx, "unstable_del_pattern", pattern, err)
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// scanAndDeleteUniversal 使用 UniversalClient 扫描并删除匹配的 key（用于代理/单点模式）
func (c *UnstableCache) scanAndDeleteUniversal(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
		keys, nextCursor, err := c.client.Scan(writeCtx, cursor, pattern, 100).Result()
		cancel()

		if err != nil {
			c.onError(ctx, "unstable_scan", pattern, err)
			return err
		}

		if len(keys) > 0 {
			writeCtx2, cancel2 := withTimeout(ctx, c.opts.WriteTimeout)
			_, err := c.client.Del(writeCtx2, keys...).Result()
			cancel2()

			if err != nil {
				c.onError(ctx, "unstable_del_pattern", pattern, err)
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Del 删除指定 key（支持通配符）
func (c *UnstableCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	fullKeys := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != "" {
			fullKeys = append(fullKeys, joinPrefix(c.opts.Prefix, k))
		}
	}

	if len(fullKeys) == 0 {
		return nil
	}

	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	err := c.client.Del(writeCtx, fullKeys...).Err()
	if err != nil {
		c.onError(ctx, "unstable_del", fullKeys[0], err)
	}
	return err
}

// GetVersion 获取当前版本号
func (c *UnstableCache) GetVersion() int64 {
	return c.getVersion()
}

func (c *UnstableCache) getOrLoadInternal(
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

	// 1. 先查缓存
	readCtx, cancel := withTimeout(ctx, c.opts.ReadTimeout)
	defer cancel()

	data, err := c.client.Get(readCtx, fullKey).Bytes()
	if err == nil {
		// 缓存命中
		found, payload, uerr := unpack(data)
		if uerr != nil {
			c.onError(ctx, "unstable_unpack", fullKey, uerr)
			return uerr
		}
		if !found {
			return ErrNotFound
		}
		return c.opts.Codec.Unmarshal(payload, dest)
	}

	if err != redis.Nil {
		// Redis 错误，降级
		c.onError(ctx, "unstable_get", fullKey, err)
		return c.loadAndFill(ctx, loader, dest)
	}

	// 2. 缓存未命中，singleflight 加载
	packed, err, _ := c.sf.Do(fullKey, func() (interface{}, error) {
		// 双重检查（带超时）
		checkCtx, checkCancel := withTimeout(ctx, c.opts.ReadTimeout)
		defer checkCancel()
		data2, err2 := c.client.Get(checkCtx, fullKey).Bytes()
		if err2 == nil {
			return data2, nil
		}

		// 加载数据
		val, lerr := loader(ctx)
		if lerr != nil {
			if c.isNotFound(lerr) {
				// 负缓存
				packed := packNotFound()
				c.asyncSet(ctx, fullKey, packed, c.opts.NegativeTTL)
			}
			return nil, lerr
		}

		// 序列化
		raw, merr := c.opts.Codec.Marshal(val)
		if merr != nil {
			return nil, merr
		}
		packed := packFound(raw)

		// 异步写入（限制最大 TTL，避免数据过期太久）
		actualTTL := ttl
		if c.opts.MaxTTL > 0 && actualTTL > c.opts.MaxTTL {
			actualTTL = c.opts.MaxTTL
		}
		c.asyncSet(ctx, fullKey, packed, jitterTTL(actualTTL, c.opts.Jitter))

		return packed, nil
	})

	if err != nil {
		return err
	}

	// 解包
	found, payload, uerr := unpack(packed.([]byte))
	if uerr != nil {
		return uerr
	}
	if !found {
		return ErrNotFound
	}
	return c.opts.Codec.Unmarshal(payload, dest)
}

func (c *UnstableCache) loadVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	val, err := c.client.Get(ctx, c.versionKey).Int64()
	if err == nil {
		atomic.StoreInt64(&c.version, val)
	} else if err == redis.Nil {
		// 初始化版本号
		c.client.Set(ctx, c.versionKey, 1, 0)
		atomic.StoreInt64(&c.version, 1)
	}
}

func (c *UnstableCache) getVersion() int64 {
	return atomic.LoadInt64(&c.version)
}

// refreshVersionIfNeeded 如果需要，从 Redis 重新加载版本号
// 使用 singleflight 确保同一时刻只有一个 goroutine 执行刷新
func (c *UnstableCache) refreshVersionIfNeeded(ctx context.Context) {
	now := time.Now().UnixNano()
	lastCheck := atomic.LoadInt64(&c.lastVersionCheck)

	// 1秒内不重复检查
	if now-lastCheck < int64(time.Second) {
		return
	}

	// 使用 singleflight 确保只有一个 goroutine 执行刷新
	_, _, _ = c.versionSf.Do("refresh", func() (any, error) {
		// 双重检查：进入 singleflight 后再次检查时间
		now2 := time.Now().UnixNano()
		lastCheck2 := atomic.LoadInt64(&c.lastVersionCheck)
		if now2-lastCheck2 < int64(time.Second) {
			return nil, nil
		}

		// 更新检查时间
		atomic.StoreInt64(&c.lastVersionCheck, now2)

		// 从 Redis 读取版本号
		readCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		val, err := c.client.Get(readCtx, c.versionKey).Int64()
		if err == nil {
			currentVersion := atomic.LoadInt64(&c.version)
			if val > currentVersion {
				atomic.StoreInt64(&c.version, val)
			}
		}
		return nil, nil
	})
}

func (c *UnstableCache) asyncSet(ctx context.Context, key string, data []byte, ttl time.Duration) {
	gopool.Go(func() {
		writeCtx, cancel := withTimeout(context.Background(), c.opts.WriteTimeout)
		defer cancel()

		err := c.client.Set(writeCtx, key, data, ttl).Err()
		if err != nil {
			c.onError(ctx, "unstable_set", key, err)
		}
	})
}

func (c *UnstableCache) loadAndFill(ctx context.Context, loader func(ctx context.Context) (any, error), dest any) error {
	val, err := loader(ctx)
	if err != nil {
		return err
	}
	if dest != nil {
		raw, _ := c.opts.Codec.Marshal(val)
		return c.opts.Codec.Unmarshal(raw, dest)
	}
	return nil
}

func (c *UnstableCache) isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if c.opts.IsNotFound != nil && c.opts.IsNotFound(err) {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

func (c *UnstableCache) onError(ctx context.Context, op, key string, err error) {
	if c.opts.OnError != nil {
		c.opts.OnError(ctx, op, key, err)
	}
}
