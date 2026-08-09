package asynq

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

// =========================================
// 队列定义
// 统一管理所有队列名称和优先级
// =========================================
// Queue base names are immutable. Apply an environment prefix through
// Config.QueueName or Manager.QueueName; package state is never rewritten.
const (
	// QueueCritical 关键队列（最高优先级）
	// 用于：紧急任务、实时处理等
	QueueCritical = "critical"
	// QueueHigh 高优先级队列
	// 用于：需要快速响应的任务
	QueueHigh = "high"
	// QueueDefault 默认队列
	// 用于：一般异步任务
	QueueDefault = "default"
	// QueueScheduled 定时任务队列
	// 用于：延迟执行的任务
	QueueScheduled = "scheduled"
	// QueueLow 低优先级队列
	// 用于：后台任务、批量处理等
	QueueLow = "low"
	// QueueDeadLetter 死信队列
	// 用于：超过最大重试次数的失败任务，支持人工介入
	QueueDeadLetter = "dead_letter"
)

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
	return defaultQueuesForPrefix("")
}

func defaultQueuesForPrefix(prefix string) map[string]int {
	return map[string]int{
		queueName(prefix, QueueCritical):   10, // 关键任务，最高优先级
		queueName(prefix, QueueHigh):       6,  // 高优先级任务
		queueName(prefix, QueueDefault):    4,  // 一般任务
		queueName(prefix, QueueScheduled):  3,  // 定时任务
		queueName(prefix, QueueLow):        1,  // 低优先级
		queueName(prefix, QueueDeadLetter): 1,  // 死信队列（人工处理）
	}
}

func prefixQueues(queues map[string]int, prefix string) (map[string]int, error) {
	prefixed := make(map[string]int, len(queues))
	for queue, priority := range queues {
		if strings.TrimSpace(queue) == "" {
			return nil, fmt.Errorf("%w: queue name must not be blank", ErrInvalidConfig)
		}
		if priority <= 0 {
			return nil, fmt.Errorf(
				"%w: queue %q priority must be positive",
				ErrInvalidConfig,
				queue,
			)
		}
		queue = queueName(prefix, queue)
		if _, exists := prefixed[queue]; exists {
			return nil, fmt.Errorf("%w: queue prefix creates duplicate %q", ErrInvalidConfig, queue)
		}
		prefixed[queue] = priority
	}
	return prefixed, nil
}

func queueName(prefix, base string) string {
	if prefix == "" {
		return base
	}
	return prefix + base
}

// QueueName applies this configuration's environment prefix to a base name.
func (c Config) QueueName(base string) string {
	return queueName(c.QueuePrefix, base)
}

// QueueName applies this manager's immutable environment prefix to a base name.
func (m *Manager) QueueName(base string) string {
	return m.config.QueueName(base)
}

// QueueNames returns an independent, deterministic snapshot of configured
// queue names.
func (m *Manager) QueueNames() []string {
	names := make([]string, 0, len(m.config.Queues))
	for name := range m.config.Queues {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resolveQueue treats queue as an unnamespaced base name and resolves it to
// one of this manager's configured queues. Callers cannot route work into a
// different manager's namespace.
func (m *Manager) resolveQueue(base string) (string, error) {
	actual := m.QueueName(base)
	if _, ok := m.config.Queues[actual]; !ok {
		return "", fmt.Errorf(
			"%w: base queue %q resolves to unconfigured queue %q",
			ErrQueueNotFound,
			base,
			actual,
		)
	}
	return actual, nil
}

// canonicalizeQueueOptions makes Manager the final owner of task routing.
// The last explicit Queue option is interpreted as a base name; omission uses
// QueueDefault. Appending the resolved option also overrides Task-embedded
// queue options, which Asynq merges before Enqueue options.
func (m *Manager) canonicalizeQueueOptions(opts []asynq.Option) ([]asynq.Option, error) {
	base := QueueDefault
	normalized := make([]asynq.Option, 0, len(opts)+1)
	for _, opt := range opts {
		if opt == nil {
			normalized = append(normalized, opt)
			continue
		}
		if opt.Type() != asynq.QueueOpt {
			normalized = append(normalized, opt)
			continue
		}
		value, ok := opt.Value().(string)
		if !ok {
			return nil, fmt.Errorf("%w: queue option value must be a string", ErrQueueNotFound)
		}
		base = value
	}
	actual, err := m.resolveQueue(base)
	if err != nil {
		return nil, err
	}
	return append(normalized, asynq.Queue(actual)), nil
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
