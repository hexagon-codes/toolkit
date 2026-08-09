package asynq

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/hibiken/asynq"
)

// =========================================
// 健康检查
// 提供 Kubernetes liveness/readiness 探针支持
// =========================================

// HealthStatus 健康状态
type HealthStatus struct {
	Healthy     bool              `json:"healthy"`
	Ready       bool              `json:"ready"`
	Details     map[string]string `json:"details"`
	LastChecked time.Time         `json:"last_checked"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	manager       *Manager
	lastStatus    atomic.Value // *HealthStatus
	checkQueuesFn func(context.Context, *HealthStatus) error
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(m *Manager) *HealthChecker {
	checker := &HealthChecker{
		manager: m,
	}
	checker.checkQueuesFn = checker.checkQueues
	return checker
}

// Check 执行健康检查
func (h *HealthChecker) Check(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Healthy:     true,
		Ready:       true,
		Details:     make(map[string]string),
		LastChecked: time.Now(),
	}
	// 检查管理器是否启动
	if h.manager == nil || !h.manager.IsStarted() {
		status.Healthy = false
		status.Ready = false
		status.Details["manager"] = "not started"
		h.lastStatus.Store(cloneHealthStatus(status))
		return status
	}
	status.Details["manager"] = "running"
	// 检查 Redis 连接
	if err := h.checkRedis(ctx); err != nil {
		status.Healthy = false
		status.Ready = false
		status.Details["redis"] = err.Error()
	} else {
		status.Details["redis"] = "connected"
	}
	// 检查队列状态
	if err := h.checkQueuesFn(ctx, status); err != nil {
		status.Healthy = false
		status.Ready = false
		status.Details["queues"] = err.Error()
	}
	h.lastStatus.Store(cloneHealthStatus(status))
	return status
}

func cloneHealthStatus(status *HealthStatus) *HealthStatus {
	if status == nil {
		return nil
	}
	clone := *status
	clone.Details = make(map[string]string, len(status.Details))
	for key, value := range status.Details {
		clone.Details[key] = value
	}
	return &clone
}

// checkRedis 检查 Redis 连接
func (h *HealthChecker) checkRedis(ctx context.Context) error {
	if h.manager == nil {
		return ErrManagerNotInitialized
	}
	return h.manager.pingRedis(ctx)
}

// checkQueues 检查队列状态
func (h *HealthChecker) checkQueues(ctx context.Context, status *HealthStatus) error {
	if ctx == nil {
		return fmt.Errorf("%w: queue health check requires a non-nil context", ErrInvalidContext)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.manager == nil {
		return ErrManagerNotInitialized
	}
	queues := h.manager.QueueNames()
	return h.manager.withInspector(func(inspector *asynq.Inspector) error {
		for _, q := range queues {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := configuredQueueInfo(inspector, q)
			if err != nil {
				return fmt.Errorf("inspect queue %q: %w", q, err)
			}
			// 检查队列积压
			if info.Pending > 10000 {
				status.Details[fmt.Sprintf("queue_%s", q)] = fmt.Sprintf("high_backlog: %d", info.Pending)
			} else {
				status.Details[fmt.Sprintf("queue_%s", q)] = fmt.Sprintf("pending=%d, active=%d", info.Pending, info.Active)
			}
		}
		return nil
	})
}

// IsHealthy 是否健康（用于 liveness 探针）
func (h *HealthChecker) IsHealthy() bool {
	if v := h.lastStatus.Load(); v != nil {
		if status, ok := v.(*HealthStatus); ok {
			return status.Healthy
		}
	}
	return false
}

// IsReady 是否就绪（用于 readiness 探针）
func (h *HealthChecker) IsReady() bool {
	if v := h.lastStatus.Load(); v != nil {
		if status, ok := v.(*HealthStatus); ok {
			return status.Ready
		}
	}
	return false
}

// GetLastStatus 获取最后一次检查状态
func (h *HealthChecker) GetLastStatus() *HealthStatus {
	if v := h.lastStatus.Load(); v != nil {
		if status, ok := v.(*HealthStatus); ok {
			return cloneHealthStatus(status)
		}
	}
	return nil
}

// =========================================
// 优雅关闭
// =========================================

// GracefulShutdown 优雅关闭配置
type GracefulShutdown struct {
	manager         *Manager
	shutdownTimeout time.Duration
	onShutdown      []func()
}

// NewGracefulShutdown 创建优雅关闭处理器
func NewGracefulShutdown(m *Manager, timeout time.Duration) *GracefulShutdown {
	return &GracefulShutdown{
		manager:         m,
		shutdownTimeout: timeout,
		onShutdown:      make([]func(), 0),
	}
}

// OnShutdown 注册关闭回调
func (g *GracefulShutdown) OnShutdown(fn func()) {
	g.onShutdown = append(g.onShutdown, fn)
}

// Shutdown 执行优雅关闭
func (g *GracefulShutdown) Shutdown(ctx context.Context) error {
	if g.manager == nil {
		return ErrManagerNotInitialized
	}
	if ctx == nil {
		return fmt.Errorf("%w: graceful shutdown requires a non-nil context", ErrInvalidContext)
	}
	g.manager.logger.Log("[Asynq] graceful shutdown initiated...")
	// 执行注册的回调
	var callbackErr error
	for _, fn := range g.onShutdown {
		callbackErr = errors.Join(callbackErr, invokeShutdownCallback(fn))
	}
	// 创建超时上下文
	shutdownCtx, cancel := context.WithTimeout(ctx, g.shutdownTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- g.manager.Stop()
	}()
	select {
	case err := <-done:
		g.manager.logger.Log("[Asynq] all tasks completed")
		return errors.Join(callbackErr, err)
	case <-shutdownCtx.Done():
		g.manager.logger.Error("[Asynq] shutdown timeout, forcing stop")
		return errors.Join(callbackErr, shutdownCtx.Err())
	}
}

func invokeShutdownCallback(fn func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = callbackPanicError("shutdown callback", recovered)
		}
	}()
	fn()
	return nil
}

// =========================================
// 队列监控
// =========================================

// QueueStats 队列统计
type QueueStats struct {
	Name      string `json:"name"`
	Pending   int    `json:"pending"`
	Active    int    `json:"active"`
	Scheduled int    `json:"scheduled"`
	Retry     int    `json:"retry"`
	Archived  int    `json:"archived"`
	Completed int    `json:"completed"`
}

// GetQueueStats 获取所有队列统计
func GetQueueStats() ([]QueueStats, error) {
	m := GetManager()
	if m == nil {
		return nil, ErrManagerNotInitialized
	}
	queues := m.QueueNames()
	stats := make([]QueueStats, 0, len(queues))
	err := m.withInspector(func(inspector *asynq.Inspector) error {
		for _, q := range queues {
			info, err := configuredQueueInfo(inspector, q)
			if err != nil {
				return fmt.Errorf("inspect queue %q: %w", q, err)
			}
			stats = append(stats, QueueStats{
				Name:      q,
				Pending:   info.Pending,
				Active:    info.Active,
				Scheduled: info.Scheduled,
				Retry:     info.Retry,
				Archived:  info.Archived,
				Completed: info.Completed,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func configuredQueueInfo(inspector *asynq.Inspector, queue string) (*asynq.QueueInfo, error) {
	info, err := inspector.GetQueueInfo(queue)
	if err == nil {
		return info, nil
	}
	_, probeErr := inspector.ListPendingTasks(queue, asynq.PageSize(1))
	if errors.Is(probeErr, asynq.ErrQueueNotFound) {
		return &asynq.QueueInfo{Queue: queue}, nil
	}
	if probeErr != nil {
		return nil, errors.Join(err, probeErr)
	}
	return inspector.GetQueueInfo(queue)
}

// GetDeadLetterTasks 获取死信队列任务
func GetDeadLetterTasks(queue string, limit int) ([]*asynq.TaskInfo, error) {
	m := GetManager()
	if m == nil {
		return nil, ErrManagerNotInitialized
	}
	var tasks []*asynq.TaskInfo
	err := m.withInspector(func(inspector *asynq.Inspector) error {
		actual, err := m.resolveQueue(queue)
		if err != nil {
			return err
		}
		tasks, err = inspector.ListArchivedTasks(actual, asynq.PageSize(limit))
		if errors.Is(err, asynq.ErrQueueNotFound) {
			tasks = []*asynq.TaskInfo{}
			return nil
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// RetryDeadLetterTask 重试死信任务
func RetryDeadLetterTask(queue, taskID string) error {
	m := GetManager()
	if m == nil {
		return ErrManagerNotInitialized
	}
	return m.withInspector(func(inspector *asynq.Inspector) error {
		actual, err := m.resolveQueue(queue)
		if err != nil {
			return err
		}
		return inspector.RunTask(actual, taskID)
	})
}

// DeleteDeadLetterTask 删除死信任务
func DeleteDeadLetterTask(queue, taskID string) error {
	m := GetManager()
	if m == nil {
		return ErrManagerNotInitialized
	}
	return m.withInspector(func(inspector *asynq.Inspector) error {
		actual, err := m.resolveQueue(queue)
		if err != nil {
			return err
		}
		return inspector.DeleteTask(actual, taskID)
	})
}

// =========================================
// 便捷函数
// =========================================

// GetHealthChecker 获取健康检查器
func GetHealthChecker() *HealthChecker {
	m := GetManager()
	if m == nil {
		return nil
	}
	return NewHealthChecker(m)
}

// SetupGracefulShutdown 配置优雅关闭
func SetupGracefulShutdown(timeout time.Duration) *GracefulShutdown {
	m := GetManager()
	if m == nil {
		return nil
	}
	return NewGracefulShutdown(m, timeout)
}
