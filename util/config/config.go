// Package config 提供配置加载与合并工具。
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrNotFound 配置项不存在
	ErrNotFound = errors.New("config: key not found")
	// ErrInvalidType 类型不匹配
	ErrInvalidType = errors.New("config: invalid type")
	// ErrUnsupportedFormat 不支持的配置文件格式
	ErrUnsupportedFormat = errors.New("config: unsupported file format")
	// ErrInvalidConfig 配置内容无效
	ErrInvalidConfig = errors.New("config: invalid data")
	// ErrUnsafePath 配置路径不满足安全约束
	ErrUnsafePath = errors.New("config: unsafe path")
	// ErrInsecurePermissions 配置文件可被非所有者修改
	ErrInsecurePermissions = errors.New("config: insecure permissions")
	// ErrFileTooLarge 配置文件超过大小限制
	ErrFileTooLarge = errors.New("config: file too large")
	// ErrInvalidPrefix 环境变量前缀无效
	ErrInvalidPrefix = errors.New("config: invalid environment prefix")
)

// DefaultMaxFileBytes 是单个配置文件的默认大小上限。
const DefaultMaxFileBytes = 8 << 20

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
// 读取时固定父目录句柄，拒绝最终符号链接、非常规文件和可被组或其他用户写入的文件。
func (c *Config) LoadFile(path string) error {
	cleanPath := filepath.Clean(path)
	format, err := configFormat(cleanPath)
	if err != nil {
		return err
	}
	data, err := readConfigFile(cleanPath)
	if err != nil {
		return err
	}
	return c.loadData(data, format)
}

// loadData 根据格式解析数据
func (c *Config) loadData(data []byte, format string) error {
	if len(data) > DefaultMaxFileBytes {
		return fmt.Errorf("%w: got %d bytes, limit is %d", ErrFileTooLarge, len(data), DefaultMaxFileBytes)
	}
	parsed := make(map[string]any)
	parser := &Config{data: parsed}
	switch format {
	case ".json":
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&parsed); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
		}
		var trailing json.RawMessage
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return fmt.Errorf("%w: trailing data", ErrInvalidConfig)
			}
			return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
		}
		if parsed == nil {
			return fmt.Errorf("%w: root must be an object", ErrInvalidConfig)
		}
	case ".yaml", ".yml":
		if err := parser.parseYAML(data); err != nil {
			return err
		}
	case ".toml":
		if err := parser.parseTOML(data); err != nil {
			return err
		}
	case ".env":
		if err := parser.parseEnv(data); err != nil {
			return err
		}
	default:
		return ErrUnsupportedFormat
	}

	c.mu.Lock()
	c.data = parsed
	c.mu.Unlock()
	return nil
}

func configFormat(path string) (string, error) {
	format := strings.ToLower(filepath.Ext(path))
	switch format {
	case ".json", ".yaml", ".yml", ".toml", ".env":
		return format, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

func readConfigFile(path string) (data []byte, err error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()

	name := filepath.Base(path)
	linkInfo, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: final path is a symbolic link", ErrUnsafePath)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !fileInfo.Mode().IsRegular() || !os.SameFile(linkInfo, fileInfo) {
		return nil, fmt.Errorf("%w: path identity changed", ErrUnsafePath)
	}
	if runtime.GOOS != "windows" && fileInfo.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("%w: file must not be group-writable or world-writable", ErrInsecurePermissions)
	}
	if fileInfo.Size() > DefaultMaxFileBytes {
		return nil, fmt.Errorf("%w: got %d bytes, limit is %d", ErrFileTooLarge, fileInfo.Size(), DefaultMaxFileBytes)
	}

	data, err = io.ReadAll(io.LimitReader(file, DefaultMaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > DefaultMaxFileBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrFileTooLarge, DefaultMaxFileBytes)
	}
	return data, nil
}

