package asynq

import (
	"context"
	"errors"
	"fmt"
	"time"

	hibasynq "github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// =========================================
// Prometheus Metrics 定义
// 用于 Grafana 监控和告警
// =========================================
var (
	// 队列长度（按队列名分组）
	// 示例: asynq_queue_size{queue="scheduled"} 150
	QueueSizeGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "asynq_queue_size",
			Help: "Current number of tasks in each queue (pending + active + scheduled + retry)",
		},
		[]string{"queue"},
	)
	// 活跃任务数（正在处理的任务）
	ActiveTasksGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "asynq_active_tasks",
			Help: "Number of tasks currently being processed",
		},
		[]string{"queue"},
	)
	// 待处理任务数（等待中的任务）
	PendingTasksGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "asynq_pending_tasks",
			Help: "Number of tasks waiting to be processed",
		},
		[]string{"queue"},
	)
	// 调度任务数（延迟执行的任务）
	ScheduledTasksGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "asynq_scheduled_tasks",
			Help: "Number of tasks scheduled for future processing",
		},
		[]string{"queue"},
	)
	// 重试任务数（失败后等待重试的任务）
	RetryTasksGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "asynq_retry_tasks",
			Help: "Number of tasks waiting to be retried",
		},
		[]string{"queue"},
	)
	// 失败任务数（Dead Letter Queue）
	DeadTasksGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "asynq_dead_tasks",
			Help: "Number of tasks in dead letter queue (exhausted retries)",
		},
		[]string{"queue"},
	)
	// 任务处理总数（按类型和状态分组）
	// 示例: asynq_tasks_processed_total{type="task:poll", status="success"} 1234
	TasksProcessedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "asynq_tasks_processed_total",
			Help: "Total number of tasks processed by type and result status",
		},
		[]string{"type", "status"}, // status: success, failed, retry
	)
	// 任务处理延迟（按任务类型分组）
	// 示例: asynq_task_duration_seconds{type="task:poll"}
	TaskDurationHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "asynq_task_duration_seconds",
			Help:    "Task processing duration in seconds by task type",
			Buckets: []float64{.1, .5, 1, 2, 5, 10, 30, 60, 120, 300}, // 0.1s ~ 5min
		},
		[]string{"type"},
	)
	// Worker 并发数（当前活跃的 Worker 数量）
	ActiveWorkersGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "asynq_active_workers",
			Help: "Current number of active workers processing tasks",
		},
	)
	// Asynq 系统健康状态（1=健康，0=不健康）
	SystemHealthGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "asynq_system_health",
			Help: "Asynq system health status (1=healthy, 0=unhealthy)",
		},
	)
	// 任务入队总数（按队列分组）
	TasksEnqueuedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "asynq_tasks_enqueued_total",
			Help: "Total number of tasks enqueued into each queue",
		},
		[]string{"queue", "type"},
	)
	// 任务等待时间（从入队到开始处理的延迟）
	TaskLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "asynq_task_latency_seconds",
			Help:    "Task latency from enqueue to start processing",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600}, // 1s ~ 1h
		},
		[]string{"queue"},
	)
)

// InitMetrics 初始化所有指标（确保在 /metrics 中显示，即使值为 0）。
// 在 StartMetricsUpdater 启动时调用一次
func InitMetrics(queues []string) {
	if len(queues) == 0 {
		queues = []string{QueueCritical, QueueHigh, QueueDefault, QueueScheduled, QueueLow, QueueDeadLetter}
	}
	for _, queue := range queues {
		QueueSizeGauge.WithLabelValues(queue).Set(0)
		ActiveTasksGauge.WithLabelValues(queue).Set(0)
		PendingTasksGauge.WithLabelValues(queue).Set(0)
		ScheduledTasksGauge.WithLabelValues(queue).Set(0)
		RetryTasksGauge.WithLabelValues(queue).Set(0)
		DeadTasksGauge.WithLabelValues(queue).Set(0)
	}
	// 初始化系统状态 Gauge
	SystemHealthGauge.Set(0)
	ActiveWorkersGauge.Set(0)
	// 初始化 Counter 和 Histogram（使用占位标签，确保指标在 /metrics 中可见）
	// 注意：Counter/Histogram 初始化后值为 0，不影响实际统计
	taskTypes := []string{TaskTypeTaskProcess, TaskTypeTaskCallback}
	statuses := []string{"success", "failed", "retry"}
	for _, taskType := range taskTypes {
		for _, status := range statuses {
			TasksProcessedCounter.WithLabelValues(taskType, status)
		}
		TaskDurationHistogram.WithLabelValues(taskType)
	}
	// 初始化入队计数器
	for _, queue := range queues {
		for _, taskType := range taskTypes {
			TasksEnqueuedCounter.WithLabelValues(queue, taskType)
		}
		// 初始化等待时间 Histogram
		TaskLatencyHistogram.WithLabelValues(queue)
	}
}

