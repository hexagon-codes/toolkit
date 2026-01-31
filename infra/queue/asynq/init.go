package asynq

import (
	"context"
	"fmt"
	"time"
)

// =========================================
// Asynq 任务轮询系统初始化
// =========================================
// PollingConfig Asynq 轮询配置
type PollingConfig struct {
	// Enabled 是否启用 Asynq 轮询（启用后替代传统的批量更新方式）
	Enabled bool
	// RedisAddr Redis 地址（可选，默认使用系统 Redis 配置）
	RedisAddr string
	// Concurrency Worker 并发数
	Concurrency int
	// MigrateExisting 启动时是否迁移现有未完成任务
	MigrateExisting bool
}

// DefaultPollingConfig 默认配置
func DefaultPollingConfig() PollingConfig {
	return PollingConfig{
		Enabled:         GetConfigProvider().IsPollingEnabled(),
		RedisAddr:       "", // 使用系统统一的 Redis 配置
		Concurrency:     GetConfigProvider().GetConcurrency(),
		MigrateExisting: false, // 默认不迁移，需要时通过环境变量临时开启
	}
}

// WorkerDependencies Worker 依赖（由外部注入）
type WorkerDependencies struct {
	// RegisterTaskPollWorker 注册任务轮询 Worker
	RegisterTaskPollWorker func() error
	// RegisterWebhookWorker 注册 Webhook Worker
	RegisterWebhookWorker func() error
	// RegisterBatchPollWorker 注册批量轮询 Worker
	RegisterBatchPollWorker func() error
	// RegisterStatsWorker 注册统计 Worker（可选）
	RegisterStatsWorker func(*Manager)
	// MigrateFunc 迁移现有任务的函数
	MigrateFunc func(ctx context.Context, dryRun bool) (int, error)
}

// InitPolling 初始化 Asynq 任务轮询系统
// 返回 cleanup 函数，应在应用退出时调用
func InitPolling(config PollingConfig, deps WorkerDependencies) (func(), error) {
	if !config.Enabled {
		GetLogger().Log("[Asynq] Polling disabled, using legacy batch update mechanism")
		return func() {}, nil
	}
	// 初始化队列名称（添加环境前缀，用于多环境隔离）
	InitQueueNames()
	if prefix := GetConfigProvider().GetQueuePrefix(); prefix != "" {
		GetLogger().Log(fmt.Sprintf("[Asynq] Queue prefix enabled: %s (queues: %s, %s, %s, %s, %s, %s)",
			prefix, QueueCritical, QueueHigh, QueueDefault, QueueScheduled, QueueLow, QueueDeadLetter))
	}
	// 检查 Redis 是否启用
	if !GetConfigProvider().IsRedisEnabled() {
		return nil, fmt.Errorf("redis not enabled, cannot initialize asynq polling system")
	}
	// 获取 Redis 集群配置
	redisConfig := getRedisConfigFromProvider()
	if redisConfig == nil {
		return nil, fmt.Errorf("redis config is invalid")
	}
	// 调试日志：检查 Redis 配置
	passwordMask := "****"
	if redisConfig.Password == "" {
		passwordMask = "(空密码-WARNING)"
	}
	usernameMask := redisConfig.Username
	if usernameMask == "" {
		usernameMask = "(default)"
	}
	GetLogger().Log(fmt.Sprintf("[Asynq-Config] Cluster nodes=%v, Username=%s, Password=%s",
		redisConfig.Addrs, usernameMask, passwordMask))
	// 1. 初始化 Asynq Manager（集群模式）
	managerConfig := &Config{
		RedisAddrs:  redisConfig.Addrs,
		Password:    redisConfig.Password,
		Username:    redisConfig.Username,
		Concurrency: config.Concurrency,
		Queues:      DefaultQueues(),
	}
	manager, err := InitManager(managerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create asynq manager: %w", err)
	}
	// 2. 注册处理器
	if deps.RegisterTaskPollWorker != nil {
		if err := deps.RegisterTaskPollWorker(); err != nil {
			return nil, fmt.Errorf("failed to register task poll worker: %w", err)
		}
	}
	if deps.RegisterWebhookWorker != nil {
		if err := deps.RegisterWebhookWorker(); err != nil {
			return nil, fmt.Errorf("failed to register webhook worker: %w", err)
		}
	}
	if deps.RegisterBatchPollWorker != nil {
		if err := deps.RegisterBatchPollWorker(); err != nil {
			return nil, fmt.Errorf("failed to register batch poll worker: %w", err)
		}
	}
	// 注册统计 Worker（定时任务）
	if deps.RegisterStatsWorker != nil {
		deps.RegisterStatsWorker(manager)
	}
	// 3. 初始化背压控制器
	backpressure := GetBackpressureController()
	backpressure.SetManager(manager)
	backpressure.SetConfig(BackpressureConfig{
		MaxQueueSize:      10000,
		WarningThreshold:  0.7,
		CriticalThreshold: 0.9,
		CheckInterval:     30 * time.Second, // 修复：之前是 30 纳秒，现在是 30 秒
		OnWarning: func(queue string, size int, threshold int) {
			GetLogger().Log(fmt.Sprintf("[Backpressure-Warning] Queue %s at %d/%d", queue, size, threshold))
		},
		OnCritical: func(queue string, size int, threshold int) {
			GetLogger().Error(fmt.Sprintf("[Backpressure-Critical] Queue %s at %d/%d", queue, size, threshold))
		},
		OnRecover: func(queue string, size int) {
			GetLogger().Log(fmt.Sprintf("[Backpressure-Recover] Queue %s recovered to %d", queue, size))
		},
	})
	backpressure.Start()
	// 4. 启动 Worker
	if err := manager.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start asynq worker: %w", err)
	}
	GetLogger().Log("[Asynq] Polling system initialized successfully")
	// 5. 迁移现有任务（可选，使用分布式锁确保只有一个 Pod 执行）
	if config.MigrateExisting && deps.MigrateFunc != nil {
		go func() {
			// 尝试获取迁移锁
			if !AcquireMigrationLock() {
				GetLogger().Log("[Asynq] Another pod is migrating, skip migration on this pod")
				return
			}
			defer ReleaseMigrationLock()
			GetLogger().Log("[Asynq] Acquired migration lock, starting migration...")
			count, err := deps.MigrateFunc(context.Background(), false)
			if err != nil {
				GetLogger().Error(fmt.Sprintf("[Asynq] Migration failed: %v", err))
			} else {
				GetLogger().Log(fmt.Sprintf("[Asynq] Migrated %d tasks to Asynq polling", count))
			}
		}()
	}
	// 返回清理函数
	cleanup := func() {
		GetLogger().Log("[Asynq] Shutting down polling system...")
		backpressure.Stop()
		manager.Stop()
		GetLogger().Log("[Asynq] Polling system shutdown complete")
	}
	return cleanup, nil
}

// IsPollingEnabled 检查是否启用 Asynq 轮询
func IsPollingEnabled() bool {
	return GetManager() != nil
}
