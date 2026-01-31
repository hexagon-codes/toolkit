package multi

import (
	"context"
	"time"
)

// Builder 多层缓存构建器（提供更友好的 API）
//
// 示例：
//
//	cache := multi.NewBuilder().
//	    WithLocal(localCache, 10*time.Minute).
//	    WithRedis(redisCache, 60*time.Minute).
//	    Build()
type Builder struct {
	layers []LayerConfig
	opts   []Option
}

// NewBuilder 创建构建器
func NewBuilder() *Builder {
	return &Builder{
		layers: make([]LayerConfig, 0),
		opts:   make([]Option, 0),
	}
}

// WithLayer 添加缓存层
//
// 参数：
//   - layer: 缓存层实例
//   - ttl: 该层的 TTL
//   - name: 层名称（用于日志/监控）
func (b *Builder) WithLayer(layer Layer, ttl time.Duration, name string) *Builder {
	b.layers = append(b.layers, LayerConfig{
		Layer: layer,
		TTL:   ttl,
		Name:  name,
	})
	return b
}

// WithLocal 添加本地缓存层（语义化别名）
func (b *Builder) WithLocal(layer Layer, ttl time.Duration) *Builder {
	return b.WithLayer(layer, ttl, "local")
}

// WithRedis 添加 Redis 缓存层（语义化别名）
func (b *Builder) WithRedis(layer Layer, ttl time.Duration) *Builder {
	return b.WithLayer(layer, ttl, "redis")
}

// WithOptions 添加配置选项
func (b *Builder) WithOptions(opts ...Option) *Builder {
	b.opts = append(b.opts, opts...)
	return b
}

// WithIsNotFound 设置 NotFound 判断函数
func (b *Builder) WithIsNotFound(fn func(err error) bool) *Builder {
	b.opts = append(b.opts, WithIsNotFound(fn))
	return b
}

// WithOnError 设置错误回调
func (b *Builder) WithOnError(fn func(ctx context.Context, layer string, op string, key string, err error)) *Builder {
	b.opts = append(b.opts, WithOnError(fn))
	return b
}

// WithSkipBackfill 跳过回填
func (b *Builder) WithSkipBackfill(skip bool) *Builder {
	b.opts = append(b.opts, WithSkipBackfill(skip))
	return b
}

// Build 构建多层缓存
func (b *Builder) Build() *Cache {
	return NewCache(b.layers, b.opts...)
}
