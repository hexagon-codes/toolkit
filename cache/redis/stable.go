package redis

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// StableCache 稳定 key 缓存（用于 FindOne 等单条记录查询）。
// Redis 客户端由调用方持有；StableCache 不会关闭客户端。
type StableCache struct {
	client redis.UniversalClient
	sf     singleflight.Group
	opts   Options

	// mutationMu 将主动写入、删除与共享加载回填线性化。
	mutationMu sync.RWMutex
	generation atomic.Uint64
}

// NewStableCache 创建稳定 key 缓存。
func NewStableCache(client redis.UniversalClient, opts ...Option) *StableCache {
	return &StableCache{
		client: client,
		opts:   applyOptions(opts...),
	}
}

// GetOrLoad 获取或加载单条记录（稳定 key）。
func (c *StableCache) GetOrLoad(
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
			c.onError(ctx, "stable_unpack", fullKey, unpackErr)
			forceReload = true
		case !found:
			return ErrNotFound
		default:
			if decodeErr := c.opts.Codec.Unmarshal(payload, dest); decodeErr == nil {
				return nil
			} else {
				c.onError(ctx, "stable_decode", fullKey, decodeErr)
				forceReload = true
			}
		}
	} else if err != redis.Nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		c.onError(ctx, "stable_get", fullKey, err)
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
				c.onError(sharedCtx, "stable_double_check", fullKey, secondErr)
			}
		}

		value, loadErr := loader(sharedCtx)
		if loadErr != nil {
			if c.isNotFound(loadErr) {
				if writeErr := c.setIfCurrent(sharedCtx, fullKey, packNotFound(), c.opts.NegativeTTL, generation); writeErr != nil {
					c.onError(sharedCtx, "stable_set_negative", fullKey, writeErr)
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
		if writeErr := c.setIfCurrent(sharedCtx, fullKey, packed, jitterTTL(ttl, c.opts.Jitter), generation); writeErr != nil {
			// 缓存写失败不应覆盖已经成功的数据源读取。
			c.onError(sharedCtx, "stable_set", fullKey, writeErr)
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

// Del 删除指定 key（精确失效）。
func (c *StableCache) Del(ctx context.Context, keys ...string) error {
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
		c.onError(ctx, "stable_del", fullKeys[0], err)
	}
	return err
}

// Set 主动写入缓存（Write-Through 模式）。
func (c *StableCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := c.validateCall(ctx); err != nil {
		return err
	}
	if key == "" {
		return ErrInvalidKey
	}
	if isNilInterface(value) {
		return ErrInvalidValue
	}
	raw, err := c.opts.Codec.Marshal(value)
	if err != nil {
		return err
	}
	fullKey := joinPrefix(c.opts.Prefix, key)
	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	c.mutationMu.Lock()
	c.generation.Add(1)
	err = c.client.Set(writeCtx, fullKey, packFound(raw), jitterTTL(ttl, c.opts.Jitter)).Err()
	c.mutationMu.Unlock()
	if err != nil {
		c.onError(ctx, "stable_set_sync", fullKey, err)
	}
	return err
}

func (c *StableCache) setIfCurrent(ctx context.Context, key string, data []byte, ttl time.Duration, generation uint64) error {
	c.mutationMu.RLock()
	defer c.mutationMu.RUnlock()
	if c.generation.Load() != generation {
		return nil
	}
	writeCtx, cancel := withTimeout(context.WithoutCancel(ctx), c.opts.WriteTimeout)
	defer cancel()
	return c.client.Set(writeCtx, key, data, ttl).Err()
}

func (c *StableCache) payloadDecodes(packed []byte, destination any) bool {
	found, payload, err := unpack(packed)
	if err != nil || !found {
		return err == nil
	}
	probe := newDestinationLike(destination)
	return probe != nil && c.opts.Codec.Unmarshal(payload, probe) == nil
}

func (c *StableCache) validateCall(ctx context.Context) error {
	if c == nil || isNilInterface(c.client) {
		return ErrInvalidClient
	}
	if ctx == nil {
		return ErrInvalidContext
	}
	return ctx.Err()
}

func (c *StableCache) loadAndFill(ctx context.Context, loader func(ctx context.Context) (any, error), dest any) error {
	return loadAndFillCommon(ctx, c.opts.Codec, loader, dest)
}

func (c *StableCache) isNotFound(err error) bool {
	return isNotFoundCommon(err, c.opts.IsNotFound)
}

func (c *StableCache) onError(ctx context.Context, op, key string, err error) {
	if c.opts.OnError != nil {
		c.opts.OnError(ctx, op, key, err)
	}
}
