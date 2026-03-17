// Package redis 提供 Redis 缓存封装
//
// 实现了 cache.Layer 接口，可与多层缓存配合使用。
//
// 基本用法:
//
//	c := redis.New(redisClient)
//	c.Set(ctx, "key", value, 5*time.Minute)
//	val, ok := c.Get(ctx, "key")
//
// GetOrLoad 模式:
//
//	val, err := c.GetOrLoad(ctx, "key", ttl, &dest, func(ctx context.Context) (any, error) {
//	    return fetchData(ctx)
//	})
//
// --- English ---
//
// Package redis provides a Redis cache wrapper.
//
// Implements the cache.Layer interface for use with multi-layer cache.
//
// Basic usage:
//
//	c := redis.New(redisClient)
//	c.Set(ctx, "key", value, 5*time.Minute)
//	val, ok := c.Get(ctx, "key")
//
// GetOrLoad pattern:
//
//	val, err := c.GetOrLoad(ctx, "key", ttl, &dest, func(ctx context.Context) (any, error) {
//	    return fetchData(ctx)
//	})
package redis