// UpdateQueueMetrics 更新队列相关的 metrics（从 Inspector 获取数据）
// 应该被定时任务定期调用（如每 15 秒）
func UpdateQueueMetrics(ctx context.Context, manager *Manager) error {
	if ctx == nil {
		return fmt.Errorf("%w: update metrics requires a non-nil context", ErrInvalidContext)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if manager == nil {
		SystemHealthGauge.Set(0)
		ActiveWorkersGauge.Set(0)
		return ErrManagerNotInitialized
	}
	queues := manager.QueueNames()
	err := manager.withInspector(func(inspector *hibasynq.Inspector) error {
		// 更新活跃 Worker 数量，只统计处理本 Manager 队列的 worker。
		servers, err := inspector.Servers()
		if err != nil {
			return err
		}
		totalActiveWorkers := 0
		for _, server := range servers {
			for _, worker := range server.ActiveWorkers {
				if _, configured := manager.config.Queues[worker.Queue]; configured {
					totalActiveWorkers++
				}
			}
		}
		ActiveWorkersGauge.Set(float64(totalActiveWorkers))
		for _, queue := range queues {
			info, err := configuredQueueInfo(inspector, queue)
			if err != nil {
				return fmt.Errorf("inspect queue %q: %w", queue, err)
			}
			totalSize := info.Active + info.Pending + info.Scheduled + info.Retry
			QueueSizeGauge.WithLabelValues(queue).Set(float64(totalSize))
			ActiveTasksGauge.WithLabelValues(queue).Set(float64(info.Active))
			PendingTasksGauge.WithLabelValues(queue).Set(float64(info.Pending))
			ScheduledTasksGauge.WithLabelValues(queue).Set(float64(info.Scheduled))
			RetryTasksGauge.WithLabelValues(queue).Set(float64(info.Retry))
			archivedInfo, err := inspector.ListArchivedTasks(queue)
			if errors.Is(err, hibasynq.ErrQueueNotFound) {
				DeadTasksGauge.WithLabelValues(queue).Set(0)
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect archived queue %q: %w", queue, err)
			}
			DeadTasksGauge.WithLabelValues(queue).Set(float64(len(archivedInfo)))
		}
		return nil
	})
	if err != nil {
		SystemHealthGauge.Set(0)
		ActiveWorkersGauge.Set(0)
		return err
	}
	SystemHealthGauge.Set(1)
	return nil
}

// RecordTaskProcessed 记录任务处理结果（在 Worker Handler 中调用）
// taskType: 任务类型（如 "task:poll", "task:retry"）
// status: 处理状态（"success", "failed", "retry"）
func RecordTaskProcessed(taskType, status string) {
	TasksProcessedCounter.WithLabelValues(taskType, status).Inc()
}

// RecordTaskDuration 记录任务处理耗时（在 Worker Handler 中调用）
// taskType: 任务类型
// duration: 处理耗时
func RecordTaskDuration(taskType string, duration time.Duration) {
	TaskDurationHistogram.WithLabelValues(taskType).Observe(duration.Seconds())
}

// RecordTaskEnqueued 记录任务入队（在 Enqueue 时调用）
// queue: 队列名称（如 "scheduled", "high"）
// taskType: 任务类型
func RecordTaskEnqueued(queue, taskType string) {
	TasksEnqueuedCounter.WithLabelValues(queue, taskType).Inc()
}

// RecordTaskLatency 记录任务等待时间（从入队到开始处理）
// queue: 队列名称
// latency: 等待时间
func RecordTaskLatency(queue string, latency time.Duration) {
	TaskLatencyHistogram.WithLabelValues(queue).Observe(latency.Seconds())
}

// UpdateActiveWorkers 更新活跃 Worker 数量
// count: 当前活跃的 Worker 数量
func UpdateActiveWorkers(count int) {
	ActiveWorkersGauge.Set(float64(count))
}

// StartMetricsUpdater 启动定时更新 Metrics 的后台任务
// interval: 更新间隔（推荐 15 秒）
func StartMetricsUpdater(ctx context.Context, manager *Manager, interval time.Duration) error {
	if ctx == nil {
		return fmt.Errorf("%w: start metrics updater requires a non-nil context", ErrInvalidContext)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if interval <= 0 {
		return fmt.Errorf("%w: metrics interval must be positive", ErrInvalidConfig)
	}
	// 初始化所有指标（确保在 /metrics 中可见，即使没有数据）
	if manager != nil {
		InitMetrics(manager.QueueNames())
	} else {
		InitMetrics(nil)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// 立即更新一次
	if err := UpdateQueueMetrics(ctx, manager); err != nil {
		if errors.Is(err, ErrManagerStopped) || errors.Is(err, ErrManagerNotInitialized) {
			return err
		}
		GetLogger().Error(fmt.Sprintf("[Asynq] update queue metrics: %v", err))
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := UpdateQueueMetrics(ctx, manager); err != nil {
				if errors.Is(err, ErrManagerStopped) {
					return err
				}
				GetLogger().Error(fmt.Sprintf("[Asynq] update queue metrics: %v", err))
			}
		}
	}
}
