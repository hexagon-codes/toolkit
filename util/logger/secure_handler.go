package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
)

const redactedValue = "[REDACTED]"

type secureHandler struct {
	handler slog.Handler
}

type levelHandler struct {
	handler slog.Handler
	level   *slog.LevelVar
}

func newLevelHandler(handler slog.Handler, level *slog.LevelVar) slog.Handler {
	return &levelHandler{handler: handler, level: level}
}

func (h *levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level.Level() && h.handler.Enabled(ctx, level)
}

func (h *levelHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.handler.Handle(ctx, record)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newLevelHandler(h.handler.WithAttrs(attrs), h.level)
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return newLevelHandler(h.handler.WithGroup(name), h.level)
}

func newSecureHandler(handler slog.Handler) slog.Handler {
	if handler == nil {
		panic("logger: nil handler")
	}
	if _, ok := handler.(*secureHandler); ok {
		return handler
	}
	return &secureHandler{handler: handler}
}

func (h *secureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *secureHandler) Handle(ctx context.Context, record slog.Record) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = recoveredError("logger: handler panicked", recovered)
		}
	}()

	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(redactAttribute(attr))
		return true
	})
	return h.handler.Handle(ctx, clean)
}

func (h *secureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		clean[index] = redactAttribute(attr)
	}
	return newSecureHandler(h.handler.WithAttrs(clean))
}

func (h *secureHandler) WithGroup(name string) slog.Handler {
	return newSecureHandler(h.handler.WithGroup(name))
}

func redactAttribute(attr slog.Attr) slog.Attr {
	if isSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redactedValue)
	}

	value := attr.Value.Resolve()
	if value.Kind() != slog.KindGroup {
		return slog.Attr{Key: attr.Key, Value: value}
	}
	group := value.Group()
	clean := make([]slog.Attr, len(group))
	for index, nested := range group {
		clean[index] = redactAttribute(nested)
	}
	return slog.Attr{Key: attr.Key, Value: slog.GroupValue(clean...)}
}

func isSensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	if normalized == "" {
		return false
	}
	for _, suffix := range [...]string{
		"password",
		"passwd",
		"secret",
		"credential",
		"credentials",
		"apikey",
		"privatekey",
		"authorization",
		"token",
		"cookie",
		"sessionid",
	} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
		if strings.HasSuffix(normalized, suffix+"hash") {
			return true
		}
	}
	return false
}

func recoveredError(message string, recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s: %v", message, recovered)
}
