package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNotFound 配置项不存在
	ErrNotFound = errors.New("config: key not found")
	// ErrInvalidType 类型不匹配
	ErrInvalidType = errors.New("config: invalid type")
	// ErrUnsupportedFormat 不支持的配置文件格式
	ErrUnsupportedFormat = errors.New("config: unsupported file format")
)

// Config 配置管理器
type Config struct {
	data map[string]any
	mu   sync.RWMutex
}

// New 创建配置管理器
func New() *Config {
	return &Config{
		data: make(map[string]any),
	}
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	c := New()
	if err := c.LoadFile(path); err != nil {
		return nil, err
	}
	return c, nil
}

// LoadFile 从文件加载配置
//
// 注意：如果路径来自用户输入，调用者应先验证路径安全性
func (c *Config) LoadFile(path string) error {
	// 规范化路径，防止路径遍历
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(cleanPath))
	return c.loadData(data, ext)
}

// loadData 根据格式解析数据
func (c *Config) loadData(data []byte, format string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch format {
	case ".json":
		return json.Unmarshal(data, &c.data)
	case ".yaml", ".yml":
		return c.parseYAML(data)
	case ".toml":
		return c.parseTOML(data)
	case ".env":
		return c.parseEnv(data)
	default:
		return ErrUnsupportedFormat
	}
}

// parseYAML 简单的 YAML 解析（不依赖外部库）
// 警告：这是简化实现，只支持简单的 key: value 格式
// 不支持嵌套结构、数组、多行字符串等复杂 YAML 特性
// 对于复杂配置，建议使用 gopkg.in/yaml.v3
func (c *Config) parseYAML(data []byte) error {
	// 简化实现：只支持简单的 key: value 格式
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除引号
		value = strings.Trim(value, "\"'")

		c.data[key] = parseValue(value)
	}
	return nil
}

// parseTOML 简单的 TOML 解析（不依赖外部库）
// 警告：这是简化实现，只支持简单的 key = value 格式和基本的 [section]
// 不支持嵌套表、数组、内联表等复杂 TOML 特性
// 对于复杂配置，建议使用 github.com/BurntSushi/toml
func (c *Config) parseTOML(data []byte) error {
	// 简化实现：只支持简单的 key = value 格式
	lines := strings.Split(string(data), "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除引号
		value = strings.Trim(value, "\"'")

		if currentSection != "" {
			key = currentSection + "." + key
		}

		c.data[key] = parseValue(value)
	}
	return nil
}

// parseEnv 解析 .env 文件
func (c *Config) parseEnv(data []byte) error {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除引号
		value = strings.Trim(value, "\"'")

		c.data[key] = value
	}
	return nil
}

// parseValue 解析值的类型
func parseValue(s string) any {
	// Bool
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Int
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}

	// Float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Duration
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	// String
	return s
}

// LoadEnv 从环境变量加载配置
func (c *Config) LoadEnv(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		if prefix != "" {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			key = strings.TrimPrefix(key, prefix)
			key = strings.TrimPrefix(key, "_")
		}

		// 转换 key 格式: APP_DATABASE_HOST -> database.host
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "_", ".")

		c.data[key] = parseValue(value)
	}
}

// Set 设置配置项
func (c *Config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Get 获取配置项
func (c *Config) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 直接查找
	if v, ok := c.data[key]; ok {
		return v, true
	}

	// 尝试环境变量
	envKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if v := os.Getenv(envKey); v != "" {
		return parseValue(v), true
	}

	return nil, false
}

// GetString 获取字符串配置
func (c *Config) GetString(key string) string {
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

// GetStringDefault 获取字符串配置，带默认值
func (c *Config) GetStringDefault(key, defaultValue string) string {
	v := c.GetString(key)
	if v == "" {
		return defaultValue
	}
	return v
}

// GetInt 获取整数配置
func (c *Config) GetInt(key string) int {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return 0
}

// GetIntDefault 获取整数配置，带默认值
func (c *Config) GetIntDefault(key string, defaultValue int) int {
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetInt64 获取 int64 配置
func (c *Config) GetInt64(key string) int64 {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

// GetFloat64 获取浮点数配置
func (c *Config) GetFloat64(key string) float64 {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0
}

// GetFloat64Default 获取浮点数配置，带默认值
func (c *Config) GetFloat64Default(key string, defaultValue float64) float64 {
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// GetBool 获取布尔配置
func (c *Config) GetBool(key string) bool {
	v, ok := c.Get(key)
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1" || val == "yes"
	case int:
		return val != 0
	case int64:
		return val != 0
	}
	return false
}

// GetBoolDefault 获取布尔配置，带默认值
func (c *Config) GetBoolDefault(key string, defaultValue bool) bool {
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1" || val == "yes"
	case int:
		return val != 0
	case int64:
		return val != 0
	}
	return defaultValue
}

// GetDuration 获取时间间隔配置
func (c *Config) GetDuration(key string) time.Duration {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case time.Duration:
		return val
	case string:
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	case int64:
		return time.Duration(val)
	case int:
		return time.Duration(val)
	}
	return 0
}

// GetDurationDefault 获取时间间隔配置，带默认值
func (c *Config) GetDurationDefault(key string, defaultValue time.Duration) time.Duration {
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	switch val := v.(type) {
	case time.Duration:
		return val
	case string:
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	case int64:
		return time.Duration(val)
	case int:
		return time.Duration(val)
	}
	return defaultValue
}

// GetStringSlice 获取字符串切片配置
func (c *Config) GetStringSlice(key string) []string {
	v, ok := c.Get(key)
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		// 逗号分隔
		if val == "" {
			return nil
		}
		parts := strings.Split(val, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			result = append(result, strings.TrimSpace(p))
		}
		return result
	}
	return nil
}

