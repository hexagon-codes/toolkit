package asynq

import (
	"context"
	"encoding/json"
	"github.com/hibiken/asynq"
	"time"
)

// =========================================
// 任务构建器和辅助函数
// 提供便捷的任务创建和入队方法
// =========================================
// TaskBuilder 任务构建器
type TaskBuilder struct {
	taskType string
	payload  interface{}
	opts     []asynq.Option
}

// NewTask 创建任务构建器
func NewTask(taskType string) *TaskBuilder {
	return &TaskBuilder{
		taskType: taskType,
		opts:     make([]asynq.Option, 0),
	}
}

// Payload 设置载荷
func (b *TaskBuilder) Payload(payload interface{}) *TaskBuilder {
	b.payload = payload
	return b
}

// Queue 设置队列
func (b *TaskBuilder) Queue(queue string) *TaskBuilder {
	b.opts = append(b.opts, asynq.Queue(queue))
	return b
}

// MaxRetry 设置最大重试次数
func (b *TaskBuilder) MaxRetry(n int) *TaskBuilder {
	b.opts = append(b.opts, asynq.MaxRetry(n))
	return b
}

// Timeout 设置超时时间
func (b *TaskBuilder) Timeout(d time.Duration) *TaskBuilder {
	b.opts = append(b.opts, asynq.Timeout(d))
	return b
}

// Deadline 设置截止时间
func (b *TaskBuilder) Deadline(t time.Time) *TaskBuilder {
	b.opts = append(b.opts, asynq.Deadline(t))
	return b
}

// ProcessIn 延迟处理
func (b *TaskBuilder) ProcessIn(d time.Duration) *TaskBuilder {
	b.opts = append(b.opts, asynq.ProcessIn(d))
	return b
}

// ProcessAt 定时处理
func (b *TaskBuilder) ProcessAt(t time.Time) *TaskBuilder {
	b.opts = append(b.opts, asynq.ProcessAt(t))
	return b
}

// TaskID 设置任务 ID（用于去重）
func (b *TaskBuilder) TaskID(id string) *TaskBuilder {
	b.opts = append(b.opts, asynq.TaskID(id))
	return b
}

// Unique 设置唯一性约束
func (b *TaskBuilder) Unique(d time.Duration) *TaskBuilder {
	b.opts = append(b.opts, asynq.Unique(d))
	return b
}

// Retention 设置结果保留时间
func (b *TaskBuilder) Retention(d time.Duration) *TaskBuilder {
	b.opts = append(b.opts, asynq.Retention(d))
	return b
}

// Build 构建任务
func (b *TaskBuilder) Build() (*asynq.Task, error) {
	var data []byte
	var err error
	if b.payload != nil {
		data, err = json.Marshal(b.payload)
		if err != nil {
			return nil, err
		}
	}
	return asynq.NewTask(b.taskType, data, b.opts...), nil
}

// Enqueue 直接入队（使用全局管理器）
func (b *TaskBuilder) Enqueue(ctx context.Context) (*asynq.TaskInfo, error) {
	task, err := b.Build()
	if err != nil {
		return nil, err
	}
	manager := GetManager()
	if manager == nil {
		return nil, ErrManagerNotInitialized
	}
	return manager.Enqueue(ctx, task)
}

// EnqueueWith 使用指定管理器入队
func (b *TaskBuilder) EnqueueWith(ctx context.Context, m *Manager) (*asynq.TaskInfo, error) {
	task, err := b.Build()
	if err != nil {
		return nil, err
	}
	return m.Enqueue(ctx, task)
}

// =========================================
// 快捷入队函数
// =========================================
// EnqueueTask 快捷入队任务
func EnqueueTask(ctx context.Context, taskType string, payload interface{}, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	manager := GetManager()
	if manager == nil {
		return nil, ErrManagerNotInitialized
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	task := asynq.NewTask(taskType, data, opts...)
	return manager.Enqueue(ctx, task, opts...)
}

// EnqueueTaskDelayed 延迟入队任务
func EnqueueTaskDelayed(ctx context.Context, taskType string, payload interface{}, delay time.Duration, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	opts = append(opts, asynq.ProcessIn(delay))
	return EnqueueTask(ctx, taskType, payload, opts...)
}

// EnqueueTaskAt 定时入队任务
func EnqueueTaskAt(ctx context.Context, taskType string, payload interface{}, processAt time.Time, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	opts = append(opts, asynq.ProcessAt(processAt))
	return EnqueueTask(ctx, taskType, payload, opts...)
}

// EnqueueTaskUnique 唯一任务入队（去重）
func EnqueueTaskUnique(ctx context.Context, taskType string, payload interface{}, taskID string, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	opts = append(opts, asynq.TaskID(taskID))
	return EnqueueTask(ctx, taskType, payload, opts...)
}

// =========================================
// 任务载荷解析
// =========================================
// ParsePayload 解析任务载荷
func ParsePayload[T any](t *asynq.Task) (*T, error) {
	var payload T
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