// parseYAML 简单的 YAML 解析（不依赖外部库）
// 警告：这是简化实现，只支持简单的 key: value 格式
// 不支持嵌套结构、数组、多行字符串等复杂 YAML 特性
// 对于复杂配置，建议使用 gopkg.in/yaml.v3
func (c *Config) parseYAML(data []byte) error {
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) != "" && len(line) != len(strings.TrimLeft(line, " \t")) {
			return configLineError("YAML", index+1, "nested values are not supported")
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = stripInlineComment(line, true)

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return configLineError("YAML", index+1, "expected key: value")
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			return configLineError("YAML", index+1, "key is empty")
		}
		if _, exists := c.data[key]; exists {
			return configLineError("YAML", index+1, "duplicate key")
		}
		rawValue := stripInlineComment(strings.TrimSpace(parts[1]), true)
		if isUnsupportedCompositeValue(rawValue) {
			return configLineError("YAML", index+1, "composite values are not supported")
		}
		value, err := parseTextValue(rawValue, true)
		if err != nil {
			return configLineError("YAML", index+1, err.Error())
		}
		c.data[key] = value
	}
	return nil
}

// parseTOML 简单的 TOML 解析（不依赖外部库）
// 警告：这是简化实现，只支持简单的 key = value 格式和基本的 [section]
// 不支持嵌套表、数组、内联表等复杂 TOML 特性
// 对于复杂配置，建议使用 github.com/BurntSushi/toml
func (c *Config) parseTOML(data []byte) error {
	lines := strings.Split(string(data), "\n")
	currentSection := ""

	for index, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = stripInlineComment(line, false)
		if line == "" {
			continue
		}

		// Section
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return configLineError("TOML", index+1, "unterminated section")
			}
			currentSection = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if currentSection == "" {
				return configLineError("TOML", index+1, "section is empty")
			}
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return configLineError("TOML", index+1, "expected key = value")
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			return configLineError("TOML", index+1, "key is empty")
		}

		if currentSection != "" {
			key = currentSection + "." + key
		}
		if _, exists := c.data[key]; exists {
			return configLineError("TOML", index+1, "duplicate key")
		}
		rawValue := stripInlineComment(strings.TrimSpace(parts[1]), false)
		if isUnsupportedCompositeValue(rawValue) {
			return configLineError("TOML", index+1, "composite values are not supported")
		}
		value, err := parseTextValue(rawValue, true)
		if err != nil {
			return configLineError("TOML", index+1, err.Error())
		}
		c.data[key] = value
	}
	return nil
}

// parseEnv 解析 .env 文件
func (c *Config) parseEnv(data []byte) error {
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return configLineError("dotenv", index+1, "expected key=value")
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			return configLineError("dotenv", index+1, "key is empty")
		}
		if _, exists := c.data[key]; exists {
			return configLineError("dotenv", index+1, "duplicate key")
		}
		value, err := parseTextValue(strings.TrimSpace(parts[1]), false)
		if err != nil {
			return configLineError("dotenv", index+1, err.Error())
		}
		c.data[key] = value
	}
	return nil
}

func parseTextValue(raw string, inferType bool) (any, error) {
	value := raw
	if strings.HasPrefix(raw, "\"") || strings.HasSuffix(raw, "\"") {
		if len(raw) < 2 || !strings.HasPrefix(raw, "\"") || !strings.HasSuffix(raw, "\"") {
			return nil, errors.New("mismatched double quote")
		}
		unquoted, err := strconv.Unquote(raw)
		if err != nil {
			return nil, errors.New("invalid quoted value")
		}
		value = unquoted
	} else if strings.HasPrefix(raw, "'") || strings.HasSuffix(raw, "'") {
		if len(raw) < 2 || !strings.HasPrefix(raw, "'") || !strings.HasSuffix(raw, "'") {
			return nil, errors.New("mismatched single quote")
		}
		value = strings.ReplaceAll(raw[1:len(raw)-1], "''", "'")
	}
	if inferType && value == raw {
		return parseValue(value), nil
	}
	return value, nil
}

func isUnsupportedCompositeValue(raw string) bool {
	if raw == "" || strings.HasPrefix(raw, "\"") || strings.HasPrefix(raw, "'") {
		return false
	}
	return strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") ||
		strings.HasPrefix(raw, "|") || strings.HasPrefix(raw, ">")
}

func stripInlineComment(raw string, requireWhitespace bool) string {
	var quote byte
	escaped := false
	for index := 0; index < len(raw); index++ {
		current := raw[index]
		if quote != 0 {
			if quote == '"' && escaped {
				escaped = false
				continue
			}
			if quote == '"' && current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '"' || current == '\'' {
			quote = current
			continue
		}
		if current == '#' && (!requireWhitespace || index == 0 || raw[index-1] == ' ' || raw[index-1] == '\t') {
			return strings.TrimSpace(raw[:index])
		}
	}
	return raw
}

func configLineError(format string, line int, detail string) error {
	return fmt.Errorf("%w: %s line %d: %s", ErrInvalidConfig, format, line, detail)
}

// parseValue 解析值的类型
func parseValue(s string) any {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "true") {
		return true
	}
	if strings.EqualFold(s, "false") {
		return false
	}

	// Int
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}

	// Float
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f
	}

	// Duration
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	// String
	return s
}

