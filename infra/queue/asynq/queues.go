package asynq

import (
	"errors"
	"fmt"
	"github.com/hibiken/asynq"
	"time"
)

// =========================================
// 队列定义
// 统一管理所有队列名称和优先级
// =========================================
// 基础队列名称（内部使用）
const (
	baseQueueCritical   = "critical"
	baseQueueHigh       = "high"
	baseQueueDefault    = "default"
	baseQueueScheduled  = "scheduled"
	baseQueueLow        = "low"
	baseQueueDeadLetter = "dead_letter"
)

// 队列名称变量（带环境前缀，在 init 时初始化）
var (
	// QueueCritical 关键队列（最高优先级）
	// 用于：紧急任务、实时处理等
	QueueCritical = baseQueueCritical
	// QueueHigh 高优先级队列
	// 用于：需要快速响应的任务
	QueueHigh = baseQueueHigh
	// QueueDefault 默认队列
	// 用于：一般异步任务
	QueueDefault = baseQueueDefault
	// QueueScheduled 定时任务队列
	// 用于：延迟执行的任务
	QueueScheduled = baseQueueScheduled
	// QueueLow 低优先级队列
	// 用于：后台任务、批量处理等
	QueueLow = baseQueueLow
	// QueueDeadLetter 死信队列
	// 用于：超过最大重试次数的失败任务，支持人工介入
	QueueDeadLetter = baseQueueDeadLetter
)

// InitQueueNames 初始化队列名称（添加环境前缀）
// 必须在使用队列之前调用（通常在 main.go 初始化时）
func InitQueueNames() {
	prefix := GetConfigProvider().GetQueuePrefix()
	if prefix == "" {
		return // 无前缀，使用默认队列名
	}
	QueueCritical = prefix + baseQueueCritical
	QueueHigh = prefix + baseQueueHigh
	QueueDefault = prefix + baseQueueDefault
	QueueScheduled = prefix + baseQueueScheduled
	QueueLow = prefix + baseQueueLow
	QueueDeadLetter = prefix + baseQueueDeadLetter
}

// 预定义任务类型前缀（示例）
// 使用者应该在自己的项目中定义具体的任务类型
const (
	// TaskPrefixTask 任务处理相关
	TaskPrefixTask = "task:"
	// TaskPrefixNotify 通知相关
	TaskPrefixNotify = "notify:"
	// TaskPrefixDLQ 死信队列相关
	TaskPrefixDLQ = "dlq:"
)

// 预定义任务类型（示例）
// 使用者应该在自己的项目中定义具体的任务类型
const (
	// 任务处理相关
	TaskTypeTaskProcess  = TaskPrefixTask + "process"  // 任务处理
	TaskTypeTaskCallback = TaskPrefixTask + "callback" // Webhook 回调处理
	// 死信队列相关
	TaskTypeDeadLetter      = TaskPrefixDLQ + "task"  // 死信任务
	TaskTypeDeadLetterRetry = TaskPrefixDLQ + "retry" // 死信任务重试
	// 通知相关（示例）
	TaskTypeNotifyEmail = TaskPrefixNotify + "email" // 邮件通知
)

// =========================================
// Asynq TaskID 格式
// 用于任务去重和追踪
// =========================================
const (
	// TaskIDPrefixPoll 轮询任务 ID 前缀
	TaskIDPrefixPoll = "poll"
	// TaskIDPrefixRetry 重试任务 ID 前缀
	TaskIDPrefixRetry = "retry"
)

// FormatPollTaskID 生成轮询任务的 Asynq TaskID
// 格式: poll:{taskID}:{retryCount}
// 示例: poll:task_abc123:0
func FormatPollTaskID(taskID string, retryCount int) string {
	return fmt.Sprintf("%s:%s:%d", TaskIDPrefixPoll, taskID, retryCount)
}

// FormatPollTaskIDInitial 生成初始轮询任务的 Asynq TaskID（retryCount=0）
// 格式: poll:{taskID}:0
func FormatPollTaskIDInitial(taskID string) string {
	return FormatPollTaskID(taskID, 0)
}

// FormatRetryTaskID 生成死信队列重试任务的 Asynq TaskID
// 格式: poll:{taskID}:retry:{timestamp}
func FormatRetryTaskID(taskID string) string {
	return fmt.Sprintf("%s:%s:%s:%d", TaskIDPrefixPoll, taskID, TaskIDPrefixRetry, time.Now().UnixNano())
}

// DefaultQueues 默认队列配置
// 数值表示优先级权重，权重越高越优先处理
func DefaultQueues() map[string]int {
	return map[string]int{
		QueueCritical:   10, // 关键任务，最高优先级
		QueueHigh:       6,  // 高优先级任务
		QueueDefault:    4,  // 一般任务
		QueueScheduled:  3,  // 定时任务
		QueueLow:        1,  // 低优先级
		QueueDeadLetter: 1,  // 死信队列（人工处理）
	}
}

// IsTaskConflictError 检查是否是任务冲突错误
// 注意：使用 asynq.TaskID() 时冲突返回 ErrTaskIDConflict
//
//	使用 asynq.Unique() 时冲突返回 ErrDuplicateTask
//
// 这两种错误都表示任务已存在，应该静默跳过而不是返回错误
func IsTaskConflictError(err error) bool {
	return errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict)
}
