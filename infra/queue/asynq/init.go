package asynq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// PollingConfig contains polling policy only. Redis and worker runtime
// settings belong to Config.
type PollingConfig struct {
	Enabled         bool
	MigrateExisting bool
}

// DefaultPollingConfig keeps polling opt-in and migration separately opt-in.
func DefaultPollingConfig() PollingConfig {
	return PollingConfig{}
}

// WorkerDependencies are host application hooks installed during startup.
type WorkerDependencies struct {
	RegisterTaskPollWorker  func() error
	RegisterWebhookWorker   func() error
	RegisterBatchPollWorker func() error
	RegisterStatsWorker     func(*Manager)
	MigrateFunc             func(ctx context.Context, dryRun bool) (int, error)
}

// InitPolling initializes a verified manager, registers workers, starts all
// queue components, and optionally performs a migration under a token lease.
// The returned cleanup owns all resources created here.
func InitPolling(
	ctx context.Context,
	pollingConfig PollingConfig,
	managerConfig Config,
	deps WorkerDependencies,
) (func() error, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: initialize polling requires a non-nil context", ErrInvalidContext)
	}
	if !pollingConfig.Enabled {
		GetLogger().Log("[Asynq] Polling disabled, using legacy batch update mechanism")
		return func() error { return nil }, nil
	}

	manager, err := InitManager(ctx, managerConfig)
	if err != nil {
		return nil, fmt.Errorf("asynq: initialize polling manager: %w", err)
	}
	rollback := func(cause error) (func() error, error) {
		uninstallGlobalManager(manager)
		return nil, errors.Join(cause, manager.Stop())
	}

	if manager.config.QueuePrefix != "" {
		GetLogger().Log(fmt.Sprintf(
			"[Asynq] Queue prefix enabled: %s (queues: %s, %s, %s, %s, %s, %s)",
			manager.config.QueuePrefix,
			manager.QueueName(QueueCritical),
			manager.QueueName(QueueHigh),
			manager.QueueName(QueueDefault),
			manager.QueueName(QueueScheduled),
			manager.QueueName(QueueLow),
			manager.QueueName(QueueDeadLetter),
		))
	}

	if deps.RegisterTaskPollWorker != nil {
		if err := deps.RegisterTaskPollWorker(); err != nil {
			return rollback(fmt.Errorf("asynq: register task poll worker: %w", err))
		}
	}
	if deps.RegisterWebhookWorker != nil {
		if err := deps.RegisterWebhookWorker(); err != nil {
			return rollback(fmt.Errorf("asynq: register webhook worker: %w", err))
		}
	}
	if deps.RegisterBatchPollWorker != nil {
		if err := deps.RegisterBatchPollWorker(); err != nil {
			return rollback(fmt.Errorf("asynq: register batch poll worker: %w", err))
		}
	}
	if deps.RegisterStatsWorker != nil {
		deps.RegisterStatsWorker(manager)
	}

	if err := manager.Start(ctx); err != nil {
		return rollback(fmt.Errorf("asynq: start polling workers: %w", err))
	}

	backpressure := GetBackpressureController()
	backpressure.SetManager(manager)
	if err := backpressure.SetConfig(BackpressureConfig{
		MaxQueueSize:      10000,
		WarningThreshold:  0.7,
		CriticalThreshold: 0.9,
		CheckInterval:     30 * time.Second,
		OnWarning: func(queue string, size, threshold int) {
			GetLogger().Log(fmt.Sprintf(
				"[Backpressure-Warning] Queue %s at %d/%d",
				queue,
				size,
				threshold,
			))
		},
		OnCritical: func(queue string, size, threshold int) {
			GetLogger().Error(fmt.Sprintf(
				"[Backpressure-Critical] Queue %s at %d/%d",
				queue,
				size,
				threshold,
			))
		},
		OnRecover: func(queue string, size int) {
			GetLogger().Log(fmt.Sprintf(
				"[Backpressure-Recover] Queue %s recovered to %d",
				queue,
				size,
			))
		},
	}); err != nil {
		backpressure.SetManager(nil)
		return rollback(fmt.Errorf("asynq: configure backpressure: %w", err))
	}
	if err := backpressure.Start(); err != nil {
		backpressure.SetManager(nil)
		return rollback(fmt.Errorf("asynq: start backpressure: %w", err))
	}

	if pollingConfig.MigrateExisting && deps.MigrateFunc != nil {
		if err := runMigration(ctx, manager, deps.MigrateFunc); err != nil {
			backpressure.Stop()
			backpressure.SetManager(nil)
			return rollback(fmt.Errorf("asynq: migrate existing tasks: %w", err))
		}
	}

	GetLogger().Log("[Asynq] Polling system initialized successfully")
	var cleanupOnce sync.Once
	var cleanupErr error
	cleanup := func() error {
		cleanupOnce.Do(func() {
			GetLogger().Log("[Asynq] Shutting down polling system...")
			backpressure.Stop()
			backpressure.SetManager(nil)
			uninstallGlobalManager(manager)
			cleanupErr = manager.Stop()
			GetLogger().Log("[Asynq] Polling system shutdown complete")
		})
		return cleanupErr
	}
	return cleanup, nil
}

func runMigration(
	ctx context.Context,
	manager *Manager,
	migrate func(context.Context, bool) (int, error),
) error {
	return runMigrationWithLeaseTTL(ctx, manager, migrate, MigrationLockTTL)
}

func runMigrationWithLeaseTTL(
	ctx context.Context,
	manager *Manager,
	migrate func(context.Context, bool) (int, error),
	leaseTTL time.Duration,
) (resultErr error) {
	lease, acquired, err := manager.acquireMigrationLease(ctx, leaseTTL)
	if err != nil {
		return err
	}
	if !acquired {
		GetLogger().Log("[Asynq] Another pod is migrating, skip migration on this pod")
		return nil
	}
	defer func() {
		cleanupParent := context.WithoutCancel(ctx)
		cleanupCtx, cancel := context.WithTimeout(cleanupParent, redisOpTimeout)
		defer cancel()
		if err := lease.Release(cleanupCtx); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release migration lease: %w", err))
		}
	}()

	GetLogger().Log("[Asynq] Acquired migration lease, starting migration...")
	migrationCtx, cancelMigration := context.WithCancel(ctx)
	defer cancelMigration()
	refreshStop := make(chan struct{})
	refreshDone := make(chan error, 1)
	refreshInterval := leaseTTL / 3
	if refreshInterval < time.Millisecond {
		refreshInterval = time.Millisecond
	}
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-refreshStop:
				refreshDone <- nil
				return
			case <-migrationCtx.Done():
				refreshDone <- nil
				return
			case <-ticker.C:
				if err := lease.Refresh(migrationCtx); err != nil {
					cancelMigration()
					refreshDone <- fmt.Errorf("refresh migration lease: %w", err)
					return
				}
			}
		}
	}()

	count, migrationErr := migrate(migrationCtx, false)
	close(refreshStop)
	refreshErr := <-refreshDone
	if err := errors.Join(migrationErr, refreshErr); err != nil {
		return err
	}
	GetLogger().Log(fmt.Sprintf("[Asynq] Migrated %d tasks to Asynq polling", count))
	return nil
}

func uninstallGlobalManager(manager *Manager) {
	managerMu.Lock()
	if globalManager == manager {
		globalManager = nil
	}
	managerMu.Unlock()
}

// IsPollingEnabled reports whether the global polling manager is fully started.
func IsPollingEnabled() bool {
	manager := GetManager()
	return manager != nil && manager.IsStarted()
}
