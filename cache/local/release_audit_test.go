package local

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type typedNilCodec struct{}

func (c *typedNilCodec) Marshal(value any) ([]byte, error) {
	if c == nil {
		panic("typed nil codec used")
	}
	return json.Marshal(value)
}

func (c *typedNilCodec) Unmarshal(data []byte, destination any) error {
	if c == nil {
		panic("typed nil codec used")
	}
	return json.Unmarshal(data, destination)
}

func TestCacheExpiresAtExactDeadline(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	cache := NewCacheWithCleanup(8, -1, WithNow(func() time.Time { return now }), WithJitter(0))
	t.Cleanup(cache.Stop)

	cache.Set("key", "value", time.Minute)
	now = now.Add(time.Minute)

	if value, ok := cache.Get("key"); ok {
		t.Fatalf("Get() = (%v, true) at the expiration deadline, want a miss", value)
	}
}

func TestCacheClockRollbackDoesNotEvictMostRecentlyUsedEntry(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	now := base
	cache := NewCacheWithCleanup(2, -1, WithNow(func() time.Time { return now }), WithJitter(0))
	t.Cleanup(cache.Stop)

	cache.Set("a", "a", 0)
	cache.Set("b", "b", 0)
	now = base.Add(time.Minute)
	if _, ok := cache.Get("a"); !ok {
		t.Fatal("Get(a) missed before the clock rollback")
	}

	// 模拟宿主机时钟回拨；最近访问顺序不能因此倒退。
	now = base.Add(-time.Hour)
	if _, ok := cache.Get("a"); !ok {
		t.Fatal("Get(a) missed after the clock rollback")
	}
	cache.Set("c", "c", 0)

	if _, ok := cache.Get("a"); !ok {
		t.Fatal("the most recently used entry was evicted after a clock rollback")
	}
	if _, ok := cache.Get("b"); ok {
		t.Fatal("the least recently used entry survived eviction after a clock rollback")
	}
}

func TestCacheCanceledWaiterDoesNotBlockOnSharedLoad(t *testing.T) {
	cache := NewCacheNoCleanup(8, WithJitter(0))
	t.Cleanup(cache.Stop)

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

func TestCacheDeletePreventsInFlightLoadFromRepopulatingKey(t *testing.T) {
	cache := NewCacheNoCleanup(8, WithJitter(0))
	t.Cleanup(cache.Stop)

	started := make(chan struct{})
	release := make(chan struct{})
	loadDone := make(chan error, 1)
	go func() {
		var destination string
		loadDone <- cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
			close(started)
			<-release
			return "stale", nil
		})
	}()
	<-started

	if err := cache.Del(context.Background(), "key"); err != nil {
		t.Fatalf("Del() error = %v", err)
	}
	close(release)
	if err := <-loadDone; err != nil {
		t.Fatalf("in-flight GetOrLoad() error = %v", err)
	}

	loaderCalled := false
	var destination string
	if err := cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		loaderCalled = true
		return "fresh", nil
	}); err != nil {
		t.Fatalf("second GetOrLoad() error = %v", err)
	}
	if !loaderCalled || destination != "fresh" {
		t.Fatalf("second GetOrLoad() = (%q, loader=%v), want fresh data from loader", destination, loaderCalled)
	}
}

func TestCacheReloadsInvalidEnvelopeMarker(t *testing.T) {
	cache := NewCacheNoCleanup(8, WithJitter(0))
	t.Cleanup(cache.Stop)
	cache.setItemWithGen("key", []byte{2, '"', 'x', '"'}, time.Minute, cache.getGeneration(), false)

	var destination string
	loaderCalled := false
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

func TestCacheTreatsTypedNilCodecAsUnset(t *testing.T) {
	var codec *typedNilCodec
	cache := NewCacheNoCleanup(8, WithCodec(codec), WithJitter(0))
	t.Cleanup(cache.Stop)

	var destination string
	if err := cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		return "value", nil
	}); err != nil {
		t.Fatalf("GetOrLoad() error = %v", err)
	}
	if destination != "value" {
		t.Fatalf("GetOrLoad() destination = %q, want value", destination)
	}
}

func TestCacheRejectsNilLoaderValue(t *testing.T) {
	cache := NewCacheNoCleanup(8, WithJitter(0))
	t.Cleanup(cache.Stop)

	var destination string
	err := cache.GetOrLoad(context.Background(), "key", time.Minute, &destination, func(context.Context) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("GetOrLoad() accepted a nil loader value")
	}
}

func TestCacheReloadsCorruptSerializedPayload(t *testing.T) {
	cache := NewCacheNoCleanup(8, WithJitter(0))
	t.Cleanup(cache.Stop)
	cache.setItemWithGen("key", packFound([]byte("{")), time.Minute, cache.getGeneration(), false)

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

func TestCacheStopWaitsForCleanupLoop(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	cache := NewCacheWithCleanup(8, time.Millisecond, WithNow(func() time.Time {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return time.Now()
	}))
	<-entered

	stopped := make(chan struct{})
	go func() {
		cache.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		close(release)
		t.Fatal("Stop() returned while the cleanup loop was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop() did not return after the cleanup loop exited")
	}
}