func configInt(value any) (int, bool) {
	parsed, ok := configInt64(value)
	if !ok {
		return 0, false
	}
	if strconv.IntSize == 32 && (parsed < math.MinInt32 || parsed > math.MaxInt32) {
		return 0, false
	}
	return int(parsed), true
}

func configInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return exactFloatInt64(float64(typed))
	case float64:
		return exactFloatInt64(typed)
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func exactFloatInt64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < math.MinInt64 || value >= -math.MinInt64 {
		return 0, false
	}
	return int64(value), true
}

func configFloat64(value any) (float64, bool) {
	var parsed float64
	switch typed := value.(type) {
	case float32:
		parsed = float64(typed)
	case float64:
		parsed = typed
	case int:
		parsed = float64(typed)
	case int8:
		parsed = float64(typed)
	case int16:
		parsed = float64(typed)
	case int32:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case uint:
		parsed = float64(typed)
	case uint8:
		parsed = float64(typed)
	case uint16:
		parsed = float64(typed)
	case uint32:
		parsed = float64(typed)
	case uint64:
		parsed = float64(typed)
	case json.Number:
		var err error
		parsed, err = strconv.ParseFloat(typed.String(), 64)
		if err != nil {
			return 0, false
		}
	case string:
		var err error
		parsed, err = strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func parseConfigBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "t", "1", "yes", "on":
		return true, nil
	case "false", "f", "0", "no", "off":
		return false, nil
	default:
		return false, errors.New("invalid boolean value")
	}
}

func cloneConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneConfigValue(item)
		}
		return cloned
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneConfigValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}

// LoadEnv 从带有非空前缀的环境变量加载配置。
func (c *Config) LoadEnv(prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[string]any)
	}
	prefix = strings.TrimSuffix(prefix, "_")
	if prefix == "" {
		return ErrInvalidPrefix
	}
	prefixBoundary := prefix
	if prefixBoundary != "" {
		prefixBoundary += "_"
	}

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		if prefix != "" {
			if !strings.HasPrefix(key, prefixBoundary) {
				continue
			}
			key = strings.TrimPrefix(key, prefixBoundary)
		}
		if key == "" {
			continue
		}

		// 转换 key 格式: APP_DATABASE_HOST -> database.host
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "_", ".")

		c.data[key] = parseValue(value)
	}
	return nil
}

// Set 设置配置项
func (c *Config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[string]any)
	}
	c.data[key] = cloneConfigValue(value)
}

// Get 获取配置项
func (c *Config) Get(key string) (any, bool) {
	c.mu.RLock()
	if v, ok := c.data[key]; ok {
		result := cloneConfigValue(v)
		c.mu.RUnlock()
		return result, true
	}
	c.mu.RUnlock()

	envKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if v, ok := os.LookupEnv(envKey); ok {
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
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	value, ok := v.(string)
	if !ok {
		return defaultValue
	}
	return value
}

// GetInt 获取整数配置
func (c *Config) GetInt(key string) int {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	if value, valid := configInt(v); valid {
		return value
	}
	return 0
}

// GetIntDefault 获取整数配置，带默认值
func (c *Config) GetIntDefault(key string, defaultValue int) int {
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	if value, valid := configInt(v); valid {
		return value
	}
	return defaultValue
}

// GetInt64 获取 int64 配置
func (c *Config) GetInt64(key string) int64 {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	if value, valid := configInt64(v); valid {
		return value
	}
	return 0
}

// GetFloat64 获取浮点数配置
func (c *Config) GetFloat64(key string) float64 {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	if value, valid := configFloat64(v); valid {
		return value
	}
	return 0
}

// GetFloat64Default 获取浮点数配置，带默认值
func (c *Config) GetFloat64Default(key string, defaultValue float64) float64 {
	v, ok := c.Get(key)
	if !ok {
		return defaultValue
	}
	if value, valid := configFloat64(v); valid {
		return value
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
		parsed, err := parseConfigBool(val)
		return err == nil && parsed
	default:
		parsed, valid := configInt64(v)
		return valid && parsed != 0
	}
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
		parsed, err := parseConfigBool(val)
		if err == nil {
			return parsed
		}
	default:
		parsed, valid := configInt64(v)
		if valid {
			return parsed != 0
		}
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
	default:
		if parsed, valid := configInt64(v); valid {
			return time.Duration(parsed)
		}
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
	default:
		if parsed, valid := configInt64(v); valid {
			return time.Duration(parsed)
		}
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
	sort.Strings(keys)
	return keys
}

// All 返回所有配置
func (c *Config) All() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]any, len(c.data))
	for k, v := range c.data {
		result[k] = cloneConfigValue(v)
	}
	return result
}

