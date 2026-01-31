package asynq

import (
	"context"
	"fmt"
	"time"
)

const (
	// PollingLockTTL 轮询锁过期时间（秒）
	// 计算依据：
	// - 默认轮询间隔：180s（配置轮询周期 × 12）
	// - 单次最长操作：任务处理(60s) + 文件上传(300s) = 360s
	// - 安全余量：60s
	// 设置为 480s（8分钟）确保：
	// - 无需依赖续租机制即可覆盖所有场景
	// - ExtendPollingLock 续租作为额外保障（超长任务）
	// - Worker 崩溃后最多 8 分钟恢复（可接受）
	PollingLockTTL = 480 // 8 分钟

	// redisOpTimeout Redis 操作超时时间
	redisOpTimeout = 5 * time.Second
)

// MarkTaskAsPolling 标记任务正在轮询
// 返回 true 表示成功获取锁，false 表示任务已在轮询中
func MarkTaskAsPolling(taskID string) bool {
	if !GetConfigProvider().IsRedisEnabled() {
		return true // Redis 未启用，允许继续
	}
	key := fmt.Sprintf("polling_lock:%s", taskID)
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	// 使用 SetNX（只在不存在时设置）
	success, err := GetRedisClient().SetNX(ctx, key, "1", PollingLockTTL*time.Second).Result()
	if err != nil {
		GetLogger().Log(fmt.Sprintf("[PollingLock] SetNX error: %s, err=%v", taskID, err))
		return true // 错误时允许继续（降级）
	}
	return success
}

// IsTaskPolling 检查任务是否正在轮询
func IsTaskPolling(taskID string) bool {
	if !GetConfigProvider().IsRedisEnabled() {
		return false
	}
	key := fmt.Sprintf("polling_lock:%s", taskID)
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	val, err := GetRedisClient().Get(ctx, key).Result()
	if err != nil {
		return false // 不存在或错误，认为未轮询
	}
	return val == "1"
}

// ReleasePollingLock 释放轮询锁（可选，依赖 TTL 自动过期）
func ReleasePollingLock(taskID string) {
	if !GetConfigProvider().IsRedisEnabled() {
		return
	}
	key := fmt.Sprintf("polling_lock:%s", taskID)
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	GetRedisClient().Del(ctx, key)
}

// ExtendPollingLock 延长轮询锁（任务处理时间超过 TTL 时使用）
func ExtendPollingLock(taskID string) {
	if !GetConfigProvider().IsRedisEnabled() {
		return
	}
	key := fmt.Sprintf("polling_lock:%s", taskID)
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	GetRedisClient().Expire(ctx, key, PollingLockTTL*time.Second)
}

// =========================================
// 迁移分布式锁
// 确保多 Pod 启动时只有一个执行迁移
// =========================================
const (
	// MigrationLockKey 迁移锁的 Redis key
	MigrationLockKey = "asynq:migration_lock"
	// MigrationLockTTL 迁移锁过期时间（2分钟）
	// 缩短 TTL 以便 Pod 崩溃后其他 Pod 能更快接管迁移任务
	// 如果迁移任务量大，会在执行过程中定期续租
	MigrationLockTTL = 2 * time.Minute
)

// AcquireMigrationLock 获取迁移锁
// 返回 true 表示成功获取锁，可以执行迁移
// 返回 false 表示其他 Pod 正在迁移，应跳过
func AcquireMigrationLock() bool {
	if !GetConfigProvider().IsRedisEnabled() {
		return true // Redis 未启用，允许继续（单 Pod 场景）
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	// 使用 SetNX 尝试获取锁
	success, err := GetRedisClient().SetNX(ctx, MigrationLockKey, "1", MigrationLockTTL).Result()
	if err != nil {
		GetLogger().Log(fmt.Sprintf("[MigrationLock] SetNX error: %v, allowing migration", err))
		return true // 错误时允许继续（降级）
	}
	return success
}

// ReleaseMigrationLock 释放迁移锁
func ReleaseMigrationLock() {
	if !GetConfigProvider().IsRedisEnabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	GetRedisClient().Del(ctx, MigrationLockKey)
}
