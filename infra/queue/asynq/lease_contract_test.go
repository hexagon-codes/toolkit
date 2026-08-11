package asynq

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestAcquirePollingLeaseFailsClosedWithoutRedisClient(t *testing.T) {
	manager := &Manager{}
	lease, acquired, err := manager.AcquirePollingLease(context.Background(), "task-123")
	if !errors.Is(err, ErrRedisClientUnavailable) {
		t.Fatalf("AcquirePollingLease() error = %v, want ErrRedisClientUnavailable", err)
	}
	if acquired || lease != nil {
		t.Fatalf("AcquirePollingLease() = (%v, %v), want (nil, false) on infrastructure failure", lease, acquired)
	}
}

func TestAcquirePollingLeaseFailsClosedOnRedisError(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	redisServer.Close()

	lease, acquired, err := manager.AcquirePollingLease(context.Background(), "task-123")
	if err == nil {
		t.Fatal("AcquirePollingLease() error = nil, want Redis connection error")
	}
	if acquired || lease != nil {
		t.Fatalf("AcquirePollingLease() = (%v, %v), want (nil, false) on Redis error", lease, acquired)
	}
}

func TestLeaseOperationsRespectManagerStopBoundary(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	lease, acquired, err := manager.AcquirePollingLease(context.Background(), "stopped-manager-task")
	if err != nil || !acquired || lease == nil {
		t.Fatalf("AcquirePollingLease() = (%v, %v, %v), want lease, true, nil", lease, acquired, err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if _, _, err := manager.AcquirePollingLease(context.Background(), "new-task"); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("AcquirePollingLease() after Stop error = %v, want ErrManagerStopped", err)
	}
	if err := lease.Refresh(context.Background()); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("lease Refresh() after Stop error = %v, want ErrManagerStopped", err)
	}
	if err := lease.Release(context.Background()); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("lease Release() after Stop error = %v, want ErrManagerStopped", err)
	}
}

func TestLeaseOperationsRejectNilContext(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if _, _, acquireErr := manager.AcquirePollingLease(nil, "nil-context-task"); !errors.Is(acquireErr, ErrInvalidContext) { //nolint:staticcheck // 专门验证 nil context 防护。
		t.Fatalf("AcquirePollingLease(nil) error = %v, want ErrInvalidContext", acquireErr)
	}
	lease, acquired, err := manager.AcquirePollingLease(context.Background(), "lease-nil-context-task")
	if err != nil || !acquired || lease == nil {
		t.Fatalf("AcquirePollingLease() = (%v, %v, %v), want lease, true, nil", lease, acquired, err)
	}
	if err := lease.Refresh(nil); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // 专门验证 nil context 防护。
		t.Fatalf("lease Refresh(nil) error = %v, want ErrInvalidContext", err)
	}
	if err := lease.Release(nil); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // 专门验证 nil context 防护。
		t.Fatalf("lease Release(nil) error = %v, want ErrInvalidContext", err)
	}
}

func TestPollingLeaseUsesTokenForRefreshAndRelease(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	lease, acquired, err := manager.AcquirePollingLease(context.Background(), "task-123")
	if err != nil || !acquired || lease == nil {
		t.Fatalf("AcquirePollingLease() = (%v, %v, %v), want lease, true, nil", lease, acquired, err)
	}
	competingLease, competingAcquired, err := manager.AcquirePollingLease(context.Background(), "task-123")
	if err != nil {
		t.Fatalf("competing AcquirePollingLease() error = %v", err)
	}
	if competingAcquired || competingLease != nil {
		t.Fatalf("competing AcquirePollingLease() = (%v, %v), want nil, false", competingLease, competingAcquired)
	}

	if setErr := manager.redisClient.Set(context.Background(), lease.key, "replacement-owner", time.Minute).Err(); setErr != nil {
		t.Fatalf("replace lease owner error = %v", setErr)
	}
	if refreshErr := lease.Refresh(context.Background()); !errors.Is(refreshErr, ErrLeaseLost) {
		t.Fatalf("stale lease Refresh() error = %v, want ErrLeaseLost", refreshErr)
	}
	if releaseErr := lease.Release(context.Background()); !errors.Is(releaseErr, ErrLeaseLost) {
		t.Fatalf("stale lease Release() error = %v, want ErrLeaseLost", releaseErr)
	}
	value, err := manager.redisClient.Get(context.Background(), lease.key).Result()
	if err != nil {
		t.Fatalf("read replacement owner error = %v", err)
	}
	if value != "replacement-owner" {
		t.Fatalf("replacement owner = %q, want preserved replacement-owner", value)
	}
}

func TestMigrationLeaseUsesSharedPrimitive(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	lease, acquired, err := manager.AcquireMigrationLease(context.Background())
	if err != nil || !acquired || lease == nil {
		t.Fatalf("AcquireMigrationLease() = (%v, %v, %v), want lease, true, nil", lease, acquired, err)
	}
	if lease.key != MigrationLockKey {
		t.Fatalf("migration lease key = %q, want %q", lease.key, MigrationLockKey)
	}
	if err := lease.Refresh(context.Background()); err != nil {
		t.Fatalf("migration lease Refresh() error = %v", err)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("migration lease Release() error = %v", err)
	}
}

