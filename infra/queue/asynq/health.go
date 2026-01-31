package asynq

import (
	"context"
	"fmt"
	"github.com/hibiken/asynq"
	"sync/atomic"
	"time"
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
	manager    *Manager
	lastStatus atomic.Value // *HealthStatus
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(m *Manager) *HealthChecker {
	return &HealthChecker{
		manager: m,
	}
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
		h.lastStatus.Store(status)
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
	if err := h.checkQueues(ctx, status); err != nil {
		status.Details["queues"] = err.Error()
	}
	h.lastStatus.Store(status)
	return status
}

// checkRedis 检查 Redis 连接
func (h *HealthChecker) checkRedis(ctx context.Context) error {
	// 尝试获取队列信息来验证连接
	inspector := h.manager.GetInspector()
	_, err := inspector.Queues()
	return err
}

// checkQueues 检查队列状态
func (h *HealthChecker) checkQueues(ctx context.Context, status *HealthStatus) error {
	inspector := h.manager.GetInspector()
	queues, err := inspector.Queues()
	if err != nil {
		return err
	}
	for _, q := range queues {
		info, err := inspector.GetQueueInfo(q)
		if err != nil {
			status.Details[fmt.Sprintf("queue_%s", q)] = err.Error()
			continue
		}
		// 检查队列积压
		if info.Pending > 10000 {
			status.Details[fmt.Sprintf("queue_%s", q)] = fmt.Sprintf("high_backlog: %d", info.Pending)
		} else {
			status.Details[fmt.Sprintf("queue_%s", q)] = fmt.Sprintf("pending=%d, active=%d", info.Pending, info.Active)
		}
	}
	return nil
}

// IsHealthy 是否健康（用于 liveness 探针）
func (h *HealthChecker) IsHealthy() bool {
	if v := h.lastStatus.Load(); v != nil {
		return v.(*HealthStatus).Healthy
	}
	return false
}

// IsReady 是否就绪（用于 readiness 探针）
func (h *HealthChecker) IsReady() bool {
	if v := h.lastStatus.Load(); v != nil {
		return v.(*HealthStatus).Ready
	}
	return false
}

// GetLastStatus 获取最后一次检查状态
func (h *HealthChecker) GetLastStatus() *HealthStatus {
	if v := h.lastStatus.Load(); v != nil {
		return v.(*HealthStatus)
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
	g.manager.logger.Log("[Asynq] graceful shutdown initiated...")
	// 执行注册的回调
	for _, fn := range g.onShutdown {
		fn()
	}
	// 创建超时上下文
	shutdownCtx, cancel := context.WithTimeout(ctx, g.shutdownTimeout)
	defer cancel()
	// 停止调度器（停止产生新任务）
	if g.manager.scheduler != nil {
		g.manager.logger.Log("[Asynq] stopping scheduler...")
		g.manager.scheduler.Shutdown()
	}
	// 等待正在处理的任务完成
	done := make(chan struct{})
	go func() {
		if g.manager.server != nil {
			g.manager.logger.Log("[Asynq] waiting for active tasks to complete...")
			g.manager.server.Shutdown()
		}
		close(done)
	}()
	select {
	case <-done:
		g.manager.logger.Log("[Asynq] all tasks completed")
	case <-shutdownCtx.Done():
		g.manager.logger.Error("[Asynq] shutdown timeout, forcing stop")
	}
	// 关闭客户端
	if g.manager.client != nil {
		g.manager.client.Close()
	}
	g.manager.logger.Log("[Asynq] graceful shutdown completed")
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
	inspector := m.GetInspector() // 复用 Inspector
	queues, err := inspector.Queues()
	if err != nil {
		return nil, err
	}
	stats := make([]QueueStats, 0, len(queues))
	for _, q := range queues {
		info, err := inspector.GetQueueInfo(q)
		if err != nil {
			continue
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
	return stats, nil
}

// GetDeadLetterTasks 获取死信队列任务
func GetDeadLetterTasks(queue string, limit int) ([]*asynq.TaskInfo, error) {
	m := GetManager()
	if m == nil {
		return nil, ErrManagerNotInitialized
	}
	inspector := m.GetInspector() // 复用 Inspector
	tasks, err := inspector.ListArchivedTasks(queue, asynq.PageSize(limit))
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
	inspector := m.GetInspector() // 复用 Inspector
	return inspector.RunTask(queue, taskID)
}

// DeleteDeadLetterTask 删除死信任务
func DeleteDeadLetterTask(queue, taskID string) error {
	m := GetManager()
	if m == nil {
		return ErrManagerNotInitialized
	}
	inspector := m.GetInspector() // 复用 Inspector
	return inspector.DeleteTask(queue, taskID)
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
