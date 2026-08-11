// Package env 提供环境变量的类型化访问能力。
package env

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Get 获取环境变量，不存在返回空字符串
func Get(key string) string {
	return os.Getenv(key)
}

// GetDefault 获取环境变量，不存在返回默认值
func GetDefault(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

// Lookup 查找环境变量，返回值和是否存在
func Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

// MustGet 获取环境变量，不存在则 panic
//
// ⚠️ 警告：仅在程序初始化时使用（如 init 函数或 main 开头）
// 不要在请求处理路径中使用，否则缺少配置会导致服务崩溃
// 建议在容器/云部署前验证所有必需的环境变量
func MustGet(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		panic("environment variable not set: " + key)
	}
	return val
}

// GetInt 获取整数环境变量
func GetInt(key string) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return 0
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return i
}

// GetIntDefault 获取整数环境变量，不存在或解析失败返回默认值
func GetIntDefault(key string, defaultVal int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// GetInt64 获取 int64 环境变量
func GetInt64(key string) int64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return 0
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0
	}
	return i
}

// GetInt64Default 获取 int64 环境变量，不存在或解析失败返回默认值
func GetInt64Default(key string, defaultVal int64) int64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultVal
	}
	return i
}

// GetFloat64 获取 float64 环境变量
func GetFloat64(key string) float64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// GetFloat64Default 获取 float64 环境变量，不存在或解析失败返回默认值
func GetFloat64Default(key string, defaultVal float64) float64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return defaultVal
	}
	return f
}

// GetBool 获取布尔环境变量
// 支持: "true", "1", "yes", "on" (不区分大小写) 返回 true
// 其他值返回 false
func GetBool(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return val == "true" || val == "1" || val == "yes" || val == "on"
}

// GetBoolDefault 获取布尔环境变量，不存在返回默认值
func GetBoolDefault(key string, defaultVal bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	val = strings.ToLower(strings.TrimSpace(val))
	return val == "true" || val == "1" || val == "yes" || val == "on"
}

// GetDuration 获取时间间隔环境变量
// 支持格式: "1s", "5m", "2h" 等
func GetDuration(key string) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return 0
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0
	}
	return d
}

// GetDurationDefault 获取时间间隔环境变量，不存在或解析失败返回默认值
func GetDurationDefault(key string, defaultVal time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

// GetSlice 获取切片环境变量（逗号分隔）
func GetSlice(key string) []string {
	val := os.Getenv(key)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// GetSliceDefault 获取切片环境变量，不存在返回默认值
func GetSliceDefault(key string, defaultVal []string) []string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return append([]string(nil), defaultVal...)
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// Set 设置环境变量
func Set(key, value string) error {
	return os.Setenv(key, value)
}

// Unset 删除环境变量
func Unset(key string) error {
	return os.Unsetenv(key)
}

// Exists 判断环境变量是否存在
func Exists(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

// IsProd 判断是否为生产环境
// 检查 GO_ENV, ENV, ENVIRONMENT 变量
func IsProd() bool {
	value := currentEnvironment()
	return value == "prod" || value == "production"
}

// IsDev 判断是否为开发环境
func IsDev() bool {
	value := currentEnvironment()
	return value == "dev" || value == "development" || value == "local"
}

// IsTest 判断是否为测试环境
func IsTest() bool {
	value := currentEnvironment()
	return value == "test" || value == "testing" || value == "staging"
}

// currentEnvironment 按固定优先级返回第一个非空环境标识。
func currentEnvironment() string {
	envVars := []string{"GO_ENV", "ENV", "ENVIRONMENT"}
	for _, envVar := range envVars {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(envVar)))
		if value != "" {
			return value
		}
	}
	return ""
}
