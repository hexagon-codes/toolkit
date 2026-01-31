//go:build test || testing

package asynq

import "time"

// =========================================
// 测试辅助函数（仅供测试使用）
// 导出内部结构以便外部测试包访问
// =========================================
// GetHandlerCount 获取已注册的处理器数量（测试用）
func (m *Manager) GetHandlerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.handlers)
}

// GetScheduleCount 获取已注册的定时任务数量（测试用）
func (m *Manager) GetScheduleCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.schedules)
}

// GetFirstScheduleCronspec 获取第一个定时任务的 cronspec（测试用）
func (m *Manager) GetFirstScheduleCronspec() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.schedules) > 0 {
		return m.schedules[0].Cronspec
	}
	return ""
}

// NewMetricsWithMaxSamples 创建指定 maxSamples 的 Metrics（测试用）
func NewMetricsWithMaxSamples(maxSamples int) *Metrics {
	return &Metrics{
		processed:  make(map[string]int64),
		succeeded:  make(map[string]int64),
		failed:     make(map[string]int64),
		durations:  make(map[string]*ringBuffer),
		maxSamples: maxSamples,
	}
}

// GetSampleCount 获取指定任务类型的样本数量（测试用）
func (m *Metrics) GetSampleCount(taskType string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if rb := m.durations[taskType]; rb != nil {
		return rb.Count()
	}
	return 0
}

// GetProcessedCount 获取指定任务类型的处理数量（测试用）
func (m *Metrics) GetProcessedCount(taskType string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.processed[taskType]
}

// GetSucceededCount 获取指定任务类型的成功数量（测试用）
func (m *Metrics) GetSucceededCount(taskType string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.succeeded[taskType]
}

// GetFailedCount 获取指定任务类型的失败数量（测试用）
func (m *Metrics) GetFailedCount(taskType string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.failed[taskType]
}

// GetAverageDuration 获取指定任务类型的平均耗时（测试用）
func (m *Metrics) GetAverageDuration(taskType string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if rb := m.durations[taskType]; rb != nil {
		return rb.Average()
	}
	return 0
}