// GetStringMap 获取字符串映射配置
func (c *Config) GetStringMap(key string) map[string]string {
	v, ok := c.Get(key)
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case map[string]string:
		return val
	case map[string]any:
		result := make(map[string]string)
		for k, v := range val {
			if s, ok := v.(string); ok {
				result[k] = s
			}
		}
		return result
	}
	return nil
}

// Has 判断配置项是否存在
func (c *Config) Has(key string) bool {
	_, ok := c.Get(key)
	return ok
}

// Keys 返回所有配置键
func (c *Config) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.data))
	for k := range c.data {
		keys = append(keys, k)
	}
	return keys
}

// All 返回所有配置
func (c *Config) All() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]any, len(c.data))
	for k, v := range c.data {
		result[k] = v
	}
	return result
}

// Unmarshal 将配置解析到结构体
func (c *Config) Unmarshal(v any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 转为 JSON 再解析（简化实现）
	data, err := json.Marshal(c.data)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// UnmarshalKey 将指定 key 的配置解析到结构体
func (c *Config) UnmarshalKey(key string, v any) error {
	val, ok := c.Get(key)
	if !ok {
		return ErrNotFound
	}

	// 转为 JSON 再解析
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// BindEnv 绑定环境变量到结构体字段
func BindEnv(v any, prefix string) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return ErrInvalidType
	}

	val = val.Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if !field.CanSet() {
			continue
		}

		fieldType := typ.Field(i)

		// 获取环境变量名
		envName := fieldType.Tag.Get("env")
		if envName == "" {
			envName = strings.ToUpper(fieldType.Name)
		}
		if prefix != "" {
			envName = prefix + "_" + envName
		}

		envValue := os.Getenv(envName)
		if envValue == "" {
			// 使用默认值
			if defaultValue := fieldType.Tag.Get("default"); defaultValue != "" {
				envValue = defaultValue
			} else {
				continue
			}
		}

		// 设置值
		if err := setField(field, envValue); err != nil {
			return err
		}
	}

	return nil
}

// setField 设置字段值
func setField(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
		} else {
			i, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(i)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(u)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)
	case reflect.Bool:
		b := value == "true" || value == "1" || value == "yes"
		field.SetBool(b)
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(value, ",")
			slice := reflect.MakeSlice(field.Type(), len(parts), len(parts))
			for i, p := range parts {
				slice.Index(i).SetString(strings.TrimSpace(p))
			}
			field.Set(slice)
		}
	default:
		return ErrInvalidType
	}
	return nil
}

// --- 全局配置 ---

var globalConfig = New()

// Global 获取全局配置
func Global() *Config {
	return globalConfig
}

// SetGlobal 设置全局配置
func SetGlobal(c *Config) {
	globalConfig = c
}

// LoadGlobal 加载全局配置
func LoadGlobal(path string) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	globalConfig = c
	return nil
}

// Get 从全局配置获取值
func Get(key string) (any, bool) {
	return globalConfig.Get(key)
}

// GetString 从全局配置获取字符串
func GetString(key string) string {
	return globalConfig.GetString(key)
}

// GetStringDefault 从全局配置获取字符串，带默认值
func GetStringDefault(key, defaultValue string) string {
	return globalConfig.GetStringDefault(key, defaultValue)
}

// GetInt 从全局配置获取整数
func GetInt(key string) int {
	return globalConfig.GetInt(key)
}

// GetIntDefault 从全局配置获取整数，带默认值
func GetIntDefault(key string, defaultValue int) int {
	return globalConfig.GetIntDefault(key, defaultValue)
}

// GetBool 从全局配置获取布尔值
func GetBool(key string) bool {
	return globalConfig.GetBool(key)
}

// GetDuration 从全局配置获取时间间隔
func GetDuration(key string) time.Duration {
	return globalConfig.GetDuration(key)
}

// Set 设置全局配置项
func Set(key string, value any) {
	globalConfig.Set(key, value)
}

// Has 判断全局配置项是否存在
func Has(key string) bool {
	return globalConfig.Has(key)
}