// Unmarshal 将配置解析到结构体
func (c *Config) Unmarshal(v any) error {
	c.mu.RLock()
	data, err := json.Marshal(c.data)
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	return decodeConfigTarget(data, v)
}

// UnmarshalKey 将指定 key 的配置解析到结构体
func (c *Config) UnmarshalKey(key string, v any) error {
	val, ok := c.Get(key)
	if !ok {
		return ErrNotFound
	}

	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return decodeConfigTarget(data, v)
}

func decodeConfigTarget(data []byte, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return ErrInvalidType
	}
	temporary := reflect.New(rv.Elem().Type())
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(temporary.Interface()); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unmarshal config: trailing data")
		}
		return fmt.Errorf("unmarshal config: %w", err)
	}
	rv.Elem().Set(temporary.Elem())
	return nil
}

// BindEnv 绑定环境变量到结构体字段
func BindEnv(v any, prefix string) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
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

		envValue, exists := os.LookupEnv(envName)
		if !exists {
			defaultValue, hasDefault := fieldType.Tag.Lookup("default")
			if !hasDefault {
				continue
			}
			envValue = defaultValue
		}

		if err := setField(field, envValue); err != nil {
			return fmt.Errorf("bind environment variable %s to field %s: %w", envName, fieldType.Name, err)
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
			i, err := strconv.ParseInt(value, 10, field.Type().Bits())
			if err != nil {
				return err
			}
			field.SetInt(i)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(u)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return err
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("non-finite floating-point value")
		}
		field.SetFloat(f)
	case reflect.Bool:
		b, err := parseConfigBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return ErrInvalidType
		}
		if value == "" {
			field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			return nil
		}
		parts := strings.Split(value, ",")
		slice := reflect.MakeSlice(field.Type(), len(parts), len(parts))
		for i, p := range parts {
			slice.Index(i).SetString(strings.TrimSpace(p))
		}
		field.Set(slice)
	default:
		return ErrInvalidType
	}
	return nil
}

// --- 全局配置 ---

var globalConfigPtr atomic.Pointer[Config]

func init() {
	globalConfigPtr.Store(New())
}

// Global 获取全局配置
func Global() *Config {
	return globalConfigPtr.Load()
}

// SetGlobal 设置非空的全局配置。
func SetGlobal(c *Config) error {
	if c == nil {
		return ErrInvalidType
	}
	globalConfigPtr.Store(c)
	return nil
}

// LoadGlobal 加载全局配置
func LoadGlobal(path string) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	globalConfigPtr.Store(c)
	return nil
}

// Get 从全局配置获取值
func Get(key string) (any, bool) {
	return Global().Get(key)
}

// GetString 从全局配置获取字符串
func GetString(key string) string {
	return Global().GetString(key)
}

// GetStringDefault 从全局配置获取字符串，带默认值
func GetStringDefault(key, defaultValue string) string {
	return Global().GetStringDefault(key, defaultValue)
}

// GetInt 从全局配置获取整数
func GetInt(key string) int {
	return Global().GetInt(key)
}

// GetIntDefault 从全局配置获取整数，带默认值
func GetIntDefault(key string, defaultValue int) int {
	return Global().GetIntDefault(key, defaultValue)
}

// GetBool 从全局配置获取布尔值
func GetBool(key string) bool {
	return Global().GetBool(key)
}

// GetDuration 从全局配置获取时间间隔
func GetDuration(key string) time.Duration {
	return Global().GetDuration(key)
}

// Set 设置全局配置项
func Set(key string, value any) {
	Global().Set(key, value)
}

// Has 判断全局配置项是否存在
func Has(key string) bool {
	return Global().Has(key)
}
