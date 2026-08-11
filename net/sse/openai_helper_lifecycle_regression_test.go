package sse

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type helperReadCloser struct {
	reader     io.Reader
	closeErr   error
	closeCount atomic.Int32
}

func (source *helperReadCloser) Read(buffer []byte) (int, error) {
	return source.reader.Read(buffer)
}

func (source *helperReadCloser) Close() error {
	source.closeCount.Add(1)
	return source.closeErr
}

func TestReadOpenAIStreamOwnsAndClosesSourceExactlyOnce(t *testing.T) {
	t.Parallel()

	source := &helperReadCloser{reader: strings.NewReader("data: {\"id\":1}\n\ndata: [DONE]\n\n")}
	if err := ReadOpenAIStream[map[string]int](source, func(map[string]int) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

func TestReadOpenAIStreamJoinsHandlerAndCloseErrors(t *testing.T) {
	t.Parallel()

	handlerErr := errors.New("handler failed")
	closeErr := errors.New("close failed")
	source := &helperReadCloser{
		reader:   strings.NewReader("data: {\"id\":1}\n\n"),
		closeErr: closeErr,
	}
	err := ReadOpenAIStream[map[string]int](source, func(map[string]int) error { return handlerErr })
	if !errors.Is(err, handlerErr) || !errors.Is(err, closeErr) {
		t.Fatalf("ReadOpenAIStream() error = %v, want handler and close errors", err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

func TestReadOpenAIStreamClosesSourceWhenHandlerIsInvalid(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	source := &helperReadCloser{reader: strings.NewReader(""), closeErr: closeErr}
	err := ReadOpenAIStream[struct{}](source, nil)
	if !errors.Is(err, ErrInvalidHandler) || !errors.Is(err, closeErr) {
		t.Fatalf("ReadOpenAIStream() error = %v, want invalid handler and close errors", err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

func TestReadOpenAIStreamJoinsJSONAndCloseErrors(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	source := &helperReadCloser{
		reader:   strings.NewReader("data: not-json\n\n"),
		closeErr: closeErr,
	}
	err := ReadOpenAIStream[map[string]int](source, func(map[string]int) error { return nil })
	if err == nil || !errors.Is(err, closeErr) {
		t.Fatalf("ReadOpenAIStream() error = %v, want JSON and close errors", err)
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("ReadOpenAIStream() error = %v, want JSON syntax error", err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

func TestCollectOpenAIStreamJoinsBudgetAndCloseErrorsWithoutPartialResults(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	input := "data: {\"id\":1}\n\ndata: {\"id\":2}\n\n"
	source := &helperReadCloser{reader: strings.NewReader(input), closeErr: closeErr}
	items, err := CollectOpenAIStream[map[string]int](source, CollectConfig{
		MaxEvents:     1,
		MaxTotalBytes: int64(len(input)),
	})
	if items != nil || !errors.Is(err, ErrMaxEventsExceeded) || !errors.Is(err, closeErr) {
		t.Fatalf("CollectOpenAIStream() = (%v, %v), want nil with budget and close errors", items, err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

func TestCollectOpenAIStreamClosesSourceWhenConfigIsInvalid(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	source := &helperReadCloser{reader: strings.NewReader(""), closeErr: closeErr}
	items, err := CollectOpenAIStream[struct{}](source, CollectConfig{})
	if items != nil || !errors.Is(err, ErrInvalidCollectionConfig) || !errors.Is(err, closeErr) {
		t.Fatalf("CollectOpenAIStream() = (%v, %v), want nil with config and close errors", items, err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}

func TestCollectOpenAIStreamReturnsNoResultsWhenCloseFails(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	input := "data: {\"id\":1}\n\ndata: [DONE]\n\n"
	source := &helperReadCloser{reader: strings.NewReader(input), closeErr: closeErr}
	items, err := CollectOpenAIStream[map[string]int](source, CollectConfig{
		MaxEvents:     1,
		MaxTotalBytes: int64(len(input)),
	})
	if items != nil || !errors.Is(err, closeErr) {
		t.Fatalf("CollectOpenAIStream() = (%v, %v), want nil with close error", items, err)
	}
	if count := source.closeCount.Load(); count != 1 {
		t.Fatalf("source Close() count = %d, want 1", count)
	}
}
