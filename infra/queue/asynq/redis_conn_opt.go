package asynq

import (
	"errors"
	"sync"

	queue "github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

// redisConnOpt keeps Asynq on the same canonical Redis factory as the rest of
// the process. Each Asynq component still receives its own go-redis client, as
// required by asynq.RedisConnOpt.
type redisConnOpt struct {
	factory *redisconn.Factory
	mu      sync.Mutex
	clients []redis.UniversalClient
	closed  bool
	sealed  redis.UniversalClient
}

func newRedisConnOpt(factory *redisconn.Factory) *redisConnOpt {
	return &redisConnOpt{factory: factory}
}

func (o *redisConnOpt) MakeRedisClient() interface{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return o.sealed
	}
	client := o.factory.NewClient()
	o.clients = append(o.clients, client)
	return client
}

func (o *redisConnOpt) mark() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.clients)
}

func (o *redisConnOpt) closeFrom(mark int) error {
	o.mu.Lock()
	if mark < 0 || mark > len(o.clients) {
		mark = len(o.clients)
	}
	clients := append([]redis.UniversalClient(nil), o.clients[mark:]...)
	o.clients = o.clients[:mark]
	o.mu.Unlock()

	var closeErr error
	for _, client := range clients {
		if err := client.Close(); err != nil && !errors.Is(err, redis.ErrClosed) {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

func (o *redisConnOpt) closeAll() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	// RedisConnOpt cannot return an error from MakeRedisClient. Keep one closed
	// lazy client so callers retaining the option after Stop cannot create a new
	// live pool; surface the one possible Close error through Manager.Stop.
	sealed := o.factory.NewClient()
	sealedErr := sealed.Close()
	o.closed = true
	o.sealed = sealed
	clients := append([]redis.UniversalClient(nil), o.clients...)
	o.clients = nil
	o.mu.Unlock()

	var closeErr error
	if sealedErr != nil && !errors.Is(sealedErr, redis.ErrClosed) {
		closeErr = errors.Join(closeErr, sealedErr)
	}
	for _, client := range clients {
		if err := client.Close(); err != nil && !errors.Is(err, redis.ErrClosed) {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

var _ queue.RedisConnOpt = (*redisConnOpt)(nil)
