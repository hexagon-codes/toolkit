package asynq

import "time"

// =========================================
// 通用任务类型定义
// 使用者应该在自己的项目中定义具体的 Payload 结构
// =========================================

// TaskPayload 通用任务载荷接口
// 业务方应该实现自己的 Payload 结构
type TaskPayload interface {
	GetTaskID() string
}

// TaskConfig 通用任务配置
type TaskConfig struct {
	InitialDelay    time.Duration // 初始延迟
	MaxDelay        time.Duration // 最大延迟
	MaxRetry        int           // 最大重试次数
	Timeout         time.Duration // 超时时间
	SupportsWebhook bool          // 是否支持 Webhook
	BatchSupported  bool          // 是否支持批量处理
	MaxBatchSize    int           // 最大批量大小
}

// DefaultTaskConfig 默认任务配置
func DefaultTaskConfig() TaskConfig {
	return TaskConfig{
		InitialDelay:    3 * time.Second,
		MaxDelay:        10 * time.Second,
		MaxRetry:        3,
		Timeout:         60 * time.Second,
		SupportsWebhook: false,
		BatchSupported:  false,
		MaxBatchSize:    0,
	}
}

// CalculateBackoff 计算退避延迟（指数退避）
func CalculateBackoff(retryCount int, initialDelay, maxDelay time.Duration) time.Duration {
	if retryCount == 0 {
		return initialDelay
	}
	delay := initialDelay * time.Duration(1<<uint(retryCount))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
