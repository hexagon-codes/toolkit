package redis

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// UnstableCache 不稳定 key 缓存，适用于聚合查询、JOIN 和列表。
// Redis 客户端由调用方持有；UnstableCache 不会关闭客户端。
type UnstableCache struct {
	client redis.UniversalClient
	sf     singleflight.Group
	opts   Options

	versionKey       string
	version          int64
	lastVersionCheck int64
	versionSf        singleflight.Group

	mutationMu sync.RWMutex
	generation atomic.Uint64
}

// NewUnstableCache 创建不稳定 key 缓存。
func NewUnstableCache(client redis.UniversalClient, versionKey string, opts ...Option) *UnstableCache {
	cache := &UnstableCache{
		client:     client,
		opts:       applyOptions(opts...),
		versionKey: versionKey,
	}
	// Redis 暂时不可用时仍使用安全的非零本地版本降级。
	atomic.StoreInt64(&cache.version, 1)
	if !isNilInterface(client) {
		cache.loadVersion()
	}
	return cache
}

// GetOrLoad 使用版本号获取或加载聚合数据。
func (c *UnstableCache) GetOrLoad(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {
	if err := c.validateCall(ctx); err != nil {
		return err
	}
	c.refreshVersionIfNeeded(ctx)
	versionedKey := fmt.Sprintf("%s:v%d", key, c.getVersion())
	return c.getOrLoadInternal(ctx, versionedKey, ttl, dest, loader)
}

// GetOrLoadWithoutVersion 不使用版本号获取或加载数据。
func (c *UnstableCache) GetOrLoadWithoutVersion(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {
	return c.getOrLoadInternal(ctx, key, ttl, dest, loader)
}

// InvalidateVersion 递增版本号，使旧版本 key 失效。
func (c *UnstableCache) InvalidateVersion(ctx context.Context) error {
	if err := c.validateCall(ctx); err != nil {
		return err
	}
	if c.versionKey == "" {
		return ErrInvalidKey
	}
	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	c.mutationMu.Lock()
	c.generation.Add(1)
	newVersion, err := c.client.Incr(writeCtx, c.versionKey).Result()
	c.mutationMu.Unlock()
	if err != nil {
		c.onError(ctx, "unstable_incr_version", c.versionKey, err)
		return err
	}
	atomic.StoreInt64(&c.version, newVersion)
	return nil
}

// InvalidatePattern 使用 SCAN 删除匹配的 key。
func (c *UnstableCache) InvalidatePattern(ctx context.Context, pattern string) error {
	if err := c.validateCall(ctx); err != nil {
		return err
	}
	if pattern == "" {
		return ErrInvalidKey
	}
	fullPattern := joinPrefix(c.opts.Prefix, pattern)

	c.mutationMu.Lock()
	c.generation.Add(1)
	var err error
	if clusterClient, ok := c.client.(*redis.ClusterClient); ok {
		err = clusterClient.ForEachMaster(ctx, func(nodeContext context.Context, node *redis.Client) error {
			return c.scanAndDeleteWithClient(nodeContext, node, fullPattern)
		})
	} else {
		err = c.scanAndDeleteUniversal(ctx, fullPattern)
	}
	c.mutationMu.Unlock()
	if err != nil {
		c.onError(ctx, "unstable_invalidate_pattern", fullPattern, err)
	}
	return err
}

func (c *UnstableCache) scanAndDeleteWithClient(ctx context.Context, client *redis.Client, pattern string) error {
	var cursor uint64
	for {
		writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
		keys, nextCursor, err := client.Scan(writeCtx, cursor, pattern, 100).Result()
		cancel()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			deleteCtx, deleteCancel := withTimeout(ctx, c.opts.WriteTimeout)
			_, err = client.Del(deleteCtx, keys...).Result()
			deleteCancel()
			if err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

func (c *UnstableCache) scanAndDeleteUniversal(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
		keys, nextCursor, err := c.client.Scan(writeCtx, cursor, pattern, 100).Result()
		cancel()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			deleteCtx, deleteCancel := withTimeout(ctx, c.opts.WriteTimeout)
			_, err = c.client.Del(deleteCtx, keys...).Result()
			deleteCancel()
			if err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

// Del 删除指定 key。
func (c *UnstableCache) Del(ctx context.Context, keys ...string) error {
	if err := c.validateCall(ctx); err != nil {
		return err
	}
	fullKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			fullKeys = append(fullKeys, joinPrefix(c.opts.Prefix, key))
		}
	}
	if len(fullKeys) == 0 {
		return nil
	}
	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	c.mutationMu.Lock()
	c.generation.Add(1)
	err := c.client.Del(writeCtx, fullKeys...).Err()
	c.mutationMu.Unlock()
	if err != nil {
		c.onError(ctx, "unstable_del", fullKeys[0], err)
	}
	return err
}

// GetVersion 返回当前本地版本号快照。
func (c *UnstableCache) GetVersion() int64 {
	if c == nil {
		return 0
	}
	return c.getVersion()
}

func (c *UnstableCache) getOrLoadInternal(
	ctx context.Context,
	key string,
	ttl time.Duration,
	dest any,
	loader func(ctx context.Context) (any, error),
) error {
	if err := c.validateCall(ctx); err != nil {
		return err
	}
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
	generation := c.generation.Load()
	forceReload := false
	readCtx, cancel := withTimeout(ctx, c.opts.ReadTimeout)
	data, err := c.client.Get(readCtx, fullKey).Bytes()
	cancel()
	if err == nil {
		found, payload, unpackErr := unpack(data)
		switch {
		case unpackErr != nil:
			c.onError(ctx, "unstable_unpack", fullKey, unpackErr)
			forceReload = true
		case !found:
			return ErrNotFound
		default:
			decodeErr := c.opts.Codec.Unmarshal(payload, dest)
			if decodeErr == nil {
				return nil
			}
			c.onError(ctx, "unstable_decode", fullKey, decodeErr)
			forceReload = true
		}
	} else if err != redis.Nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		c.onError(ctx, "unstable_get", fullKey, err)
		return c.loadAndFill(ctx, loader, dest)
	}

	resultChannel := c.sf.DoChan(fullKey, func() (any, error) {
		sharedCtx, sharedCancel := context.WithTimeout(context.WithoutCancel(ctx), defaultLoaderTimeout)
		defer sharedCancel()
		if !forceReload {
			checkCtx, checkCancel := withTimeout(sharedCtx, c.opts.ReadTimeout)
			secondData, secondErr := c.client.Get(checkCtx, fullKey).Bytes()
			checkCancel()
			if secondErr == nil && c.payloadDecodes(secondData, dest) {
				return secondData, nil
			}
			if secondErr != nil && secondErr != redis.Nil && sharedCtx.Err() == nil {
				c.onError(sharedCtx, "unstable_double_check", fullKey, secondErr)
			}
		}

		value, loadErr := loader(sharedCtx)
		if loadErr != nil {
			if c.isNotFound(loadErr) {
				if writeErr := c.setIfCurrent(sharedCtx, fullKey, packNotFound(), c.opts.NegativeTTL, generation); writeErr != nil {
					c.onError(sharedCtx, "unstable_set_negative", fullKey, writeErr)
				}
			}
			return nil, loadErr
		}
		if isNilInterface(value) {
			return nil, ErrInvalidValue
		}
		raw, marshalErr := c.opts.Codec.Marshal(value)
		if marshalErr != nil {
			return nil, marshalErr
		}
		packed := packFound(raw)
		actualTTL := ttl
		if c.opts.MaxTTL > 0 && actualTTL > c.opts.MaxTTL {
			actualTTL = c.opts.MaxTTL
		}
		if writeErr := c.setIfCurrent(sharedCtx, fullKey, packed, jitterTTL(actualTTL, c.opts.Jitter), generation); writeErr != nil {
			c.onError(sharedCtx, "unstable_set", fullKey, writeErr)
		}
		return packed, nil
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			return result.Err
		}
		packed, ok := result.Val.([]byte)
		if !ok {
			return ErrCorrupt
		}
		found, payload, unpackErr := unpack(packed)
		if unpackErr != nil {
			return unpackErr
		}
		if !found {
			return ErrNotFound
		}
		return c.opts.Codec.Unmarshal(payload, dest)
	}
}

func (c *UnstableCache) setIfCurrent(ctx context.Context, key string, data []byte, ttl time.Duration, generation uint64) error {
	c.mutationMu.RLock()
	defer c.mutationMu.RUnlock()
	if c.generation.Load() != generation {
		return nil
	}
	writeCtx, cancel := withTimeout(context.WithoutCancel(ctx), c.opts.WriteTimeout)
	defer cancel()
	return c.client.Set(writeCtx, key, data, ttl).Err()
}

func (c *UnstableCache) payloadDecodes(packed []byte, destination any) bool {
	found, payload, err := unpack(packed)
	if err != nil || !found {
		return err == nil
	}
	probe := newDestinationLike(destination)
	return probe != nil && c.opts.Codec.Unmarshal(payload, probe) == nil
}

func (c *UnstableCache) loadVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	value, err := c.client.Get(ctx, c.versionKey).Int64()
	switch err {
	case nil:
		atomic.StoreInt64(&c.version, value)
	case redis.Nil:
		set, setErr := c.client.SetNX(ctx, c.versionKey, 1, 0).Result()
		if setErr != nil {
			c.onError(ctx, "unstable_init_version", c.versionKey, setErr)
			return
		}
		if set {
			atomic.StoreInt64(&c.version, 1)
			return
		}
		reloaded, reloadErr := c.client.Get(ctx, c.versionKey).Int64()
		if reloadErr != nil {
			c.onError(ctx, "unstable_reload_version", c.versionKey, reloadErr)
			return
		}
		atomic.StoreInt64(&c.version, reloaded)
	default:
		c.onError(ctx, "unstable_load_version", c.versionKey, err)
	}
}

func (c *UnstableCache) getVersion() int64 {
	return atomic.LoadInt64(&c.version)
}

func (c *UnstableCache) refreshVersionIfNeeded(ctx context.Context) {
	now := c.opts.Now().UnixNano()
	lastCheck := atomic.LoadInt64(&c.lastVersionCheck)
	if now >= lastCheck && now-lastCheck < int64(time.Second) {
		return
	}
	_, refreshErr, _ := c.versionSf.Do("refresh", func() (any, error) {
		now = c.opts.Now().UnixNano()
		lastCheck = atomic.LoadInt64(&c.lastVersionCheck)
		if now >= lastCheck && now-lastCheck < int64(time.Second) {
			return nil, nil
		}
		atomic.StoreInt64(&c.lastVersionCheck, now)
		readCtx, cancel := withTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		value, err := c.client.Get(readCtx, c.versionKey).Int64()
		if err != nil {
			return nil, err
		}
		if value > atomic.LoadInt64(&c.version) {
			atomic.StoreInt64(&c.version, value)
		}
		return nil, nil
	})
	if refreshErr != nil {
		c.onError(ctx, "unstable_refresh_version", c.versionKey, refreshErr)
	}
}

func (c *UnstableCache) validateCall(ctx context.Context) error {
	if c == nil || isNilInterface(c.client) {
		return ErrInvalidClient
	}
	if ctx == nil {
		return ErrInvalidContext
	}
	return ctx.Err()
}

func (c *UnstableCache) loadAndFill(ctx context.Context, loader func(ctx context.Context) (any, error), dest any) error {
	return loadAndFillCommon(ctx, c.opts.Codec, loader, dest)
}

func (c *UnstableCache) isNotFound(err error) bool {
	return isNotFoundCommon(err, c.opts.IsNotFound)
}

func (c *UnstableCache) onError(ctx context.Context, op, key string, err error) {
	if c.opts.OnError != nil {
		c.opts.OnError(ctx, op, key, err)
	}
}
