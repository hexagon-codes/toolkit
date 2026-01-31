// Package local provides an in-memory cache with TTL support.
//
// Features automatic expiration, size limits, and singleflight deduplication.
//
// Basic usage:
//
//	c := local.New()
//	c.Set("key", value, 5*time.Minute)
//	val, ok := c.Get("key")
//
// With options:
//
//	c := local.New(
//	    local.WithMaxSize(1000),
//	    local.WithCleanupInterval(time.Minute),
//	)
//
// GetOrLoad pattern:
//
//	val, err := c.GetOrLoad(ctx, "key", ttl, func(ctx context.Context) (any, error) {
//	    return db.Find(ctx, id)
//	})
package local
