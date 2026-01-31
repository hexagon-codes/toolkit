package mysql

import (
	"time"
)

// Config MySQL 配置
type Config struct {
	// 基础配置
	DSN string // 数据源名称 (Data Source Name)

	// 连接池配置
	MaxOpenConns    int           // 最大打开连接数（默认：100）
	MaxIdleConns    int           // 最大空闲连接数（默认：10）
	ConnMaxLifetime time.Duration // 连接最大生命周期（默认：1小时）
	ConnMaxIdleTime time.Duration // 连接最大空闲时间（默认：10分钟）

	// 超时配置
	ConnectTimeout time.Duration // 连接超时（默认：10秒）
	ReadTimeout    time.Duration // 读超时（默认：30秒）
	WriteTimeout   time.Duration // 写超时（默认：30秒）

	// 其他配置
	ParseTime        bool   // 是否解析时间类型（默认：true）
	Charset          string // 字符集（默认：utf8mb4）
	Collation        string // 排序规则（默认：utf8mb4_unicode_ci）
	Loc              string // 时区（默认：Local）
	MaxAllowedPacket int    // 最大包大小（默认：4MB）

	// 日志
	Logger Logger // 可选的日志接口
}

// DefaultConfig 返回默认配置
func DefaultConfig(dsn string) *Config {
	return &Config{
		DSN:              dsn,
		MaxOpenConns:     100,
		MaxIdleConns:     10,
		ConnMaxLifetime:  time.Hour,
		ConnMaxIdleTime:  10 * time.Minute,
		ConnectTimeout:   10 * time.Second,
		ReadTimeout:      30 * time.Second,
		WriteTimeout:     30 * time.Second,
		ParseTime:        true,
		Charset:          "utf8mb4",
		Collation:        "utf8mb4_unicode_ci",
		Loc:              "Local",
		MaxAllowedPacket: 4 << 20, // 4MB
	}
}

// BuildDSN 构建完整的 DSN
func (c *Config) BuildDSN() string {
	if c.DSN != "" {
		return c.DSN
	}

	// 如果需要从其他字段构建 DSN，可以在这里实现
	// 格式：username:password@tcp(host:port)/dbname?params
	return ""
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
	// 默认不输出，避免污染日志
}

// Error 实现 Logger 接口
func (l *StdLogger) Error(msg string, err error) {
	// 默认不输出，避免污染日志
}
