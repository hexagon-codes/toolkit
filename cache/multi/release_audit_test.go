package multi

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	localcache "github.com/hexagon-codes/toolkit/cache/local"
	rediscache "github.com/hexagon-codes/toolkit/cache/redis"
)

type auditMissLayer struct {
	probes atomic.Int32
}

func (l *auditMissLayer) GetOrLoad(ctx context.Context, _ string, _ time.Duration, _ any, loader func(context.Context) (any, error)) error {
	l.probes.Add(1)
	_, err := loader(ctx)
	return err
}

func (*auditMissLayer) Del(context.Context, ...string) error { return nil }

type auditErrorLayer struct {
	err error
}

func (l *auditErrorLayer) GetOrLoad(context.Context, string, time.Duration, any, func(context.Context) (any, error)) error {
	return l.err
}

func (*auditErrorLayer) Del(context.Context, ...string) error { return nil }

type blockingBackfillAuditLayer struct {
	mu      sync.Mutex
	data    map[string]any
	entered chan struct{}
	release chan struct{}
	done    chan struct{}
	deleted chan struct{}
	once    sync.Once
	delOnce sync.Once
}

func (l *blockingBackfillAuditLayer) GetOrLoad(ctx context.Context, key string, _ time.Duration, destination any, loader func(context.Context) (any, error)) error {
	l.mu.Lock()
	value, ok := l.data[key]
	l.mu.Unlock()
	if ok {
		return copyValue(value, destination)
	}

	value, err := loader(ctx)
	if err != nil {
		return err
	}
	l.once.Do(func() { close(l.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.release:
	}
	l.mu.Lock()
	l.data[key] = value
	l.mu.Unlock()
	close(l.done)
	return copyValue(value, destination)
}

func (l *blockingBackfillAuditLayer) Del(_ context.Context, keys ...string) error {
	l.delOnce.Do(func() { close(l.deleted) })
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		delete(l.data, key)
	}
	return nil
}

func (l *blockingBackfillAuditLayer) has(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.data[key]
	return ok
}

func TestCacheCoalescesConcurrentSourceLoads(t *testing.T) {
	layer := &auditMissLayer{}
	cache := mustNewCache(t, []LayerConfig{{Layer: layer, TTL: time.Minute, Name: "memory"}}, WithSkipBackfill(true))

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	call := func() <-chan error {
		done := make(chan error, 1)
		go func() {
			var destination string
			done <- cache.GetOrLoad(context.Background(), "key", &destination, func(context.Context) (any, error) {
				if calls.Add(1) == 1 {
					close(firstStarted)
				} else {
					close(secondStarted)
				}
				<-release
				return "value", nil
			})
		}()
		return done
	}

	firstDone := call()
	<-firstStarted
	secondDone := call()

	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	select {
	case <-secondStarted:
		close(release)
		<-firstDone
		<-secondDone
		t.Fatal("concurrent source loads for the same key were not coalesced")
	case <-deadline.C:
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first GetOrLoad() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second GetOrLoad() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestCacheDeleteWinsAgainstInFlightBackfill(t *testing.T) {
	layer := &blockingBackfillAuditLayer{
		data:    make(map[string]any),
		entered: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
		deleted: make(chan struct{}),
	}
	cache := mustNewCache(t, []LayerConfig{{Layer: layer, TTL: time.Minute, Name: "memory"}})

	var destination string
	if err := cache.GetOrLoad(context.Background(), "key", &destination, func(context.Context) (any, error) {
		return "stale", nil
	}); err != nil {
		t.Fatalf("GetOrLoad() error = %v", err)
	}
	<-layer.entered

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- cache.Del(context.Background(), "key") }()
	select {
	case <-layer.deleted:
		if err := <-deleteDone; err != nil {
			t.Fatalf("Del() error = %v", err)
		}
		close(layer.release)
	case <-time.After(time.Second):
		close(layer.release)
		if err := <-deleteDone; err != nil {
			t.Fatalf("Del() error = %v", err)
		}
	}
	<-layer.done
	if layer.has("key") {
		t.Fatal("an in-flight backfill repopulated the key after Del returned")
	}
}

func TestCacheRejectsNilLoaderValue(t *testing.T) {
	cache := mustNewCache(t, []LayerConfig{{Layer: &auditMissLayer{}, TTL: time.Minute, Name: "memory"}}, WithSkipBackfill(true))
	var destination string
	err := cache.GetOrLoad(context.Background(), "key", &destination, func(context.Context) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("GetOrLoad() accepted a nil loader value")
	}
}

func TestCacheCanceledWaiterDoesNotBlockOnSharedSourceLoad(t *testing.T) {
	cache := mustNewCache(t, []LayerConfig{{Layer: &auditMissLayer{}, TTL: time.Minute, Name: "memory"}}, WithSkipBackfill(true))
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })

	leaderDone := make(chan error, 1)
	go func() {
		var destination string
		leaderDone <- cache.GetOrLoad(context.Background(), "key", &destination, func(context.Context) (any, error) {
			close(started)
			<-release
			return "value", nil
		})
	}()
	<-started

	waiterContext, cancel := context.WithCancel(context.Background())
	waiterDone := make(chan error, 1)
	go func() {
		var destination string
		waiterDone <- cache.GetOrLoad(waiterContext, "key", &destination, func(context.Context) (any, error) {
			return "unexpected", nil
		})
	}()
	cancel()

	select {
	case err := <-waiterDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		once.Do(func() { close(release) })
		<-leaderDone
		<-waiterDone
		t.Fatal("canceled waiter remained blocked on the shared source load")
	}

	once.Do(func() { close(release) })
	if err := <-leaderDone; err != nil {
		t.Fatalf("leader error = %v", err)
	}
}

