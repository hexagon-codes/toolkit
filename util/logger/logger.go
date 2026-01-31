package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	defaultLogger *Logger
	once          sync.Once
)

// Logger 日志记录器
type Logger struct {
	slog   *slog.Logger
	level  *slog.LevelVar
	config *Config
	output io.Writer // 保存 output 引用用于关闭
}

// Config 日志配置
type Config struct {
	// Level 日志级别: debug, info, warn, error
	Level string `json:"level" yaml:"level"`

	// Format 输出格式: json, text
	Format string `json:"format" yaml:"format"`

	// Output 输出目标: stdout, stderr, 或文件路径
	Output string `json:"output" yaml:"output"`

	// AddSource 是否添加调用位置
	AddSource bool `json:"addSource" yaml:"addSource"`

	// TimeFormat 时间格式，空则使用默认
	TimeFormat string `json:"timeFormat" yaml:"timeFormat"`

	// File 文件配置（当 Output 为文件路径时生效）
	File *FileConfig `json:"file" yaml:"file"`
}

// FileConfig 文件配置
type FileConfig struct {
	// MaxSize 单个文件最大大小（MB）
	MaxSize int `json:"maxSize" yaml:"maxSize"`

	// MaxBackups 最大保留文件数
	MaxBackups int `json:"maxBackups" yaml:"maxBackups"`

	// MaxAge 最大保留天数
	MaxAge int `json:"maxAge" yaml:"maxAge"`

	// Compress 是否压缩旧文件
	Compress bool `json:"compress" yaml:"compress"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Level:     "info",
		Format:    "json",
		Output:    "stdout",
		AddSource: false,
		File: &FileConfig{
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     7,
			Compress:   true,
		},
	}
}

// Init 初始化全局日志记录器
func Init(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	logger, err := New(cfg)
	if err != nil {
		return err
	}

	defaultLogger = logger
	slog.SetDefault(logger.slog)
	return nil
}

// New 创建新的日志记录器
func New(cfg *Config) (*Logger, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 解析日志级别
	levelVar := &slog.LevelVar{}
	levelVar.Set(parseLevel(cfg.Level))

	// 获取输出
	output, err := getOutput(cfg)
	if err != nil {
		return nil, err
	}

	// 创建 handler
	opts := &slog.HandlerOptions{
		Level:     levelVar,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(output, opts)
	} else {
		handler = slog.NewJSONHandler(output, opts)
	}

	return &Logger{
		slog:   slog.New(handler),
		level:  levelVar,
		config: cfg,
		output: output,
	}, nil
}

// getOutput 获取输出 writer
func getOutput(cfg *Config) (io.Writer, error) {
	switch cfg.Output {
	case "stdout", "":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	default:
		// 文件输出
		return newFileWriter(cfg.Output, cfg.File)
	}
}

// parseLevel 解析日志级别
func parseLevel(level string) slog.Level {
	switch level {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "info", "INFO", "":
		return slog.LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Default 获取默认日志记录器
func Default() *Logger {
	once.Do(func() {
		if defaultLogger == nil {
			defaultLogger, _ = New(DefaultConfig())
			slog.SetDefault(defaultLogger.slog)
		}
	})
	return defaultLogger
}

// SetDefault 设置默认日志记录器
func SetDefault(l *Logger) {
	defaultLogger = l
	slog.SetDefault(l.slog)
}

// SetLevel 动态设置日志级别
func (l *Logger) SetLevel(level string) {
	l.level.Set(parseLevel(level))
}

// With 创建带有固定字段的子记录器
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		slog:   l.slog.With(args...),
		level:  l.level,
		config: l.config,
	}
}

// WithGroup 创建带有分组的子记录器
func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		slog:   l.slog.WithGroup(name),
		level:  l.level,
		config: l.config,
	}
}

// Close 关闭日志记录器，释放文件资源
// 如果 output 是 stdout/stderr，则不做任何操作
func (l *Logger) Close() error {
	if closer, ok := l.output.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Debug 记录 Debug 级别日志
func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, args...)
}

// Info 记录 Info 级别日志
func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, args...)
}

// Warn 记录 Warn 级别日志
func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, args...)
}

// Error 记录 Error 级别日志
func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, args...)
}

// DebugContext 记录带 context 的 Debug 级别日志
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.slog.DebugContext(ctx, msg, args...)
}

// InfoContext 记录带 context 的 Info 级别日志
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.slog.InfoContext(ctx, msg, args...)
}

// WarnContext 记录带 context 的 Warn 级别日志
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.slog.WarnContext(ctx, msg, args...)
}

// ErrorContext 记录带 context 的 Error 级别日志
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.slog.ErrorContext(ctx, msg, args...)
}

// Log 记录指定级别的日志
func (l *Logger) Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	l.slog.Log(ctx, level, msg, args...)
}

// Slog 返回底层的 slog.Logger
func (l *Logger) Slog() *slog.Logger {
	return l.slog
}

// --- 包级别便捷函数 ---

// Debug 记录 Debug 级别日志
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info 记录 Info 级别日志
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn 记录 Warn 级别日志
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error 记录 Error 级别日志
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// DebugContext 记录带 context 的 Debug 级别日志
func DebugContext(ctx context.Context, msg string, args ...any) {
	Default().DebugContext(ctx, msg, args...)
}

// InfoContext 记录带 context 的 Info 级别日志
func InfoContext(ctx context.Context, msg string, args ...any) {
	Default().InfoContext(ctx, msg, args...)
}

// WarnContext 记录带 context 的 Warn 级别日志
func WarnContext(ctx context.Context, msg string, args ...any) {
	Default().WarnContext(ctx, msg, args...)
}

// ErrorContext 记录带 context 的 Error 级别日志
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Default().ErrorContext(ctx, msg, args...)
}

// With 创建带有固定字段的子记录器
func With(args ...any) *Logger {
	return Default().With(args...)
}

// WithGroup 创建带有分组的子记录器
func WithGroup(name string) *Logger {
	return Default().WithGroup(name)
}

// SetLevel 设置全局日志级别
func SetLevel(level string) {
	Default().SetLevel(level)
}
