// Package redis 提供 Redis 缓存封装
//
// 实现了 cache.Layer 接口，可与多层缓存配合使用。
//
// 基本用法（redisClient 由 infra/redisconn.Factory.Open 创建并探活）:
//
//	c := redis.NewStableCache(redisClient)
//	var dest string
//	err := c.GetOrLoad(ctx, "key", 5*time.Minute, &dest, func(ctx context.Context) (any, error) {
//	    return fetchData(ctx)
//	})
//	if err != nil {
//	    // handle load failure
//	}
//	if err := c.Set(ctx, "key", dest, 5*time.Minute); err != nil {
//	    // handle write failure
//	}
//	if err := c.Del(ctx, "key"); err != nil {
//	    // handle delete failure
//	}
//
// --- English ---
//
// Package redis provides a Redis cache wrapper.
//
// Implements the cache.Layer interface for use with multi-layer cache.
//
// Basic usage (redisClient is opened and probed by infra/redisconn.Factory.Open):
//
//	c := redis.NewStableCache(redisClient)
//	var dest string
//	err := c.GetOrLoad(ctx, "key", 5*time.Minute, &dest, func(ctx context.Context) (any, error) {
//	    return fetchData(ctx)
//	})
//	if err != nil {
//	    // handle load failure
//	}
//	if err := c.Set(ctx, "key", dest, 5*time.Minute); err != nil {
//	    // handle write failure
//	}
//	if err := c.Del(ctx, "key"); err != nil {
//	    // handle delete failure
//	}
package redis
