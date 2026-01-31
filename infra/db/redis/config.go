package redis

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// Config Redis 配置
type Config struct {
	// 连接模式
	Mode Mode // single, cluster, sentinel

	// 单机模式配置
	Addr     string // 地址 (host:port)
	Password string // 密码
	DB       int    // 数据库编号 (0-15)

	// 集群模式配置
	Addrs []string // 集群节点地址列表

	// 哨兵模式配置
	MasterName    string   // 主节点名称
	SentinelAddrs []string // 哨兵节点地址列表

	// 连接池配置
	PoolSize     int           // 连接池大小（默认：10 * runtime.NumCPU()）
	MinIdleConns int           // 最小空闲连接数（默认：0）
	MaxRetries   int           // 最大重试次数（默认：3）
	PoolTimeout  time.Duration // 连接池获取超时（默认：4秒）

	// 超时配置
	DialTimeout  time.Duration // 连接超时（默认：5秒）
	ReadTimeout  time.Duration // 读超时（默认：3秒）
	WriteTimeout time.Duration // 写超时（默认：3秒）

	// 空闲检查
	IdleTimeout        time.Duration // 空闲连接超时（默认：5分钟）
	IdleCheckFrequency time.Duration // 空闲检查频率（默认：1分钟）

	// 日志
	Logger Logger // 可选的日志接口
}

// Mode Redis 运行模式
type Mode string

const (
	ModeSingle   Mode = "single"   // 单机模式
	ModeCluster  Mode = "cluster"  // 集群模式
	ModeSentinel Mode = "sentinel" // 哨兵模式
)

// DefaultConfig 返回默认单机配置
func DefaultConfig(addr string) *Config {
	return &Config{
		Mode:               ModeSingle,
		Addr:               addr,
		Password:           "",
		DB:                 0,
		PoolSize:           10,
		MinIdleConns:       2,
		MaxRetries:         3,
		PoolTimeout:        4 * time.Second,
		DialTimeout:        5 * time.Second,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		IdleTimeout:        5 * time.Minute,
		IdleCheckFrequency: time.Minute,
	}
}

// DefaultClusterConfig 返回默认集群配置
func DefaultClusterConfig(addrs []string) *Config {
	return &Config{
		Mode:               ModeCluster,
		Addrs:              addrs,
		Password:           "",
		PoolSize:           10,
		MinIdleConns:       2,
		MaxRetries:         3,
		PoolTimeout:        4 * time.Second,
		DialTimeout:        5 * time.Second,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		IdleTimeout:        5 * time.Minute,
		IdleCheckFrequency: time.Minute,
	}
}

// ToClientOptions 转换为 redis.Options
func (c *Config) ToClientOptions() *redis.Options {
	return &redis.Options{
		Addr:            c.Addr,
		Password:        c.Password,
		DB:              c.DB,
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		MaxRetries:      c.MaxRetries,
		PoolTimeout:     c.PoolTimeout,
		DialTimeout:     c.DialTimeout,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		ConnMaxIdleTime: c.IdleTimeout,
		ConnMaxLifetime: 0, // 0 表示永不过期
	}
}

// ToClusterOptions 转换为 redis.ClusterOptions
func (c *Config) ToClusterOptions() *redis.ClusterOptions {
	return &redis.ClusterOptions{
		Addrs:           c.Addrs,
		Password:        c.Password,
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		MaxRetries:      c.MaxRetries,
		PoolTimeout:     c.PoolTimeout,
		DialTimeout:     c.DialTimeout,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		ConnMaxIdleTime: c.IdleTimeout,
		ConnMaxLifetime: 0,
	}
}

// Logger 日志接口
type Logger interface {
	// Printf 格式化输出日志
	Printf(format string, args ...any)

	// Error 输出错误日志
	Error(msg string, err error)
}

// StdLogger 标准输出日志实现
type StdLogger struct{}

// Printf 实现 Logger 接口
func (l *StdLogger) Printf(format string, args ...any) {
	// 默认不输出
}

// Error 实现 Logger 接口
func (l *StdLogger) Error(msg string, err error) {
	// 默认不输出
}
