package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	asq "github.com/hibiken/asynq"
)

// =========================================
// 死信队列管理器（简化通用版本）
// =========================================

// DeadLetterManager 死信队列管理器
type DeadLetterManager struct {
	manager *Manager
	mu      sync.RWMutex
}

var (
	globalDeadLetterManager *DeadLetterManager
	deadLetterOnce          sync.Once
)

// GetDeadLetterManager 获取全局死信队列管理器
func GetDeadLetterManager() *DeadLetterManager {
	deadLetterOnce.Do(func() {
		globalDeadLetterManager = &DeadLetterManager{}
	})
	return globalDeadLetterManager
}

// SetManager 设置管理器
func (dlm *DeadLetterManager) SetManager(manager *Manager) {
	dlm.mu.Lock()
	defer dlm.mu.Unlock()
	dlm.manager = manager
}

// SendToDeadLetter 将任务发送到死信队列
// payload: 任务载荷（必须可 JSON 序列化）
// taskID: 任务唯一标识
func (dlm *DeadLetterManager) SendToDeadLetter(ctx context.Context, taskID string, payload interface{}, reason string) error {
	dlm.mu.RLock()
	manager := dlm.manager
	dlm.mu.RUnlock()

	if manager == nil {
		return fmt.Errorf("asynq manager not initialized")
	}

	// 构建死信载荷
	dlPayload := map[string]interface{}{
		"task_id":     taskID,
		"payload":     payload,
		"reason":      reason,
		"failed_at":   time.Now().Unix(),
		"retry_count": 0,
	}

	data, err := json.Marshal(dlPayload)
	if err != nil {
		return fmt.Errorf("marshal dead letter payload failed: %w", err)
	}

	// 创建死信任务
	task := asq.NewTask("dead_letter", data)

	// 入队到死信队列
	_, err = manager.Enqueue(ctx, task,
		asq.Queue(QueueDeadLetter),
		asq.TaskID(fmt.Sprintf("dlq:%s:%d", taskID, time.Now().UnixNano())),
		asq.MaxRetry(0),               // 死信队列不重试
		asq.Retention(7*24*time.Hour), // 保留 7 天
	)

	if err != nil {
		return fmt.Errorf("enqueue to dead letter failed: %w", err)
	}

	GetLogger().Log(fmt.Sprintf("[DeadLetter] Task %s moved to DLQ, reason: %s", taskID, reason))
	return nil
}
