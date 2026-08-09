package sse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countedBlockingReadCloser struct {
	started    chan struct{}
	closed     chan struct{}
	startOnce  sync.Once
	closeOnce  sync.Once
	closeCount atomic.Int32
}

func newCountedBlockingReadCloser() *countedBlockingReadCloser {
	return &countedBlockingReadCloser{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (r *countedBlockingReadCloser) Read([]byte) (int, error) {
	r.startOnce.Do(func() { close(r.started) })
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *countedBlockingReadCloser) Close() error {
	r.closeCount.Add(1)
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestReaderCloseInterruptsBlockedReadAndClosesSourceOnce(t *testing.T) {
	source := newCountedBlockingReadCloser()
	reader := MustNewReader(source)
	readResult := make(chan error, 1)
	go func() {
		_, err := reader.Read()
		readResult <- err
	}()

	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("Reader did not begin reading")
	}

	closeResult := make(chan struct{}, 1)
	go func() {
		reader.Close()
		closeResult <- struct{}{}
	}()

	select {
	case <-closeResult:
	case <-time.After(200 * time.Millisecond):
		// 释放旧实现留下的阻塞协程，确保失败用例本身不泄漏资源。
		_ = source.Close()
		<-closeResult
		t.Fatal("Reader.Close() did not interrupt the blocked read")
	}

	if err := <-readResult; !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read() error = %v, want io.ErrClosedPipe in the error chain", err)
	}
	reader.Close()
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

type cancelObservedBody struct {
	reader     *strings.Reader
	closed     chan struct{}
	closeOnce  sync.Once
	closeCount atomic.Int32
}

func (b *cancelObservedBody) Read(buffer []byte) (int, error) {
	if b.reader != nil && b.reader.Len() > 0 {
		return b.reader.Read(buffer)
	}
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *cancelObservedBody) Close() error {
	b.closeCount.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

func TestStreamContextCancellationReleasesBackpressuredReadLoop(t *testing.T) {
	var payload strings.Builder
	for index := 0; index < 256; index++ {
		payload.WriteString("data: event\n\n")
	}
	body := &cancelObservedBody{
		reader: strings.NewReader(payload.String()),
		closed: make(chan struct{}),
	}
	client := mustClient(NewClient(
		"https://example.com/events",
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       body,
			}, nil
		})}),
	))
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for len(stream.events) < cap(stream.events) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(stream.events) != cap(stream.events) {
		_ = stream.Close()
		t.Fatal("stream did not reach the backpressure boundary")
	}

	cancel()
	select {
	case <-body.closed:
	case <-time.After(200 * time.Millisecond):
		_ = stream.Close()
		t.Fatal("context cancellation did not close the backpressured response body")
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if count := body.closeCount.Load(); count != 1 {
		t.Fatalf("response body Close() count = %d, want 1", count)
	}
}

type terminalReadCloser struct {
	reader     io.Reader
	readErr    error
	closeErr   error
	closeCount atomic.Int32
}

type diagnosticReadCloser struct {
	payload    []byte
	offset     int
	readErr    error
	closeErr   error
	closeCount atomic.Int32
}

func (b *diagnosticReadCloser) Read(buffer []byte) (int, error) {
	if b.offset >= len(b.payload) {
		return 0, b.readErr
	}
	count := copy(buffer, b.payload[b.offset:])
	b.offset += count
	if b.offset == len(b.payload) {
		return count, b.readErr
	}
	return count, nil
}

func (b *diagnosticReadCloser) Close() error {
	b.closeCount.Add(1)
	return b.closeErr
}

