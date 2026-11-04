package redis

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// errorGetClient 只覆盖缓存读取，用于确定性地模拟 Redis 故障。
type errorGetClient struct {
	redis.UniversalClient
}

func (*errorGetClient) Get(context.Context, string) *redis.StringCmd {
	return redis.NewStringResult("", errors.New("redis unavailable"))
}

func TestStableCacheRedisErrorKeepsSingleflight(t *testing.T) {
	cache := NewStableCache(&errorGetClient{})

	var loads atomic.Int32
	const workers = 8
	var wait sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var user User
			err := cache.GetOrLoad(context.Background(), "review:redis-down", time.Minute, &user, func(context.Context) (any, error) {
				loads.Add(1)
				time.Sleep(100 * time.Millisecond)
				return User{ID: 1, Name: "Alice"}, nil
			})
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("GetOrLoad() error = %v", err)
		}
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}

func TestUnstableCacheRedisErrorKeepsSingleflight(t *testing.T) {
	cache := NewUnstableCache(&errorGetClient{}, "review:version")

	var loads atomic.Int32
	const workers = 8
	var wait sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var value string
			err := cache.GetOrLoad(context.Background(), "review:redis-down", time.Minute, &value, func(context.Context) (any, error) {
				loads.Add(1)
				time.Sleep(100 * time.Millisecond)
				return "ok", nil
			})
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("GetOrLoad() error = %v", err)
		}
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
}
