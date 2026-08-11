package sse

import (
	"errors"
	"strings"
	"testing"
)

func TestCollectOpenAIStreamRequiresExplicitBudgets(t *testing.T) {
	t.Parallel()

	input := "data: {\"id\":1}\n\ndata: [DONE]\n\n"
	for _, config := range []CollectConfig{
		{},
		{MaxEvents: 1},
		{MaxTotalBytes: int64(len(input))},
		{MaxEvents: -1, MaxTotalBytes: int64(len(input))},
		{MaxEvents: 1, MaxTotalBytes: -1},
	} {
		items, err := CollectOpenAIStream[map[string]int](strings.NewReader(input), config)
		if items != nil || !errors.Is(err, ErrInvalidCollectionConfig) {
			t.Fatalf("CollectOpenAIStream(config=%+v) = (%v, %v), want (nil, ErrInvalidCollectionConfig)", config, items, err)
		}
	}
}

func TestCollectOpenAIStreamStopsAtEventBudget(t *testing.T) {
	t.Parallel()

	input := "data: {\"id\":1}\n\ndata: {\"id\":2}\n\ndata: {\"id\":3}\n\ndata: [DONE]\n\n"
	items, err := CollectOpenAIStream[map[string]int](strings.NewReader(input), CollectConfig{
		MaxEvents:     2,
		MaxTotalBytes: int64(len(input)),
	})
	if !errors.Is(err, ErrMaxEventsExceeded) {
		t.Fatalf("CollectOpenAIStream() error = %v, want ErrMaxEventsExceeded", err)
	}
	if items != nil {
		t.Fatalf("collected items = %v, want nil after an incomplete aggregation", items)
	}
}

func TestCollectOpenAIStreamStopsAtTotalByteBudget(t *testing.T) {
	t.Parallel()

	input := "data: {\"id\":1}\n\ndata: {\"id\":2}\n\ndata: [DONE]\n\n"
	items, err := CollectOpenAIStream[map[string]int](strings.NewReader(input), CollectConfig{
		MaxEvents:     10,
		MaxTotalBytes: 8,
	})
	if !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("CollectOpenAIStream() error = %v, want ErrMaxBytesExceeded", err)
	}
	if items != nil {
		t.Fatalf("collected items = %v, want nil after an incomplete aggregation", items)
	}
}

func TestCollectOpenAIStreamDiscardsPartialResultsAfterJSONError(t *testing.T) {
	t.Parallel()

	input := "data: {\"id\":1}\n\ndata: not-json\n\n"
	items, err := CollectOpenAIStream[map[string]int](strings.NewReader(input), CollectConfig{
		MaxEvents:     10,
		MaxTotalBytes: int64(len(input)),
	})
	if err == nil {
		t.Fatal("CollectOpenAIStream() error = nil, want JSON decoding error")
	}
	if items != nil {
		t.Fatalf("collected items = %v, want nil after an incomplete aggregation", items)
	}
}

func TestCollectOpenAIStreamAllowsDoneAfterExactEventBudget(t *testing.T) {
	t.Parallel()

	input := "data: {\"id\":1}\n\ndata: {\"id\":2}\n\ndata: [DONE]\n\n"
	items, err := CollectOpenAIStream[map[string]int](strings.NewReader(input), CollectConfig{
		MaxEvents:     2,
		MaxTotalBytes: int64(len(input)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("collected items = %d, want 2", len(items))
	}
}
