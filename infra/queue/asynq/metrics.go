package asynq

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"
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

// =========================================
// Metrics 初始化函数
// =========================================
// InitMetrics 初始化所有指标（确保在 /metrics 中显示，即使值为 0）
// 在 StartMetricsUpdater 启动时调用一次
func InitMetrics() {
	// 初始化队列相关 Gauge 指标（所有队列）
	queues := []string{QueueCritical, QueueHigh, QueueDefault, QueueScheduled, QueueLow, QueueDeadLetter}
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

// =========================================
// Metrics 更新函数
// =========================================
// UpdateQueueMetrics 更新队列相关的 metrics（从 Inspector 获取数据）
// 应该被定时任务定期调用（如每 15 秒）
func UpdateQueueMetrics(ctx context.Context) error {
	inspector := GetInspector()
	if inspector == nil {
		SystemHealthGauge.Set(0) // 系统不健康
		ActiveWorkersGauge.Set(0)
		return nil
	}
	// 获取所有队列信息
	queues, err := inspector.Queues()
	if err != nil {
		SystemHealthGauge.Set(0)
		ActiveWorkersGauge.Set(0)
		return err
	}
	SystemHealthGauge.Set(1) // 系统健康
	// 更新活跃 Worker 数量（从所有服务器汇总）
	servers, err := inspector.Servers()
	if err == nil {
		totalActiveWorkers := 0
		for _, srv := range servers {
			// ActiveWorkers 是 []*WorkerInfo 类型，使用 len() 获取数量
			totalActiveWorkers += len(srv.ActiveWorkers)
		}
		ActiveWorkersGauge.Set(float64(totalActiveWorkers))
	}
	// 更新每个队列的指标
	for _, queue := range queues {
		info, err := inspector.GetQueueInfo(queue)
		if err != nil {
			continue
		}
		// 队列总大小
		totalSize := info.Active + info.Pending + info.Scheduled + info.Retry
		QueueSizeGauge.WithLabelValues(queue).Set(float64(totalSize))
		// 各状态任务数
		ActiveTasksGauge.WithLabelValues(queue).Set(float64(info.Active))
		PendingTasksGauge.WithLabelValues(queue).Set(float64(info.Pending))
		ScheduledTasksGauge.WithLabelValues(queue).Set(float64(info.Scheduled))
		RetryTasksGauge.WithLabelValues(queue).Set(float64(info.Retry))
		// Dead Letter Queue（按队列分组，使用 Archived 表示）
		// 注意：asynq v0.24+ 将 Dead 改名为 Archived
		archivedInfo, err := inspector.ListArchivedTasks(queue)
		if err == nil {
			DeadTasksGauge.WithLabelValues(queue).Set(float64(len(archivedInfo)))
		}
	}
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

// =========================================
// 启动 Metrics 定时更新器
// =========================================
// StartMetricsUpdater 启动定时更新 Metrics 的后台任务
// interval: 更新间隔（推荐 15 秒）
func StartMetricsUpdater(ctx context.Context, interval time.Duration) {
	// 初始化所有指标（确保在 /metrics 中可见，即使没有数据）
	InitMetrics()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// 立即更新一次
	UpdateQueueMetrics(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			UpdateQueueMetrics(ctx)
		}
	}
}
