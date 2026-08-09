package asynq

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// RegisterTaskHandler 注册任务处理器（使用全局管理器）
func RegisterTaskHandler(taskType string, handler asynq.HandlerFunc) error {
	m := GetManager()
	if m == nil {
		return ErrManagerNotInitialized
	}
	return m.RegisterHandler(taskType, handler)
}

// RegisterScheduledTask 注册定时任务（使用全局管理器）
func RegisterScheduledTask(cronspec, taskType string, payload interface{}, opts ...asynq.Option) error {
	m := GetManager()
	if m == nil {
		return ErrManagerNotInitialized
	}
	var data []byte
	if payload != nil {
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	task := asynq.NewTask(taskType, data)
	return m.RegisterSchedule(cronspec, task, opts...)
}

// GetStats 获取统计信息
func GetStats() map[string]interface{} {
	m := GetManager()
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	queues := make(map[string]int, len(m.config.Queues))
	for queue, priority := range m.config.Queues {
		queues[queue] = priority
	}
	return map[string]interface{}{
		"started":     m.started,
		"handlers":    len(m.handlers),
		"schedules":   len(m.schedules),
		"concurrency": m.config.Concurrency,
		"queues":      queues,
	}
}
