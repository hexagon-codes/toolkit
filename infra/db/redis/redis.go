package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

// Client Redis 客户端封装
type Client struct {
	goredis.UniversalClient
}

// New 创建新的 Redis 客户端
func New(ctx context.Context, config Config) (*Client, error) {
	factory, err := redisconn.NewFactory(config)
	if err != nil {
		return nil, fmt.Errorf("create redis connection factory: %w", err)
	}

	client, err := factory.Open(ctx)
	if err != nil {
		return nil, fmt.Errorf("open redis connection: %w", err)
	}

	return newClient(client), nil
}

func newClient(client goredis.UniversalClient) *Client {
	return &Client{UniversalClient: client}
}

// Health 健康检查
func (c *Client) Health(ctx context.Context) error {
	if c == nil || c.UniversalClient == nil {
		return fmt.Errorf("redis client is nil")
	}
	if ctx == nil {
		return fmt.Errorf("%w: health check", ErrInvalidContext)
	}

	if err := c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}

	return nil
}

// GetWithDefault 获取值，不存在时返回默认值
func (c *Client) GetWithDefault(ctx context.Context, key, defaultValue string) (string, error) {
	val, err := c.Get(ctx, key).Result()
	if err == nil {
		return val, nil
	}
	if errors.Is(err, goredis.Nil) {
		return defaultValue, nil
	}
	return "", fmt.Errorf("get redis value: %w", err)
}

// SetWithExpire 设置值并指定过期时间
func (c *Client) SetWithExpire(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.Set(ctx, key, value, expiration).Err()
}

// SetNX 仅当 key 不存在时设置（分布式锁基础）
func (c *Client) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	return c.SetNXEx(ctx, key, value, expiration)
}

// SetNXEx SetNX 的封装
func (c *Client) SetNXEx(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	return c.UniversalClient.SetNX(ctx, key, value, expiration).Result()
}

// MGetValues 批量获取（简化版）
func (c *Client) MGetValues(ctx context.Context, keys ...string) ([]any, error) {
	return c.UniversalClient.MGet(ctx, keys...).Result()
}

// MSetValues 批量设置（简化版）
func (c *Client) MSetValues(ctx context.Context, values ...any) error {
	return c.UniversalClient.MSet(ctx, values...).Err()
}

// IncrByWithExpire 自增并设置过期时间（如果 key 不存在）
func (c *Client) IncrByWithExpire(ctx context.Context, key string, value int64, expiration time.Duration) (int64, error) {
	pipe := c.Pipeline()
	incrCmd := pipe.IncrBy(ctx, key, value)
	expireCmd := pipe.Expire(ctx, key, expiration)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}

	// 检查各步骤是否成功
	if err := incrCmd.Err(); err != nil {
		return 0, err
	}
	if err := expireCmd.Err(); err != nil {
		// Expire 失败但 IncrBy 成功，返回值但记录警告
		// 这种情况很少发生，通常是 key 类型不匹配
		return incrCmd.Val(), err
	}

	return incrCmd.Val(), nil
}

// ExistsCount 检查 key 是否存在并返回数量
func (c *Client) ExistsCount(ctx context.Context, keys ...string) (int64, error) {
	return c.UniversalClient.Exists(ctx, keys...).Result()
}

// DeleteKeys 删除 key（简化版）
func (c *Client) DeleteKeys(ctx context.Context, keys ...string) error {
	return c.Del(ctx, keys...).Err()
}

// SetExpireAt 设置过期时间戳
func (c *Client) SetExpireAt(ctx context.Context, key string, tm time.Time) error {
	return c.UniversalClient.ExpireAt(ctx, key, tm).Err()
}

// GetTTL 获取剩余过期时间
func (c *Client) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	return c.UniversalClient.TTL(ctx, key).Result()
}

// Close 关闭客户端
func (c *Client) Close() error {
	if c == nil || c.UniversalClient == nil {
		return nil
	}
	return c.UniversalClient.Close()
}

// Stats 返回连接池统计信息
func (c *Client) Stats() *goredis.PoolStats {
	if c == nil || c.UniversalClient == nil {
		return nil
	}
	return c.PoolStats()
}
