// Package multi provides multi-layer cache with automatic backfill.
//
// Supports Local -> Redis -> DB pattern with automatic cache warming.
//
// Basic usage:
//
//	cache := multi.NewBuilder().
//	    WithLocal(localCache, 5*time.Minute).
//	    WithRedis(redisCache, 30*time.Minute).
//	    Build()
//
//	var user User
//	err := cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
//	    return db.FindUser(ctx, 123)
//	})
//
// Features:
//   - Automatic backfill from slower to faster layers
//   - Singleflight deduplication
//   - Cache stampede protection
package multi
