package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
)

type auditPanicWriter struct {
	err error
}

func (w auditPanicWriter) Write([]byte) (int, error) {
	panic(w.err)
}

type auditShortWriter struct {
	writes int
}

func (w *auditShortWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p) - 1, nil
}

type auditCountingWriter struct {
	writes int
}

func (w *auditCountingWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func TestLoggerRedactsSensitiveAttributes(t *testing.T) {
	const wantRedacted = "[REDACTED]"
	var output bytes.Buffer
	log := NewWithHandler(slog.NewJSONHandler(&output, nil)).With(
		"api_key", "fixed-secret",
	)
	log.Info(
		"request",
		"password", "request-secret",
		"token_count", 3,
		"secretary", "Alice",
		slog.Group("auth", slog.String("access_token", "nested-secret")),
	)

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if got := entry["api_key"]; got != wantRedacted {
		t.Errorf("api_key = %v, want %q", got, wantRedacted)
	}
	if got := entry["password"]; got != wantRedacted {
		t.Errorf("password = %v, want %q", got, wantRedacted)
	}
	if got := entry["token_count"]; got != float64(3) {
		t.Errorf("token_count = %v, want 3", got)
	}
	if got := entry["secretary"]; got != "Alice" {
		t.Errorf("secretary = %v, want Alice", got)
	}
	auth, ok := entry["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth = %T, want object", entry["auth"])
	}
	if got := auth["access_token"]; got != wantRedacted {
		t.Errorf("access_token = %v, want %q", got, wantRedacted)
	}
	for _, secret := range []string{"fixed-secret", "request-secret", "nested-secret"} {
		if bytes.Contains(output.Bytes(), []byte(secret)) {
			t.Errorf("log output contains sensitive value %q", secret)
		}
	}
}

func TestContextHandlerRedactsExtractedSensitiveAttributes(t *testing.T) {
	var output bytes.Buffer
	handler := NewContextHandler(
		slog.NewJSONHandler(&output, nil),
		func(_ context.Context) []slog.Attr {
			return []slog.Attr{slog.String("authorization", "Bearer secret")}
		},
	)
	slog.New(handler).Info("request")

	if bytes.Contains(output.Bytes(), []byte("Bearer secret")) {
		t.Fatal("context handler exposed an authorization value")
	}
}

func TestLoggerContainsHandlerWriterPanic(t *testing.T) {
	panicErr := errors.New("writer exploded")
	log := NewWithHandler(slog.NewJSONHandler(auditPanicWriter{err: panicErr}, nil))
	log.Info("message")
}

func TestNewWithHandlerSetLevelControlsOutput(t *testing.T) {
	var output bytes.Buffer
	log := NewWithHandler(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	log.SetLevel("error")
	log.Info("must be filtered")
	if output.Len() != 0 {
		t.Fatalf("info output was not filtered: %s", output.String())
	}
	log.Error("must be emitted")
	if !bytes.Contains(output.Bytes(), []byte("must be emitted")) {
		t.Fatalf("error output was filtered: %s", output.String())
	}
}

func TestUseHandlerSetLevelControlsOutput(t *testing.T) {
	previousLogger := defaultLoggerPtr.Load()
	previousSlog := slog.Default()
	t.Cleanup(func() {
		defaultLoggerPtr.Store(previousLogger)
		slog.SetDefault(previousSlog)
	})

	var output bytes.Buffer
	UseHandler(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	SetLevel("error")
	Info("must be filtered")
	if output.Len() != 0 {
		t.Fatalf("info output was not filtered: %s", output.String())
	}
	Error("must be emitted")
	if !bytes.Contains(output.Bytes(), []byte("must be emitted")) {
		t.Fatalf("error output was filtered: %s", output.String())
	}
}

func TestUseHandlerWithConfigSetLevelControlsOutput(t *testing.T) {
	previousLogger := defaultLoggerPtr.Load()
	previousSlog := slog.Default()
	t.Cleanup(func() {
		defaultLoggerPtr.Store(previousLogger)
		slog.SetDefault(previousSlog)
	})

	var output bytes.Buffer
	UseHandlerWithConfig(
		slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}),
		&Config{Level: "debug", Format: "json", Output: "stdout"},
	)
	SetLevel("error")
	Info("must be filtered")
	if output.Len() != 0 {
		t.Fatalf("info output was not filtered: %s", output.String())
	}
	Error("must be emitted")
	if !bytes.Contains(output.Bytes(), []byte("must be emitted")) {
		t.Fatalf("error output was filtered: %s", output.String())
	}
}

func TestMultiWriterConvertsWriterPanicToError(t *testing.T) {
	panicErr := errors.New("writer exploded")
	writer := NewMultiWriter(auditPanicWriter{err: panicErr})

	n, err := writer.Write([]byte("payload"))
	if n != 0 {
		t.Errorf("Write() n = %d, want 0", n)
	}
	if !errors.Is(err, panicErr) {
		t.Fatalf("Write() error = %v, want panic error in chain", err)
	}
}

func TestMultiWriterStopsOnShortWrite(t *testing.T) {
	short := &auditShortWriter{}
	trailing := &auditCountingWriter{}
	writer := NewMultiWriter(short, trailing)

	n, err := writer.Write([]byte("payload"))
	if n != len("payload")-1 {
		t.Errorf("Write() n = %d, want %d", n, len("payload")-1)
	}
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() error = %v, want io.ErrShortWrite", err)
	}
	if trailing.writes != 0 {
		t.Fatalf("trailing writer calls = %d, want 0", trailing.writes)
	}
}

func TestMultiWriterOwnsItsWriterList(t *testing.T) {
	var original bytes.Buffer
	var replacement bytes.Buffer
	writers := []io.Writer{&original}
	multi := NewMultiWriter(writers...)
	writers[0] = &replacement

	if _, err := multi.Write([]byte("payload")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := original.String(); got != "payload" {
		t.Errorf("original writer output = %q, want payload", got)
	}
	if got := replacement.String(); got != "" {
		t.Errorf("replacement writer output = %q, want empty", got)
	}
}

func TestMultiWriterConcurrentAddAndWrite(t *testing.T) {
	multi := NewMultiWriter(io.Discard)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		<-start
		for range 200 {
			multi.Add(io.Discard)
		}
	}()
	for range 8 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			for range 200 {
				if _, err := multi.Write([]byte("payload")); err != nil {
					t.Errorf("Write() error = %v", err)
					return
				}
			}
		}()
	}
	close(start)
	waitGroup.Wait()
}

func TestFileWriterCloseIsIdempotent(t *testing.T) {
	writer, err := newFileWriter(filepath.Join(t.TempDir(), "audit.log"), nil)
	if err != nil {
		t.Fatalf("newFileWriter() error = %v", err)
	}
	closer := writer.(io.Closer)
	if err := closer.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
