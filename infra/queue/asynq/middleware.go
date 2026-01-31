package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hibiken/asynq"
	"runtime/debug"
	"sync"
	"time"
)

// =========================================
// 中间件层
// 提供日志、监控、恢复等横切关注点
// =========================================
// MiddlewareFunc 中间件函数类型
type MiddlewareFunc func(asynq.Handler) asynq.Handler

// LoggingMiddleware 日志中间件
// 记录任务开始、结束、耗时
func LoggingMiddleware(logger Logger) MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			start := time.Now()
			taskID, _ := asynq.GetTaskID(ctx)
			logger.Log(fmt.Sprintf("[Asynq] task_start | type=%s | task_id=%s", t.Type(), taskID))
			err := next.ProcessTask(ctx, t)
			duration := time.Since(start)
			if err != nil {
				logger.Error(fmt.Sprintf("[Asynq] task_fail | type=%s | task_id=%s | duration=%v | error=%v",
					t.Type(), taskID, duration, err))
			} else {
				logger.Log(fmt.Sprintf("[Asynq] task_done | type=%s | task_id=%s | duration=%v",
					t.Type(), taskID, duration))
			}
			return err
		})
	}
}

// RecoveryMiddleware 恢复中间件
// 捕获 panic，防止 worker 崩溃
func RecoveryMiddleware(logger Logger) MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) (err error) {
			defer func() {
				if r := recover(); r != nil {
					taskID, _ := asynq.GetTaskID(ctx)
					stack := string(debug.Stack())
					logger.Error(fmt.Sprintf("[Asynq] task_panic | type=%s | task_id=%s | panic=%v | stack=%s",
						t.Type(), taskID, r, stack))
					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()
			return next.ProcessTask(ctx, t)
		})
	}
}

// MetricsMiddleware 监控指标中间件
func MetricsMiddleware(metrics *Metrics) MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			start := time.Now()
			metrics.IncrTaskProcessed(t.Type())
			// 尝试记录任务等待时间（从入队到开始处理的延迟）
			recordTaskLatencyFromPayload(t)
			err := next.ProcessTask(ctx, t)
			duration := time.Since(start)
			metrics.RecordTaskDuration(t.Type(), duration)
			if err != nil {
				metrics.IncrTaskFailed(t.Type())
			} else {
				metrics.IncrTaskSucceeded(t.Type())
			}
			return err
		})
	}
}

