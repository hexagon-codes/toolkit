package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != "info" {
		t.Errorf("expected level info, got %s", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("expected format json, got %s", cfg.Format)
	}
	if cfg.Output != "stdout" {
		t.Errorf("expected output stdout, got %s", cfg.Output)
	}
}

func TestNew(t *testing.T) {
	logger, err := New(nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if logger == nil {
		t.Fatal("logger is nil")
	}
}

func TestNewWithConfig(t *testing.T) {
	cfg := &Config{
		Level:     "debug",
		Format:    "text",
		Output:    "stdout",
		AddSource: true,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if logger == nil {
		t.Fatal("logger is nil")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		result := parseLevel(tt.input)
		if result != tt.expected {
			t.Errorf("parseLevel(%s) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestLoggerOutput(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("output should contain message")
	}
	if !strings.Contains(output, "key") {
		t.Error("output should contain key")
	}
	if !strings.Contains(output, "value") {
		t.Error("output should contain value")
	}
}

func TestLoggerJSONOutput(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	logger.Info("test", "count", 42, "enabled", true)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if logEntry["msg"] != "test" {
		t.Errorf("expected msg 'test', got %v", logEntry["msg"])
	}
	if logEntry["count"] != float64(42) {
		t.Errorf("expected count 42, got %v", logEntry["count"])
	}
	if logEntry["enabled"] != true {
		t.Errorf("expected enabled true, got %v", logEntry["enabled"])
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	tests := []struct {
		fn    func(string, ...any)
		level string
	}{
		{logger.Debug, "DEBUG"},
		{logger.Info, "INFO"},
		{logger.Warn, "WARN"},
		{logger.Error, "ERROR"},
	}

	for _, tt := range tests {
		buf.Reset()
		tt.fn("test message")

		var logEntry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse JSON for %s: %v", tt.level, err)
		}

		if !strings.Contains(logEntry["level"].(string), tt.level) {
			t.Errorf("expected level %s, got %v", tt.level, logEntry["level"])
		}
	}
}

func TestLoggerWith(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	childLogger := logger.With("service", "test-service")
	childLogger.Info("message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if logEntry["service"] != "test-service" {
		t.Errorf("expected service 'test-service', got %v", logEntry["service"])
	}
}

func TestLoggerWithGroup(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	childLogger := logger.WithGroup("request")
	childLogger.Info("message", "path", "/api/users")

	output := buf.String()
	if !strings.Contains(output, "request") {
		t.Error("output should contain group name")
	}
}

func TestLoggerSetLevel(t *testing.T) {
	var buf bytes.Buffer

	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelError)

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: levelVar,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  levelVar,
		config: DefaultConfig(),
	}

	// Info should not be logged
	logger.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("Info should not be logged at Error level")
	}

	// Change level to Info
	logger.SetLevel("info")

	logger.Info("should appear")
	if buf.Len() == 0 {
		t.Error("Info should be logged after level change")
	}
}

func TestLoggerContext(t *testing.T) {
	var buf bytes.Buffer

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	ctx := context.Background()
	logger.InfoContext(ctx, "context message", "key", "value")

	if !strings.Contains(buf.String(), "context message") {
		t.Error("output should contain message")
	}
}

func TestFileWriter(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: logPath,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	logger.Info("test file log", "key", "value")

	// Read file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test file log") {
		t.Error("log file should contain message")
	}
}

func TestAttrs(t *testing.T) {
	// Test attribute constructors
	tests := []struct {
		attr slog.Attr
		key  string
	}{
		{String("name", "test"), "name"},
		{Int("count", 42), "count"},
		{Int64("big", 9223372036854775807), "big"},
		{Float64("pi", 3.14), "pi"},
		{Bool("enabled", true), "enabled"},
		{Time("created", time.Now()), "created"},
		{Duration("latency", time.Second), "latency"},
		{Any("data", map[string]int{"a": 1}), "data"},
		{Err(nil), "error"},
		{TraceID("abc123"), "trace_id"},
		{UserID(123), "user_id"},
		{RequestID("req-123"), "request_id"},
		{Method("GET"), "method"},
		{Path("/api/users"), "path"},
		{Status(200), "status"},
		{Latency(100 * time.Millisecond), "latency"},
		{IP("192.168.1.1"), "ip"},
		{Component("api"), "component"},
		{Action("create"), "action"},
	}

	for _, tt := range tests {
		if tt.attr.Key != tt.key {
			t.Errorf("expected key %s, got %s", tt.key, tt.attr.Key)
		}
	}
}

