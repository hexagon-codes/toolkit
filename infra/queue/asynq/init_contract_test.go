package asynq

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	queue "github.com/hibiken/asynq"
)

func TestPollingConfigContainsOnlyPollingPolicy(t *testing.T) {
	typeOfConfig := reflect.TypeOf(PollingConfig{})
	want := []string{"Enabled", "MigrateExisting"}
	if typeOfConfig.NumField() != len(want) {
		t.Fatalf("PollingConfig field count = %d, want %d", typeOfConfig.NumField(), len(want))
	}
	for index, name := range want {
		if typeOfConfig.Field(index).Name != name {
			t.Fatalf("PollingConfig field %d = %q, want %q", index, typeOfConfig.Field(index).Name, name)
		}
	}
}

func TestInitPollingStartFailureRollsBackGlobalManagerAndResources(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	var initialized *Manager

	cleanup, err := InitPolling(
		context.Background(),
		PollingConfig{Enabled: true},
		managerTestConfig(redisServer.Addr()),
		WorkerDependencies{
			RegisterStatsWorker: func(manager *Manager) {
				initialized = manager
				if err := manager.RegisterSchedule("not-a-cron-expression", queue.NewTask("stats", nil)); err != nil {
					t.Fatalf("RegisterSchedule() error = %v", err)
				}
			},
		},
	)
	if err == nil {
		if cleanup != nil {
			_ = cleanup()
		}
		t.Fatal("InitPolling() error = nil, want synchronous scheduler registration failure")
	}
	if cleanup != nil {
		t.Fatal("InitPolling() cleanup is non-nil after failed initialization")
	}
	if initialized == nil {
		t.Fatal("RegisterStatsWorker did not receive the initialized manager")
	}
	if initialized.IsStarted() {
		t.Fatal("manager remains started after startup rollback")
	}
	if GetManager() != nil {
		t.Fatal("failed InitPolling() left a global manager installed")
	}
	if err := initialized.redisClient.Ping(context.Background()).Err(); err == nil {
		t.Fatal("failed InitPolling() left its Redis client open")
	}
}

func TestInitPollingMigrationOwnsAndReleasesLease(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	var observedCompetingOwner bool

	cleanup, err := InitPolling(
		context.Background(),
		PollingConfig{Enabled: true, MigrateExisting: true},
		managerTestConfig(redisServer.Addr()),
		WorkerDependencies{
			MigrateFunc: func(ctx context.Context, _ bool) (int, error) {
				lease, acquired, err := GetManager().AcquireMigrationLease(ctx)
				if err != nil {
					return 0, err
				}
				if acquired || lease != nil {
					observedCompetingOwner = true
				}
				return 3, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("InitPolling() error = %v", err)
	}
	if cleanup == nil {
		t.Fatal("InitPolling() cleanup = nil")
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Errorf("cleanup() error = %v", cleanupErr)
		}
	}()
	if observedCompetingOwner {
		t.Fatal("migration callback acquired a second migration lease")
	}

	lease, acquired, err := GetManager().AcquireMigrationLease(context.Background())
	if err != nil || !acquired || lease == nil {
		t.Fatalf("lease after migration = (%v, %v, %v), want lease, true, nil", lease, acquired, err)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("Release() after migration error = %v", err)
	}
}

func TestRunMigrationRefreshesShortLeaseUntilWorkCompletes(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	leaseTTL := 90 * time.Millisecond

	err = runMigrationWithLeaseTTL(
		context.Background(),
		manager,
		func(ctx context.Context, _ bool) (int, error) {
			redisServer.FastForward(2 * leaseTTL / 3)
			time.Sleep(leaseTTL / 2)
			redisServer.FastForward(2 * leaseTTL / 3)
			if !redisServer.Exists(MigrationLockKey) {
				return 0, errors.New("migration lease expired during long-running work")
			}
			competing, acquired, acquireErr := manager.acquireLease(ctx, MigrationLockKey, leaseTTL)
			if acquireErr != nil {
				return 0, acquireErr
			}
			if acquired || competing != nil {
				return 0, errors.New("another owner acquired the migration lease")
			}
			return 1, nil
		},
		leaseTTL,
	)
	if err != nil {
		t.Fatalf("runMigrationWithLeaseTTL() error = %v", err)
	}
}

func TestRunMigrationRefreshFailureCancelsWorkAndFailsClosed(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	leaseTTL := 30 * time.Millisecond

	err = runMigrationWithLeaseTTL(
		context.Background(),
		manager,
		func(ctx context.Context, _ bool) (int, error) {
			redisServer.Close()
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Second):
				return 0, errors.New("migration context was not canceled after refresh failure")
			}
		},
		leaseTTL,
	)
	if err == nil {
		t.Fatal("runMigrationWithLeaseTTL() error = nil, want refresh failure")
	}
}

func TestRunMigrationReleasesWithIndependentContextAfterCancellation(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	ctx, cancel := context.WithCancel(context.Background())

	err = runMigrationWithLeaseTTL(
		ctx,
		manager,
		func(ctx context.Context, _ bool) (int, error) {
			cancel()
			<-ctx.Done()
			return 0, ctx.Err()
		},
		100*time.Millisecond,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runMigrationWithLeaseTTL() error = %v, want context.Canceled", err)
	}
	if redisServer.Exists(MigrationLockKey) {
		t.Fatal("migration lease remained after caller context cancellation")
	}
}