func (b *terminalReadCloser) Read(buffer []byte) (int, error) {
	if b.reader != nil {
		count, err := b.reader.Read(buffer)
		if count > 0 {
			return count, nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		b.reader = nil
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	return 0, io.EOF
}

func (b *terminalReadCloser) Close() error {
	b.closeCount.Add(1)
	return b.closeErr
}

func TestStreamClosesResponseBodyExactlyOnce(t *testing.T) {
	closeErr := errors.New("close failed")
	body := &terminalReadCloser{
		reader:   strings.NewReader("data: ok\n\n"),
		closeErr: closeErr,
	}
	stream := connectTestStream(context.Background(), t, http.StatusOK, "text/event-stream", body)
	for event := range stream.Events() {
		_ = event
	}

	if err := stream.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want close error", err)
	}
	if count := body.closeCount.Load(); count != 1 {
		t.Fatalf("response body Close() count = %d, want 1", count)
	}
}

func TestStreamJoinsReadAndCloseErrors(t *testing.T) {
	readErr := errors.New("read failed")
	closeErr := errors.New("close failed")
	body := &terminalReadCloser{readErr: readErr, closeErr: closeErr}
	stream := connectTestStream(context.Background(), t, http.StatusOK, "text/event-stream", body)

	for event := range stream.Events() {
		_ = event
	}
	var terminalErr error
	for err := range stream.Errors() {
		terminalErr = errors.Join(terminalErr, err)
	}
	if !errors.Is(terminalErr, readErr) || !errors.Is(terminalErr, closeErr) {
		t.Fatalf("terminal error = %v, want both read and close errors", terminalErr)
	}
	_ = stream.Close()
}

func TestClientNonOKResponseBoundsDiagnosticsAndPreservesErrors(t *testing.T) {
	const diagnosticLimit = 64 << 10
	readErr := errors.New("diagnostic read failed")
	closeErr := errors.New("diagnostic close failed")
	body := &diagnosticReadCloser{
		payload:  []byte(strings.Repeat("x", diagnosticLimit+1)),
		readErr:  readErr,
		closeErr: closeErr,
	}
	client := mustClient(NewClient(
		"https://example.com/events",
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Header:     make(http.Header),
				Body:       body,
			}, nil
		})}),
	))

	stream, err := client.Connect(context.Background())
	if stream != nil || err == nil {
		t.Fatalf("Connect() = (%v, %v), want non-nil error", stream, err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("Connect() error = %v, want *HTTPError", err)
	}
	if len(httpErr.Body) != diagnosticLimit || !httpErr.BodyTruncated {
		t.Fatalf("HTTP diagnostics = (%d bytes, truncated=%v), want (%d, true)", len(httpErr.Body), httpErr.BodyTruncated, diagnosticLimit)
	}
	if !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
		t.Fatalf("Connect() error = %v, want diagnostic read and close errors", err)
	}
	if count := body.closeCount.Load(); count != 1 {
		t.Fatalf("response body Close() count = %d, want 1", count)
	}
}

type wrappedEOFReader struct{}

func (wrappedEOFReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("wrapped end of stream: %w", io.EOF)
}

func TestIterationTreatsWrappedEOFAsNormalCompletion(t *testing.T) {
	reader := MustNewReader(wrappedEOFReader{})
	if err := reader.Each(func(*Event) error { return nil }); err != nil {
		t.Fatalf("Each() error = %v, want nil", err)
	}

	if err := ReadOpenAIStream[struct{}](wrappedEOFReader{}, func(struct{}) error { return nil }); err != nil {
		t.Fatalf("ReadOpenAIStream() error = %v, want nil", err)
	}
}

type fixedErrorReader struct {
	err error
}

func (r fixedErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestEOFMixedWithAnotherErrorIsNotDiscarded(t *testing.T) {
	sentinel := errors.New("terminal failure")
	joined := errors.Join(io.EOF, sentinel)

	reader := MustNewReader(fixedErrorReader{err: joined})
	if err := reader.Each(func(*Event) error { return nil }); !errors.Is(err, sentinel) {
		t.Fatalf("Each() error = %v, want terminal failure", err)
	}

	if err := ReadOpenAIStream[struct{}](fixedErrorReader{err: joined}, func(struct{}) error { return nil }); !errors.Is(err, sentinel) {
		t.Fatalf("ReadOpenAIStream() error = %v, want terminal failure", err)
	}

	body := &terminalReadCloser{readErr: joined}
	stream := connectTestStream(context.Background(), t, http.StatusOK, "text/event-stream", body)
	for event := range stream.Events() {
		_ = event
	}
	var streamErr error
	for err := range stream.Errors() {
		streamErr = errors.Join(streamErr, err)
	}
	if !errors.Is(streamErr, sentinel) {
		t.Fatalf("stream error = %v, want terminal failure", streamErr)
	}
}

func connectTestStream(
	ctx context.Context,
	t *testing.T,
	statusCode int,
	contentType string,
	body io.ReadCloser,
) *Stream {
	t.Helper()
	client := mustClient(NewClient(
		"https://example.com/events",
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: statusCode,
				Status:     fmt.Sprintf("%d test status", statusCode),
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       body,
			}, nil
		})}),
	))
	stream, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}