func TestContextWithLogger(t *testing.T) {
	logger, _ := New(nil)
	ctx := context.Background()

	// Add logger to context
	ctx = ContextWithLogger(ctx, logger)

	// Get logger from context
	retrieved := FromContext(ctx)
	if retrieved != logger {
		t.Error("retrieved logger should match original")
	}

	// Test Ctx shorthand
	retrieved = Ctx(ctx)
	if retrieved != logger {
		t.Error("Ctx should return same logger")
	}
}

func TestFromContextDefault(t *testing.T) {
	ctx := context.Background()

	// Should return default logger when not set
	logger := FromContext(ctx)
	if logger == nil {
		t.Error("should return default logger")
	}
}

func TestContextHandler(t *testing.T) {
	var buf bytes.Buffer

	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create context handler that extracts trace_id
	handler := NewContextHandler(baseHandler, func(ctx context.Context) []slog.Attr {
		if traceID := ctx.Value("trace_id"); traceID != nil {
			return []slog.Attr{slog.String("trace_id", traceID.(string))}
		}
		return nil
	})

	logger := slog.New(handler)

	// Log with context containing trace_id
	ctx := context.WithValue(context.Background(), "trace_id", "abc123")
	logger.InfoContext(ctx, "test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if logEntry["trace_id"] != "abc123" {
		t.Errorf("expected trace_id 'abc123', got %v", logEntry["trace_id"])
	}
}

func TestMultiWriter(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	mw := NewMultiWriter(&buf1, &buf2)

	_, err := mw.Write([]byte("test"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf1.String() != "test" {
		t.Error("buf1 should contain 'test'")
	}
	if buf2.String() != "test" {
		t.Error("buf2 should contain 'test'")
	}
}

func TestMultiWriterAdd(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	mw := NewMultiWriter(&buf1)
	mw.Add(&buf2)

	_, err := mw.Write([]byte("test"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf1.String() != "test" || buf2.String() != "test" {
		t.Error("both buffers should contain 'test'")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	// Reset default logger
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	UseHandler(handler)

	// Test package-level functions
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Error("should contain debug message")
	}
	if !strings.Contains(output, "info message") {
		t.Error("should contain info message")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("should contain warn message")
	}
	if !strings.Contains(output, "error message") {
		t.Error("should contain error message")
	}
}

func TestUseHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)

	UseHandler(handler)

	Info("test message")

	if !strings.Contains(buf.String(), "test message") {
		t.Error("should contain message")
	}
}

func TestInit(t *testing.T) {
	// Test with nil config
	err := Init(nil)
	if err != nil {
		t.Errorf("Init(nil) should not error: %v", err)
	}

	// Test with custom config
	err = Init(&Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		t.Errorf("Init with config should not error: %v", err)
	}
}

func TestStderrOutput(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: "stderr",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New with stderr output failed: %v", err)
	}
	if logger == nil {
		t.Fatal("logger is nil")
	}
}

func TestLoggerSlog(t *testing.T) {
	logger, _ := New(nil)
	slogger := logger.Slog()

	if slogger == nil {
		t.Error("Slog() should return non-nil slog.Logger")
	}
}

func TestLoggerLog(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	ctx := context.Background()
	logger.Log(ctx, slog.LevelInfo, "test log message", "key", "value")

	if !strings.Contains(buf.String(), "test log message") {
		t.Error("output should contain message")
	}
}

func TestLoggerContextMethods(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  &slog.LevelVar{},
		config: DefaultConfig(),
	}

	ctx := context.Background()

	// Test all context methods
	logger.DebugContext(ctx, "debug context")
	if !strings.Contains(buf.String(), "debug context") {
		t.Error("should contain debug context message")
	}

	buf.Reset()
	logger.WarnContext(ctx, "warn context")
	if !strings.Contains(buf.String(), "warn context") {
		t.Error("should contain warn context message")
	}

	buf.Reset()
	logger.ErrorContext(ctx, "error context")
	if !strings.Contains(buf.String(), "error context") {
		t.Error("should contain error context message")
	}
}

func TestPackageLevelContextFunctions(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	UseHandler(handler)

	ctx := context.Background()

	DebugContext(ctx, "pkg debug")
	if !strings.Contains(buf.String(), "pkg debug") {
		t.Error("should contain pkg debug message")
	}

	buf.Reset()
	InfoContext(ctx, "pkg info")
	if !strings.Contains(buf.String(), "pkg info") {
		t.Error("should contain pkg info message")
	}

	buf.Reset()
	WarnContext(ctx, "pkg warn")
	if !strings.Contains(buf.String(), "pkg warn") {
		t.Error("should contain pkg warn message")
	}

	buf.Reset()
	ErrorContext(ctx, "pkg error")
	if !strings.Contains(buf.String(), "pkg error") {
		t.Error("should contain pkg error message")
	}
}

