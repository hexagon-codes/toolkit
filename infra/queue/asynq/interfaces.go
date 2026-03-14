package asynq

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// =========================================
// 接口定义 - 解耦外部依赖
// =========================================
// Logger 日志接口
type Logger interface {
	// Log 普通日志
	Log(msg string)
	// LogSkip 带调用栈跳过的日志
	LogSkip(skip int, msg string)
	// Error 错误日志
	Error(msg string)
	// ErrorSkip 带调用栈跳过的错误日志
	ErrorSkip(skip int, msg string)
}

// ConfigProvider 配置提供者接口
type ConfigProvider interface {
	// IsRedisEnabled 是否启用 Redis
	IsRedisEnabled() bool
	// GetRedisAddrs 获取 Redis 地址列表
	GetRedisAddrs() []string
	// GetRedisPassword 获取 Redis 密码
	GetRedisPassword() string
	// GetRedisUsername 获取 Redis 用户名
	GetRedisUsername() string
	// GetConcurrency 获取并发数
	GetConcurrency() int
	// GetQueuePrefix 获取队列前缀（用于多环境隔离）
	GetQueuePrefix() string
	// IsPollingEnabled 是否启用轮询
	IsPollingEnabled() bool
}

// RedisConfig Redis 配置结构
type RedisConfig struct {
	Addrs    []string
	Password string
	Username string
}

// RedisClient Redis 客户端接口（用于分布式锁）
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// =========================================
// 全局实例（通过依赖注入设置，使用 atomic.Value 保证并发安全）
// =========================================
var (
	atomicLogger         atomic.Value // 存储 Logger
	atomicConfigProvider atomic.Value // 存储 ConfigProvider
	atomicRedisClient    atomic.Value // 存储 RedisClient
)

// redisClientHolder 包装 RedisClient 用于 atomic.Value（需要固定类型）
type redisClientHolder struct {
	client RedisClient
}

// SetLogger 设置全局日志实例
func SetLogger(logger Logger) {
	atomicLogger.Store(logger)
}

// SetConfigProvider 设置全局配置提供者
func SetConfigProvider(provider ConfigProvider) {
	atomicConfigProvider.Store(provider)
}

// SetRedisClient 设置全局 Redis 客户端
func SetRedisClient(client RedisClient) {
	atomicRedisClient.Store(&redisClientHolder{client: client})
}

// GetLogger 获取全局日志实例
func GetLogger() Logger {
	if v := atomicLogger.Load(); v != nil {
		return v.(Logger)
	}
	return &StdLogger{} // 默认实现
}

// GetConfigProvider 获取全局配置提供者
func GetConfigProvider() ConfigProvider {
	if v := atomicConfigProvider.Load(); v != nil {
		return v.(ConfigProvider)
	}
	return &DefaultConfigProvider{} // 默认实现
}

// GetRedisClient 获取全局 Redis 客户端
func GetRedisClient() RedisClient {
	if v := atomicRedisClient.Load(); v != nil {
		return v.(*redisClientHolder).client
	}
	return nil
}

// =========================================
// 默认实现
// =========================================
// StdLogger 标准输出日志实现
type StdLogger struct{}

func (l *StdLogger) Log(msg string) {
	println("[INFO]", msg)
}
func (l *StdLogger) LogSkip(skip int, msg string) {
	println("[INFO]", msg)
}
func (l *StdLogger) Error(msg string) {
	println("[ERROR]", msg)
}
func (l *StdLogger) ErrorSkip(skip int, msg string) {
	println("[ERROR]", msg)
}

// DefaultConfigProvider 默认配置提供者
type DefaultConfigProvider struct {
	RedisAddrs     []string
	RedisPassword  string
	RedisUsername  string
	Concurrency    int
	QueuePrefix    string
	PollingEnabled bool
	RedisEnabled   bool
}

func (c *DefaultConfigProvider) IsRedisEnabled() bool {
	return c.RedisEnabled
}
func (c *DefaultConfigProvider) GetRedisAddrs() []string {
	if len(c.RedisAddrs) == 0 {
		return []string{"localhost:6379"}
	}
	return c.RedisAddrs
}
func (c *DefaultConfigProvider) GetRedisPassword() string {
	return c.RedisPassword
}
func (c *DefaultConfigProvider) GetRedisUsername() string {
	return c.RedisUsername
}
func (c *DefaultConfigProvider) GetConcurrency() int {
	if c.Concurrency == 0 {
		return 10
	}
	return c.Concurrency
}
func (c *DefaultConfigProvider) GetQueuePrefix() string {
	return c.QueuePrefix
}
func (c *DefaultConfigProvider) IsPollingEnabled() bool {
	return c.PollingEnabled
}
