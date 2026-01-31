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