// recordTaskLatencyFromPayload 从 payload 中解析入队时间并记录等待延迟
// 支持任何包含 created_at 字段的 payload
func recordTaskLatencyFromPayload(t *asynq.Task) {
	// 尝试解析 payload 获取 created_at
	var payload struct {
		CreatedAt int64  `json:"created_at"`
		Platform  string `json:"platform"` // 用于确定队列名
		TaskType  int    `json:"task_type"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		// 解析失败，跳过（可能是其他类型的任务）
		return
	}
	if payload.CreatedAt == 0 {
		// 没有入队时间，跳过
		return
	}
	// 计算等待时间
	enqueuedAt := time.Unix(payload.CreatedAt, 0)
	latency := time.Since(enqueuedAt)
	// 确定队列名（根据任务类型）
	queue := getQueueNameByTaskType(payload.TaskType)
	// 记录 Prometheus 指标
	RecordTaskLatency(queue, latency)
}

// getQueueNameByTaskType 根据任务类型返回队列名
func getQueueNameByTaskType(taskType int) string {
	switch taskType {
	case 2: // 图片任务
		return QueueHigh
	case 1: // 视频任务
		return QueueScheduled
	case 3: // 音频任务
		return QueueDefault
	default:
		return QueueDefault
	}
}

// TimeoutMiddleware 超时中间件
func TimeoutMiddleware(defaultTimeout time.Duration) MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			// 检查是否已有 deadline
			if _, ok := ctx.Deadline(); !ok {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
				defer cancel()
			}
			return next.ProcessTask(ctx, t)
		})
	}
}

// RetryInfoMiddleware 重试信息中间件
// 在日志中记录重试次数
func RetryInfoMiddleware(logger Logger) MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
			retryCount, _ := asynq.GetRetryCount(ctx)
			maxRetry, _ := asynq.GetMaxRetry(ctx)
			if retryCount > 0 {
				taskID, _ := asynq.GetTaskID(ctx)
				logger.Log(fmt.Sprintf("[Asynq] task_retry | type=%s | task_id=%s | retry=%d/%d",
					t.Type(), taskID, retryCount, maxRetry))
			}
			return next.ProcessTask(ctx, t)
		})
	}
}

// ChainMiddleware 链式组合中间件
func ChainMiddleware(middlewares ...MiddlewareFunc) MiddlewareFunc {
	return func(handler asynq.Handler) asynq.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}
}

// =========================================
// 预配置的中间件组合
// =========================================
// DefaultMiddlewareChain 默认中间件链
// 包含：恢复 → 日志 → 重试信息
func DefaultMiddlewareChain(logger Logger) MiddlewareFunc {
	return ChainMiddleware(
		RecoveryMiddleware(logger),
		LoggingMiddleware(logger),
		RetryInfoMiddleware(logger),
	)
}

// ProductionMiddlewareChain 生产环境中间件链
// 包含：恢复 → 监控 → 日志 → 超时 → 重试信息
func ProductionMiddlewareChain(logger Logger, metrics *Metrics, defaultTimeout time.Duration) MiddlewareFunc {
	return ChainMiddleware(
		RecoveryMiddleware(logger),
		MetricsMiddleware(metrics),
		LoggingMiddleware(logger),
		TimeoutMiddleware(defaultTimeout),
		RetryInfoMiddleware(logger),
	)
}

// =========================================
// 监控指标收集器
// =========================================
// ringBuffer 环形缓冲区（高效滑动窗口）
type ringBuffer struct {
	data  []time.Duration
	head  int // 下一个写入位置
	count int // 当前元素数量
	size  int // 缓冲区大小
}

// newRingBuffer 创建环形缓冲区
func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		data: make([]time.Duration, size),
		size: size,
	}
}

// Add 添加元素（O(1) 复杂度）
func (r *ringBuffer) Add(d time.Duration) {
	r.data[r.head] = d
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// Average 计算平均值
func (r *ringBuffer) Average() time.Duration {
	if r.count == 0 {
		return 0
	}
	var total time.Duration
	for i := 0; i < r.count; i++ {
		total += r.data[i]
	}
	return total / time.Duration(r.count)
}

// Count 返回元素数量
func (r *ringBuffer) Count() int {
	return r.count
}

// Metrics 任务监控指标（线程安全）
type Metrics struct {
	mu         sync.RWMutex
	processed  map[string]int64
	succeeded  map[string]int64
	failed     map[string]int64
	durations  map[string]*ringBuffer
	maxSamples int
}

// NewMetrics 创建监控指标收集器
func NewMetrics() *Metrics {
	return &Metrics{
		processed:  make(map[string]int64),
		succeeded:  make(map[string]int64),
		failed:     make(map[string]int64),
		durations:  make(map[string]*ringBuffer),
		maxSamples: 1000, // 保留最近 1000 个样本
	}
}
func (m *Metrics) IncrTaskProcessed(taskType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processed[taskType]++
}
func (m *Metrics) IncrTaskSucceeded(taskType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.succeeded[taskType]++
	// 同步更新 Prometheus 指标
	RecordTaskProcessed(taskType, "success")
}
func (m *Metrics) IncrTaskFailed(taskType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed[taskType]++
	// 同步更新 Prometheus 指标
	RecordTaskProcessed(taskType, "failed")
}
func (m *Metrics) RecordTaskDuration(taskType string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.durations[taskType] == nil {
		m.durations[taskType] = newRingBuffer(m.maxSamples)
	}
	m.durations[taskType].Add(d) // O(1) 复杂度
	// 同步更新 Prometheus 指标
	RecordTaskDuration(taskType, d)
}

// GetSnapshot 获取监控快照
func (m *Metrics) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := make(map[string]interface{})
	for taskType := range m.processed {
		avgDuration := time.Duration(0)
		sampleCount := 0
		if rb := m.durations[taskType]; rb != nil {
			avgDuration = rb.Average()
			sampleCount = rb.Count()
		}
		snapshot[taskType] = map[string]interface{}{
			"processed":    m.processed[taskType],
			"succeeded":    m.succeeded[taskType],
			"failed":       m.failed[taskType],
			"avg_duration": avgDuration.String(),
			"samples":      sampleCount,
		}
	}
	return snapshot
}

// =========================================
// 应用中间件到管理器
// =========================================
// WithMiddleware 为管理器设置中间件
func (m *Manager) WithMiddleware(middleware MiddlewareFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.middleware = middleware
}

// RegisterHandlerWithMiddleware 注册带中间件的处理器
func (m *Manager) RegisterHandlerWithMiddleware(taskType string, handler asynq.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[taskType] = handler
	// 如果有中间件，包装处理器
	if m.middleware != nil {
		wrapped := m.middleware(asynq.HandlerFunc(handler))
		m.mux.Handle(taskType, wrapped)
	} else {
		m.mux.HandleFunc(taskType, handler)
	}
	m.logger.Log(fmt.Sprintf("[Asynq] registered handler: %s", taskType))
}

// =========================================
// 便捷函数
// =========================================
// SetupProductionMode 配置生产模式
// 自动添加：恢复、监控、日志、超时中间件
func SetupProductionMode(defaultTimeout time.Duration) (*Metrics, error) {
	m := GetManager()
	if m == nil {
		return nil, ErrManagerNotInitialized
	}
	metrics := NewMetrics()
	middleware := ProductionMiddlewareChain(m.logger, metrics, defaultTimeout)
	m.WithMiddleware(middleware)
	GetLogger().Log("[Asynq] production mode enabled with full middleware chain")
	return metrics, nil
}
