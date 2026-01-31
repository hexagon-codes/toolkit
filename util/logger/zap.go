package logger

import (
	"context"
	"log/slog"
)

// ZapHandler 是一个示例接口，展示如何集成 zap
// 实际使用时，需要引入 zap 包：
//
//	import (
//	    "go.uber.org/zap"
//	    "go.uber.org/zap/exp/zapslog"
//	)
//
//	// 创建 zap logger
//	zapLogger, _ := zap.NewProduction()
//
//	// 转换为 slog handler
//	handler := zapslog.NewHandler(zapLogger.Core(), nil)
//
//	// 使用 zap 后端
//	logger.UseHandler(handler)

// UseHandler 使用自定义 Handler（可用于集成 zap）
func UseHandler(h slog.Handler) {
	logger := &Logger{
		slog:   slog.New(h),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}
	SetDefault(logger)
}

// UseHandlerWithConfig 使用自定义 Handler 和配置
func UseHandlerWithConfig(h slog.Handler, cfg *Config) {
	levelVar := &slog.LevelVar{}
	levelVar.Set(parseLevel(cfg.Level))

	logger := &Logger{
		slog:   slog.New(h),
		level:  levelVar,
		config: cfg,
	}
	SetDefault(logger)
}

// NewWithHandler 使用自定义 Handler 创建 Logger
func NewWithHandler(h slog.Handler) *Logger {
	return &Logger{
		slog:   slog.New(h),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}
}

// ContextHandler 支持从 context 中提取字段的 Handler 包装器
type ContextHandler struct {
	handler   slog.Handler
	extractor func(context.Context) []slog.Attr
}

// NewContextHandler 创建支持 context 提取的 Handler
func NewContextHandler(h slog.Handler, extractor func(context.Context) []slog.Attr) *ContextHandler {
	return &ContextHandler{
		handler:   h,
		extractor: extractor,
	}
}

// Enabled 实现 slog.Handler 接口
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle 实现 slog.Handler 接口
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.extractor != nil {
		attrs := h.extractor(ctx)
		for _, attr := range attrs {
			r.AddAttrs(attr)
		}
	}
	return h.handler.Handle(ctx, r)
}

// WithAttrs 实现 slog.Handler 接口
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{
		handler:   h.handler.WithAttrs(attrs),
		extractor: h.extractor,
	}
}

// WithGroup 实现 slog.Handler 接口
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{
		handler:   h.handler.WithGroup(name),
		extractor: h.extractor,
	}
}

// --- Context 相关工具 ---

type contextKey struct{}

// ContextWithLogger 将 Logger 存入 context
func ContextWithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext 从 context 获取 Logger
func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(contextKey{}).(*Logger); ok {
		return l
	}
	return Default()
}

// Ctx 从 context 获取 Logger 的简写
func Ctx(ctx context.Context) *Logger {
	return FromContext(ctx)
}