func TestCacheCloseCancelsAndWaitsForBackfill(t *testing.T) {
	layer := &blockingBackfillAuditLayer{
		data:    make(map[string]any),
		entered: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
		deleted: make(chan struct{}),
	}
	cache := mustNewCache(t, []LayerConfig{{Layer: layer, TTL: time.Minute, Name: "memory"}})

	var destination string
	if err := cache.GetOrLoad(context.Background(), "key", &destination, func(context.Context) (any, error) {
		return "value", nil
	}); err != nil {
		t.Fatalf("GetOrLoad() error = %v", err)
	}
	<-layer.entered

	closer, ok := any(cache).(interface{ Close() error })
	if !ok {
		close(layer.release)
		<-layer.done
		t.Fatal("Cache does not expose a Close method for its background backfills")
	}
	closed := make(chan error, 1)
	go func() { closed <- closer.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		close(layer.release)
		t.Fatal("Close() did not cancel and wait for the active backfill")
	}

	var afterClose string
	if err := cache.GetOrLoad(context.Background(), "after-close", &afterClose, func(context.Context) (any, error) {
		return "unexpected", nil
	}); err == nil {
		t.Fatal("GetOrLoad() succeeded after Close()")
	}
}

func TestCacheRecognizesLocalNegativeCacheByDefault(t *testing.T) {
	localLayer := localcache.NewCacheNoCleanup(8, localcache.WithJitter(0))
	t.Cleanup(localLayer.Stop)

	var seeded string
	if err := localLayer.GetOrLoad(context.Background(), "missing", time.Minute, &seeded, func(context.Context) (any, error) {
		return nil, localcache.ErrNotFound
	}); !errors.Is(err, localcache.ErrNotFound) {
		t.Fatalf("seed negative cache error = %v", err)
	}

	cache := mustNewCache(t, []LayerConfig{{Layer: localLayer, TTL: time.Minute, Name: "local"}}, WithSkipBackfill(true))
	loaderCalled := false
	var destination string
	err := cache.GetOrLoad(context.Background(), "missing", &destination, func(context.Context) (any, error) {
		loaderCalled = true
		return "unexpected", nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetOrLoad() error = %v, want ErrNotFound", err)
	}
	if loaderCalled {
		t.Fatal("the source loader ran despite a lower-layer negative cache hit")
	}
}

func TestCacheRecognizesRedisNegativeCacheByDefault(t *testing.T) {
	cache := mustNewCache(t, []LayerConfig{{
		Layer: &auditErrorLayer{err: rediscache.ErrNotFound},
		TTL:   time.Minute,
		Name:  "redis",
	}}, WithSkipBackfill(true))
	loaderCalled := false
	var destination string
	err := cache.GetOrLoad(context.Background(), "missing", &destination, func(context.Context) (any, error) {
		loaderCalled = true
		return "unexpected", nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetOrLoad() error = %v, want ErrNotFound", err)
	}
	if loaderCalled {
		t.Fatal("the source loader ran despite a Redis negative cache hit")
	}
}
