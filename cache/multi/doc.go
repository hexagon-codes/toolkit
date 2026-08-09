// Package multi 提供多层缓存，支持自动回填
//
// 支持 Local -> Redis -> DB 模式，具有自动缓存预热功能。
//
// 基本用法:
//
//	cache, err := multi.NewCache([]multi.LayerConfig{
//	    {Layer: localCache, TTL: 5 * time.Minute, Name: "local"},
//	    {Layer: redisCache, TTL: 30 * time.Minute, Name: "redis"},
//	})
//	if err != nil {
//	    return err
//	}
//
// 构建器用法:
//
//	cache, err = multi.NewBuilder().
//	    WithLocal(localCache, 5*time.Minute).
//	    WithRedis(redisCache, 30*time.Minute).
//	    WithBackfillConcurrency(8).
//	    Build()
//	if err != nil {
//	    return err
//	}
//
//	var user User
//	err := cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
//	    return db.FindUser(ctx, 123)
//	})
//
// 构造契约:
//   - NewCache 和 Builder.Build 都返回 (*Cache, error)，使用缓存前必须处理错误
//   - 未配置层、nil 层和无效 Option 会在构造阶段返回可通过 errors.Is 识别的错误
//   - BackfillConcurrency 默认值为 4，且必须大于 0
//   - 回填并发槽饱和时跳过当前非关键回填，并通过 OnError 报告 ErrBackfillSaturated
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
//	cache, err := multi.NewCache([]multi.LayerConfig{
//	    {Layer: localCache, TTL: 5 * time.Minute, Name: "local"},
//	    {Layer: redisCache, TTL: 30 * time.Minute, Name: "redis"},
//	})
//	if err != nil {
//	    return err
//	}
//
// 构建器用法:
//
//	cache, err = multi.NewBuilder().
//	    WithLocal(localCache, 5*time.Minute).
//	    WithRedis(redisCache, 30*time.Minute).
//	    WithBackfillConcurrency(8).
//	    Build()
//	if err != nil {
//	    return err
//	}
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
