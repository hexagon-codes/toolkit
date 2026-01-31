package logger

import (
	"log/slog"
	"time"
)

// 类型安全的属性构造函数，对应 slog 的 Attr 函数

// String 创建字符串属性
func String(key, value string) slog.Attr {
	return slog.String(key, value)
}

// Int 创建整数属性
func Int(key string, value int) slog.Attr {
	return slog.Int(key, value)
}

// Int64 创建 int64 属性
func Int64(key string, value int64) slog.Attr {
	return slog.Int64(key, value)
}

// Uint64 创建 uint64 属性
func Uint64(key string, value uint64) slog.Attr {
	return slog.Uint64(key, value)
}

// Float64 创建 float64 属性
func Float64(key string, value float64) slog.Attr {
	return slog.Float64(key, value)
}

// Bool 创建布尔属性
func Bool(key string, value bool) slog.Attr {
	return slog.Bool(key, value)
}

// Time 创建时间属性
func Time(key string, value time.Time) slog.Attr {
	return slog.Time(key, value)
}

// Duration 创建时长属性
func Duration(key string, value time.Duration) slog.Attr {
	return slog.Duration(key, value)
}

// Any 创建任意类型属性
func Any(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Group 创建属性组
func Group(key string, args ...any) slog.Attr {
	return slog.Group(key, args...)
}

// Err 创建错误属性（使用 "error" 作为 key）
func Err(err error) slog.Attr {
	return slog.Any("error", err)
}

// ErrKey 创建错误属性（自定义 key）
func ErrKey(key string, err error) slog.Attr {
	return slog.Any(key, err)
}

// --- 常用业务字段 ---

// TraceID 创建 trace_id 属性
func TraceID(id string) slog.Attr {
	return slog.String("trace_id", id)
}

// UserID 创建 user_id 属性
func UserID(id any) slog.Attr {
	return slog.Any("user_id", id)
}

// RequestID 创建 request_id 属性
func RequestID(id string) slog.Attr {
	return slog.String("request_id", id)
}

// Method 创建 method 属性（HTTP method）
func Method(method string) slog.Attr {
	return slog.String("method", method)
}

// Path 创建 path 属性（URL path）
func Path(path string) slog.Attr {
	return slog.String("path", path)
}

// Status 创建 status 属性（HTTP status code）
func Status(code int) slog.Attr {
	return slog.Int("status", code)
}

// Latency 创建 latency 属性
func Latency(d time.Duration) slog.Attr {
	return slog.Duration("latency", d)
}

// IP 创建 ip 属性
func IP(ip string) slog.Attr {
	return slog.String("ip", ip)
}

// Component 创建 component 属性
func Component(name string) slog.Attr {
	return slog.String("component", name)
}

// Action 创建 action 属性
func Action(action string) slog.Attr {
	return slog.String("action", action)
}