func TestLeaseKeysAreIsolatedByManagerQueuePrefix(t *testing.T) {
	redisServer := miniredis.RunT(t)
	newManager := func(t *testing.T, prefix string) *Manager {
		t.Helper()
		config := managerTestConfig(redisServer.Addr())
		config.QueuePrefix = prefix
		manager, err := NewManager(context.Background(), config)
		if err != nil {
			t.Fatalf("NewManager(%q) error = %v", prefix, err)
		}
		t.Cleanup(func() { _ = manager.Stop() })
		return manager
	}
	tenantA := newManager(t, "tenant-a:")
	tenantB := newManager(t, "tenant-b:")
	tenantAReplica := newManager(t, "tenant-a:")

	pollingA, acquired, err := tenantA.AcquirePollingLease(context.Background(), "task-123")
	if err != nil || !acquired {
		t.Fatalf("tenant A polling lease = (%v, %v), want acquired", acquired, err)
	}
	pollingB, acquired, err := tenantB.AcquirePollingLease(context.Background(), "task-123")
	if err != nil || !acquired {
		t.Fatalf("tenant B polling lease = (%v, %v), want independently acquired", acquired, err)
	}
	competingPolling, replicaAcquired, acquireErr := tenantAReplica.AcquirePollingLease(context.Background(), "task-123")
	if acquireErr != nil || replicaAcquired || competingPolling != nil {
		t.Fatalf("same-prefix polling competitor = (%v, %v, %v), want nil, false, nil", competingPolling, replicaAcquired, acquireErr)
	}
	if pollingA.key == pollingB.key {
		t.Fatalf("different-prefix polling keys are both %q", pollingA.key)
	}

	migrationA, acquired, err := tenantA.AcquireMigrationLease(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tenant A migration lease = (%v, %v), want acquired", acquired, err)
	}
	migrationB, acquired, err := tenantB.AcquireMigrationLease(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tenant B migration lease = (%v, %v), want independently acquired", acquired, err)
	}
	competingMigration, replicaAcquired, acquireErr := tenantAReplica.AcquireMigrationLease(context.Background())
	if acquireErr != nil || replicaAcquired || competingMigration != nil {
		t.Fatalf("same-prefix migration competitor = (%v, %v, %v), want nil, false, nil", competingMigration, replicaAcquired, acquireErr)
	}
	if migrationA.key == migrationB.key {
		t.Fatalf("different-prefix migration keys are both %q", migrationA.key)
	}

	for _, lease := range []*Lease{pollingA, pollingB, migrationA, migrationB} {
		if err := lease.Release(context.Background()); err != nil {
			t.Errorf("Release(%q) error = %v", lease.key, err)
		}
	}
}

func TestInternalLeaseKeysAlwaysPrependIntersectingPrefix(t *testing.T) {
	redisServer := miniredis.RunT(t)
	newManager := func(t *testing.T, prefix string) *Manager {
		t.Helper()
		config := managerTestConfig(redisServer.Addr())
		config.QueuePrefix = prefix
		manager, err := NewManager(context.Background(), config)
		if err != nil {
			t.Fatalf("NewManager(%q) error = %v", prefix, err)
		}
		t.Cleanup(func() { _ = manager.Stop() })
		return manager
	}
	unprefixed := newManager(t, "")
	pollPrefix := newManager(t, "poll")
	migrationPrefix := newManager(t, "asynq:")

	plainPolling, acquired, err := unprefixed.AcquirePollingLease(context.Background(), "task-123")
	if err != nil || !acquired {
		t.Fatalf("unprefixed polling lease = (%v, %v), want acquired", acquired, err)
	}
	prefixedPolling, acquired, err := pollPrefix.AcquirePollingLease(context.Background(), "task-123")
	if err != nil || !acquired {
		t.Fatalf("intersecting-prefix polling lease = (%v, %v), want independently acquired", acquired, err)
	}
	if prefixedPolling.key != "pollpolling_lock:task-123" {
		t.Fatalf("intersecting-prefix polling key = %q, want pollpolling_lock:task-123", prefixedPolling.key)
	}

	plainMigration, acquired, err := unprefixed.AcquireMigrationLease(context.Background())
	if err != nil || !acquired {
		t.Fatalf("unprefixed migration lease = (%v, %v), want acquired", acquired, err)
	}
	prefixedMigration, acquired, err := migrationPrefix.AcquireMigrationLease(context.Background())
	if err != nil || !acquired {
		t.Fatalf("intersecting-prefix migration lease = (%v, %v), want independently acquired", acquired, err)
	}
	if prefixedMigration.key != "asynq:asynq:migration_lock" {
		t.Fatalf("intersecting-prefix migration key = %q, want asynq:asynq:migration_lock", prefixedMigration.key)
	}

	for _, lease := range []*Lease{plainPolling, prefixedPolling, plainMigration, prefixedMigration} {
		if err := lease.Release(context.Background()); err != nil {
			t.Errorf("Release(%q) error = %v", lease.key, err)
		}
	}
}
