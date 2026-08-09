package asynq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// PollingLockTTL is the default ownership window for task polling.
	PollingLockTTL = 8 * time.Minute
	// MigrationLockKey is the base key namespaced by Manager.QueuePrefix.
	MigrationLockKey = "asynq:migration_lock"
	// MigrationLockTTL is the migration lease window refreshed at one third.
	MigrationLockTTL = 2 * time.Minute
	redisOpTimeout   = 5 * time.Second
)

var (
	releaseLeaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0`)
	refreshLeaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0`)
)

// Lease is an ownership token for a Redis lock. Only the owner token may
// refresh or release the key, preventing a stale worker from touching a later
// owner's lock after TTL expiry.
type Lease struct {
	client redis.UniversalClient
	key    string
	token  string
	ttl    time.Duration
}

// AcquirePollingLease attempts to own a task's polling lease. A false result
// with nil error means another worker owns it; Redis failures return an error
// and never grant work.
func (m *Manager) AcquirePollingLease(ctx context.Context, taskID string) (*Lease, bool, error) {
	if taskID == "" {
		return nil, false, fmt.Errorf("%w: empty task ID", ErrInvalidLease)
	}
	return m.acquireLease(ctx, m.internalKey("polling_lock:"+taskID), PollingLockTTL)
}

// AcquireMigrationLease attempts to own the migration lease within this
// manager's queue-prefix namespace.
func (m *Manager) AcquireMigrationLease(ctx context.Context) (*Lease, bool, error) {
	return m.acquireMigrationLease(ctx, MigrationLockTTL)
}

func (m *Manager) acquireMigrationLease(ctx context.Context, ttl time.Duration) (*Lease, bool, error) {
	return m.acquireLease(ctx, m.internalKey(MigrationLockKey), ttl)
}

func (m *Manager) internalKey(base string) string {
	if m.config.QueuePrefix == "" {
		return base
	}
	return m.config.QueuePrefix + base
}

func (m *Manager) acquireLease(ctx context.Context, key string, ttl time.Duration) (*Lease, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("%w: nil context", ErrInvalidLease)
	}
	if key == "" || ttl <= 0 {
		return nil, false, fmt.Errorf("%w: key and positive TTL are required", ErrInvalidLease)
	}
	m.mu.RLock()
	client := m.redisClient
	closed := m.closed
	m.mu.RUnlock()
	if closed || client == nil {
		return nil, false, ErrRedisClientUnavailable
	}
	token, err := newLeaseToken()
	if err != nil {
		return nil, false, fmt.Errorf("asynq: generate lease token: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	acquired, err := client.SetNX(opCtx, key, token, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("asynq: acquire Redis lease %q: %w", key, err)
	}
	if !acquired {
		return nil, false, nil
	}
	return &Lease{client: client, key: key, token: token, ttl: ttl}, true, nil
}

// Refresh extends this lease only while its ownership token still matches.
func (l *Lease) Refresh(ctx context.Context) error {
	if l == nil || l.client == nil {
		return ErrRedisClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalidLease)
	}
	opCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	result, err := refreshLeaseScript.Run(
		opCtx,
		l.client,
		[]string{l.key},
		l.token,
		l.ttl.Milliseconds(),
	).Int64()
	if err != nil {
		return fmt.Errorf("asynq: refresh Redis lease %q: %w", l.key, err)
	}
	if result != 1 {
		return ErrLeaseLost
	}
	return nil
}

// Release deletes this lease only while its ownership token still matches.
func (l *Lease) Release(ctx context.Context) error {
	if l == nil || l.client == nil {
		return ErrRedisClientUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalidLease)
	}
	opCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	result, err := releaseLeaseScript.Run(
		opCtx,
		l.client,
		[]string{l.key},
		l.token,
	).Int64()
	if err != nil {
		return fmt.Errorf("asynq: release Redis lease %q: %w", l.key, err)
	}
	if result != 1 {
		return ErrLeaseLost
	}
	return nil
}

func newLeaseToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}
