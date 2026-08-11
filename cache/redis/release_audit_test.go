package redis

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type typedNilRedisCodec struct{}

func (c *typedNilRedisCodec) Marshal(value any) ([]byte, error) {
	if c == nil {
		panic("typed nil codec used")
	}
	return json.Marshal(value)
}

func (c *typedNilRedisCodec) Unmarshal(data []byte, destination any) error {
	if c == nil {
		panic("typed nil codec used")
	}
	return json.Unmarshal(data, destination)
}

type blockingSetHook struct {
	entered       chan struct{}
	release       chan struct{}
	finished      chan struct{}
	deleteEntered chan struct{}
	once          sync.Once
	deleteOnce    sync.Once
}

func (h *blockingSetHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return next(ctx, network, address)
	}
}

func (h *blockingSetHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, command goredis.Cmder) error {
		if command.Name() == "del" {
			h.deleteOnce.Do(func() { close(h.deleteEntered) })
			return next(ctx, command)
		}
		if command.Name() != "set" {
			return next(ctx, command)
		}
		h.once.Do(func() { close(h.entered) })
		select {
		case <-ctx.Done():
			close(h.finished)
			return ctx.Err()
		case <-h.release:
		}
		err := next(ctx, command)
		close(h.finished)
		return err
	}
}

func (h *blockingSetHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, commands []goredis.Cmder) error {
		return next(ctx, commands)
	}
}

func TestUnpackRejectsUnknownAndMalformedMarkers(t *testing.T) {
	tests := [][]byte{{2, '"', 'x', '"'}, {0, 1}}
	for _, payload := range tests {
		if _, _, err := unpack(payload); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("unpack(%v) error = %v, want ErrCorrupt", payload, err)
		}
	}
}

func TestOptionsTreatTypedNilCodecAsUnset(t *testing.T) {
	var codec *typedNilRedisCodec
	options := ApplyOptions(WithCodec(codec))
	if _, err := options.Codec.Marshal("value"); err != nil {
		t.Fatalf("default codec Marshal() error = %v", err)
	}
}

func TestStableCacheRejectsNilContextWithoutPanicking(t *testing.T) {
	mr, client := setupRedis(t)
	t.Cleanup(mr.Close)
	t.Cleanup(func() { _ = client.Close() })
	cache := NewStableCache(client)

	var nilContext context.Context
	var destination string
	err := cache.GetOrLoad(nilContext, "key", time.Minute, &destination, func(context.Context) (any, error) {
		return "value", nil
	})
	if err == nil {
		t.Fatal("GetOrLoad() accepted a nil context")
	}
}

func TestRedisCachesRejectTypedNilClientsWithoutPanicking(t *testing.T) {
	var client *goredis.Client

	stable := NewStableCache(client)
	var destination string
	if err := stable.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		return "value", nil
	}); err == nil {
		t.Fatal("StableCache accepted a typed nil Redis client")
	}

	unstable := NewUnstableCache(client, "version")
	if err := unstable.GetOrLoadWithoutVersion(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		return "value", nil
	}); err == nil {
		t.Fatal("UnstableCache accepted a typed nil Redis client")
	}
}

func TestStableCacheCanceledWaiterDoesNotBlockOnSharedLoad(t *testing.T) {
	mr, client := setupRedis(t)
	t.Cleanup(mr.Close)
	t.Cleanup(func() { _ = client.Close() })
	cache := NewStableCache(client, WithJitter(0), WithRedisTimeout(100*time.Millisecond, 5*time.Second))

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	leaderDone := make(chan error, 1)
	go func() {
		var destination string
		leaderDone <- cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
			close(started)
			<-release
			return "value", nil
		})
	}()
	<-started

	waiterContext, cancelWaiter := context.WithCancel(context.Background())
	waiterDone := make(chan error, 1)
	go func() {
		var destination string
		waiterDone <- cache.GetOrLoad(waiterContext, "key", time.Minute, &destination, func(context.Context) (any, error) {
			return "unexpected", nil
		})
	}()
	cancelWaiter()

	select {
	case err := <-waiterDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		releaseOnce.Do(func() { close(release) })
		<-leaderDone
		<-waiterDone
		t.Fatal("canceled waiter remained blocked on the shared load")
	}

	releaseOnce.Do(func() { close(release) })
	if err := <-leaderDone; err != nil {
		t.Fatalf("leader error = %v", err)
	}
}

func TestStableCacheDeleteWinsAgainstInFlightWriteBack(t *testing.T) {
	mr, universalClient := setupRedis(t)
	t.Cleanup(mr.Close)
	client := universalClient.(*goredis.Client)
	t.Cleanup(func() { _ = client.Close() })

	hook := &blockingSetHook{
		entered:       make(chan struct{}),
		release:       make(chan struct{}),
		finished:      make(chan struct{}),
		deleteEntered: make(chan struct{}),
	}
	client.AddHook(hook)
	cache := NewStableCache(client, WithJitter(0), WithRedisTimeout(100*time.Millisecond, 5*time.Second))

	loadDone := make(chan error, 1)
	go func() {
		var destination string
		loadDone <- cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
			return "stale", nil
		})
	}()
	<-hook.entered

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- cache.Del(context.Background(), "key") }()
	select {
	case <-hook.deleteEntered:
		if err := <-deleteDone; err != nil {
			t.Fatalf("Del() error = %v", err)
		}
		close(hook.release)
	case <-time.After(time.Second):
		close(hook.release)
		if err := <-deleteDone; err != nil {
			t.Fatalf("Del() error = %v", err)
		}
	}
	if err := <-loadDone; err != nil {
		t.Fatalf("GetOrLoad() error = %v", err)
	}
	<-hook.finished

	if mr.Exists("key") {
		t.Fatal("an in-flight write-back repopulated the key after Del returned")
	}
}

func TestStableCacheRejectsNilLoaderValue(t *testing.T) {
	mr, client := setupRedis(t)
	t.Cleanup(mr.Close)
	t.Cleanup(func() { _ = client.Close() })
	cache := NewStableCache(client, WithJitter(0))

	var destination string
	err := cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("GetOrLoad() accepted a nil loader value")
	}
}

func TestStableCacheReloadsCorruptSerializedPayload(t *testing.T) {
	mr, client := setupRedis(t)
	t.Cleanup(mr.Close)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Set(context.Background(), "key", packFound([]byte("{")), time.Minute).Err(); err != nil {
		t.Fatalf("seed corrupt payload: %v", err)
	}
	cache := NewStableCache(client, WithJitter(0))

	loaderCalled := false
	var destination string
	err := cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		loaderCalled = true
		return "fresh", nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad() error = %v", err)
	}
	if !loaderCalled || destination != "fresh" {
		t.Fatalf("GetOrLoad() = (%q, loader=%v), want a fresh reload", destination, loaderCalled)
	}
}
