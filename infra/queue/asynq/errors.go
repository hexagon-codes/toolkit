package asynq

import "errors"

// =========================================
// 错误定义
// =========================================
var (
	// ErrManagerNotInitialized 管理器未初始化
	ErrManagerNotInitialized = errors.New("asynq manager not initialized")
	// ErrTaskNotFound 任务未找到
	ErrTaskNotFound = errors.New("task not found")
	// ErrInvalidPayload 无效的载荷
	ErrInvalidPayload = errors.New("invalid payload")
	// ErrQueueNotFound 队列未找到
	ErrQueueNotFound = errors.New("queue not found")
	// ErrHandlerNotRegistered 处理器未注册
	ErrHandlerNotRegistered = errors.New("handler not registered")
)
