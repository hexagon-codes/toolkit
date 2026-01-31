package asynq

import (
	"encoding/json"
	"fmt"
	"github.com/hibiken/asynq"
)

// =========================================
// 适配器和辅助函数
// =========================================
// InitManagerFromConfig 从配置提供者初始化管理器
// 这是推荐的初始化方式
func InitManagerFromConfig(configProvider ConfigProvider) (*Manager, error) {
	// 检查 Redis 是否启用
	if !configProvider.IsRedisEnabled() {
		return nil, fmt.Errorf("redis not enabled, cannot initialize asynq")
	}
	config := &Config{
		RedisAddrs:  configProvider.GetRedisAddrs(),
		Password:    configProvider.GetRedisPassword(),
		Username:    configProvider.GetRedisUsername(),
		Concurrency: configProvider.GetConcurrency(),
		Queues:      DefaultQueues(),
		LogLevel:    asynq.InfoLevel,
	}
	return InitManager(config)
}

// =========================================
// 任务注册辅助函数
// =========================================
// RegisterTaskHandler 注册任务处理器（使用全局管理器）
func RegisterTaskHandler(taskType string, handler asynq.HandlerFunc) error {
	m := GetManager()
	if m == nil {
		return ErrManagerNotInitialized
	}
	m.RegisterHandler(taskType, handler)
	return nil
}

// RegisterScheduledTask 注册定时任务（使用全局管理器）
func RegisterScheduledTask(cronspec string, taskType string, payload interface{}, opts ...asynq.Option) error {
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
	m.RegisterSchedule(cronspec, task, opts...)
	return nil
}

// =========================================
// 监控辅助
// =========================================
// GetStats 获取统计信息
func GetStats() map[string]interface{} {
	m := GetManager()
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]interface{}{
		"started":     m.started,
		"handlers":    len(m.handlers),
		"schedules":   len(m.schedules),
		"concurrency": m.config.Concurrency,
		"queues":      m.config.Queues,
	}
}
