package multi

import (
	"context"
	"errors"
	"testing"
	"time"
)

type typedNilLayer struct{}

func (*typedNilLayer) GetOrLoad(context.Context, string, time.Duration, any, func(context.Context) (any, error)) error {
	return nil
}

func (*typedNilLayer) Del(context.Context, ...string) error { return nil }

type delErrorLayer struct {
	err error
}

func (*delErrorLayer) GetOrLoad(context.Context, string, time.Duration, any, func(context.Context) (any, error)) error {
	return nil
}

func (l *delErrorLayer) Del(context.Context, ...string) error { return l.err }

func TestNewCacheRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	var nilLayer *typedNilLayer
	tests := []struct {
		name   string
		layers []LayerConfig
		opts   []Option
		want   error
	}{
		{name: "no layers", want: ErrNoLayers},
		{name: "nil layer", layers: []LayerConfig{{Layer: nil, Name: "nil"}}, want: ErrNilLayer},
		{name: "typed nil layer", layers: []LayerConfig{{Layer: nilLayer, Name: "typed-nil"}}, want: ErrNilLayer},
		{name: "nil option", layers: []LayerConfig{{Layer: newMockLayer()}}, opts: []Option{nil}, want: ErrInvalidOption},
		{name: "nil not-found predicate", layers: []LayerConfig{{Layer: newMockLayer()}}, opts: []Option{WithIsNotFound(nil)}, want: ErrInvalidOption},
		{name: "zero backfill concurrency", layers: []LayerConfig{{Layer: newMockLayer()}}, opts: []Option{WithBackfillConcurrency(0)}, want: ErrInvalidOption},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache, err := NewCache(tt.layers, tt.opts...)
			if cache != nil {
				t.Fatalf("NewCache() cache = %#v, want nil", cache)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewCache() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBuilderBuildReturnsConfigurationError(t *testing.T) {
	t.Parallel()

	cache, err := NewBuilder().WithLayer(nil, time.Minute, "nil").Build()
	if cache != nil || !errors.Is(err, ErrNilLayer) {
		t.Fatalf("Build() = (%v, %v), want nil and ErrNilLayer", cache, err)
	}

	var builder *Builder
	cache, err = builder.Build()
	if cache != nil || !errors.Is(err, ErrNilBuilder) {
		t.Fatalf("nil Builder.Build() = (%v, %v), want nil and ErrNilBuilder", cache, err)
	}
}

func TestCacheOperationsRejectNilContext(t *testing.T) {
	t.Parallel()

	cache, err := NewCache([]LayerConfig{{Layer: newMockLayer(), Name: "memory"}})
	if err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	var destination string
	if err := cache.GetOrLoad(nilContext, "key", &destination, func(context.Context) (any, error) {
		return "value", nil
	}); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("GetOrLoad(nil) error = %v, want ErrInvalidContext", err)
	}
	if err := cache.Del(nilContext, "key"); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Del(nil) error = %v, want ErrInvalidContext", err)
	}
}

func TestCacheDelPreservesEveryLayerError(t *testing.T) {
	t.Parallel()

	firstErr := errors.New("first layer delete failed")
	secondErr := errors.New("second layer delete failed")
	cache, err := NewCache([]LayerConfig{
		{Layer: &delErrorLayer{err: firstErr}, Name: "first"},
		{Layer: &delErrorLayer{err: secondErr}, Name: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = cache.Del(context.Background(), "key")
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Del() error = %v, want both layer errors", err)
	}
}