func TestPackageLevelWith(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	UseHandler(handler)

	// Test With
	childLogger := With("service", "test")
	if childLogger == nil {
		t.Fatal("With() should return non-nil logger")
	}

	childLogger.Info("with message")
	if !strings.Contains(buf.String(), "service") {
		t.Error("should contain service field")
	}
}

func TestPackageLevelWithGroup(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	UseHandler(handler)

	// Test WithGroup
	childLogger := WithGroup("mygroup")
	if childLogger == nil {
		t.Fatal("WithGroup() should return non-nil logger")
	}

	childLogger.Info("group message", "field", "value")
	if !strings.Contains(buf.String(), "mygroup") {
		t.Error("should contain group name")
	}
}

func TestPackageLevelSetLevel(t *testing.T) {
	var buf bytes.Buffer
	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelError)

	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: levelVar,
	})

	logger := &Logger{
		slog:   slog.New(handler),
		level:  levelVar,
		config: DefaultConfig(),
	}
	SetDefault(logger)

	// Info should not be logged at Error level
	Info("should not appear")
	if buf.Len() > 0 {
		t.Error("Info should not be logged at Error level")
	}

	// Change level using package-level function
	SetLevel("info")

	Info("should appear now")
	if buf.Len() == 0 {
		t.Error("Info should be logged after SetLevel")
	}
}

func TestUseHandlerWithConfig(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	cfg := &Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	UseHandlerWithConfig(handler, cfg)

	Debug("debug message")
	if !strings.Contains(buf.String(), "debug message") {
		t.Error("should contain debug message")
	}
}

func TestNewWithHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := NewWithHandler(handler)
	if logger == nil {
		t.Fatal("NewWithHandler should return non-nil logger")
	}

	logger.Info("handler message")
	if !strings.Contains(buf.String(), "handler message") {
		t.Error("should contain message")
	}
}

func TestContextHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	handler := NewContextHandler(baseHandler, nil)
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{slog.String("fixed", "attr")})

	logger := slog.New(handlerWithAttrs)
	logger.Info("test message")

	if !strings.Contains(buf.String(), "fixed") {
		t.Error("should contain fixed attr")
	}
}

func TestContextHandlerWithGroup(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	handler := NewContextHandler(baseHandler, nil)
	handlerWithGroup := handler.WithGroup("mygroup")

	logger := slog.New(handlerWithGroup)
	logger.Info("test message", "key", "value")

	if !strings.Contains(buf.String(), "mygroup") {
		t.Error("should contain group name")
	}
}

func TestContextHandlerEnabled(t *testing.T) {
	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.LevelWarn)

	baseHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelVar,
	})

	handler := NewContextHandler(baseHandler, nil)

	ctx := context.Background()

	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Debug should not be enabled at Warn level")
	}
	if !handler.Enabled(ctx, slog.LevelError) {
		t.Error("Error should be enabled at Warn level")
	}
}

func TestFileWriterClose(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_close.log")

	writer, err := newFileWriter(logPath, nil)
	if err != nil {
		t.Fatalf("newFileWriter failed: %v", err)
	}

	fw, ok := writer.(*fileWriter)
	if !ok {
		t.Fatal("expected *fileWriter type")
	}

	// Write something
	_, err = fw.Write([]byte("test data"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Sync
	err = fw.Sync()
	if err != nil {
		t.Errorf("Sync failed: %v", err)
	}

	// Close
	err = fw.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestFileWriterCloseNil(t *testing.T) {
	fw := &fileWriter{file: nil}

	err := fw.Close()
	if err != nil {
		t.Errorf("Close on nil file should not error: %v", err)
	}

	err = fw.Sync()
	if err != nil {
		t.Errorf("Sync on nil file should not error: %v", err)
	}
}

func TestFileWriterDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "subdir", "nested", "test.log")

	writer, err := newFileWriter(logPath, nil)
	if err != nil {
		t.Fatalf("newFileWriter should create directories: %v", err)
	}

	_, err = writer.Write([]byte("nested log"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
}

func TestErrAttr(t *testing.T) {
	err := Err(errors.New("test error"))
	if err.Key != "error" {
		t.Errorf("expected key 'error', got '%s'", err.Key)
	}
}
