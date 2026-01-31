package redis

import (
	"context"
	"errors"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// StableCache 稳定 key 缓存（用于 FindOne 等单条记录查询）
//
// 特点：
// 1. key 确定性强（如 user:{id}, channel:{id}）
// 2. 精确失效（更新时删除对应 key）
// 3. 适合单条记录查询
//
// 使用场景：
// - GetUserByID(id)
// - GetChannelByID(id)
// - GetAbilityByID(id)
type StableCache struct {
	client redis.UniversalClient
	sf     singleflight.Group
	opts   Options
}

// NewStableCache 创建稳定 key 缓存
func NewStableCache(client redis.UniversalClient, opts ...Option) *StableCache {
	return &StableCache{
		client: client,
		opts:   applyOptions(opts...),
	}
}

// GetOrLoad 获取或加载单条记录（稳定 key）
//
// 示例：
//
//	var user User
//	err := cache.GetOrLoad(ctx, "user:123", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
//	    return db.FindUserByID(ctx, 123)
//	})
func (c *StableCache) GetOrLoad(
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

	// 1. 先查缓存（带超时）
	readCtx, cancel := withTimeout(ctx, c.opts.ReadTimeout)
	defer cancel()

	data, err := c.client.Get(readCtx, fullKey).Bytes()
	if err == nil {
		// 缓存命中
		found, payload, uerr := unpack(data)
		if uerr != nil {
			c.onError(ctx, "stable_unpack", fullKey, uerr)
			return uerr
		}
		if !found {
			// 负缓存命中
			return ErrNotFound
		}
		return c.opts.Codec.Unmarshal(payload, dest)
	}

	if err != redis.Nil {
		// Redis 错误，降级到直接加载
		c.onError(ctx, "stable_get", fullKey, err)
		return c.loadAndFill(ctx, loader, dest)
	}

	// 2. 缓存未命中，使用 singleflight 防击穿
	packed, err, _ := c.sf.Do(fullKey, func() (interface{}, error) {
		// 双重检查（带超时）
		checkCtx, checkCancel := withTimeout(ctx, c.opts.ReadTimeout)
		defer checkCancel()
		data2, err2 := c.client.Get(checkCtx, fullKey).Bytes()
		if err2 == nil {
			return data2, nil
		}

		// 执行加载
		val, lerr := loader(ctx)
		if lerr != nil {
			if c.isNotFound(lerr) {
				// 缓存空值（负缓存）
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

		// 异步写入缓存（带 jitter）
		c.asyncSet(ctx, fullKey, packed, jitterTTL(ttl, c.opts.Jitter))

		return packed, nil
	})

	if err != nil {
		return err
	}

	// 解包并填充
	found, payload, uerr := unpack(packed.([]byte))
	if uerr != nil {
		return uerr
	}
	if !found {
		return ErrNotFound
	}
	return c.opts.Codec.Unmarshal(payload, dest)
}

// Del 删除指定 key（精确失效）
//
// 示例：
//
//	cache.Del(ctx, "user:123", "user:456")
func (c *StableCache) Del(ctx context.Context, keys ...string) error {
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
		c.onError(ctx, "stable_del", fullKeys[0], err)
	}
	return err
}

// Set 主动写入缓存（Write-Through 模式）
//
// 示例：
//
//	cache.Set(ctx, "option:ModelRatio", "{}", 60*time.Minute)
func (c *StableCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if key == "" {
		return ErrInvalidKey
	}

	fullKey := joinPrefix(c.opts.Prefix, key)

	// 序列化
	raw, err := c.opts.Codec.Marshal(value)
	if err != nil {
		return err
	}
	packed := packFound(raw)

	// 写入缓存（带 jitter）
	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	err = c.client.Set(writeCtx, fullKey, packed, jitterTTL(ttl, c.opts.Jitter)).Err()
	if err != nil {
		c.onError(ctx, "stable_set_sync", fullKey, err)
	}
	return err
}

func (c *StableCache) asyncSet(ctx context.Context, key string, data []byte, ttl time.Duration) {
	gopool.Go(func() {
		writeCtx, cancel := withTimeout(context.Background(), c.opts.WriteTimeout)
		defer cancel()

		err := c.client.Set(writeCtx, key, data, ttl).Err()
		if err != nil {
			c.onError(ctx, "stable_set", key, err)
		}
	})
}

func (c *StableCache) loadAndFill(ctx context.Context, loader func(ctx context.Context) (any, error), dest any) error {
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

func (c *StableCache) isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if c.opts.IsNotFound != nil && c.opts.IsNotFound(err) {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

func (c *StableCache) onError(ctx context.Context, op, key string, err error) {
	if c.opts.OnError != nil {
		c.opts.OnError(ctx, op, key, err)
	}
}
