// Package multi 提供多层缓存，支持自动回填
//
// 支持 Local -> Redis -> DB 模式，具有自动缓存预热功能。
//
// 基本用法:
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
// 特性:
//   - 从慢速层自动回填到快速层
//   - Singleflight 请求合并去重
//   - 缓存雪崩防护
//
// --- English ---
//
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
