package asynq

import (
	"errors"
	"fmt"
)

// =========================================
// 错误定义
// =========================================
var (
	// ErrManagerNotInitialized 管理器未初始化
	ErrManagerNotInitialized = errors.New("asynq manager not initialized")
	// ErrInvalidConfig 表示队列运行配置无效。
	ErrInvalidConfig = errors.New("asynq invalid config")
	// ErrInvalidContext 表示阻塞操作收到空 context。
	ErrInvalidContext = errors.New("asynq invalid context")
	// ErrInvalidTask 表示入队任务为空。
	ErrInvalidTask = errors.New("asynq invalid task")
	// ErrManagerAlreadyInitialized 表示显式重复初始化全局管理器。
	ErrManagerAlreadyInitialized = errors.New("asynq manager already initialized")
	// ErrManagerStarted 表示运行时启动后执行了不安全的配置变更。
	ErrManagerStarted = errors.New("asynq manager already started")
	// ErrBackpressureRunning 表示背压控制器运行时执行了不安全的配置更新。
	ErrBackpressureRunning = errors.New("asynq backpressure controller running")
	// ErrManagerStopped 表示管理器释放资源后仍被使用。
	ErrManagerStopped = errors.New("asynq manager stopped")
	// ErrRedisClientUnavailable 表示无法建立租约所需的 Redis 安全条件。
	ErrRedisClientUnavailable = errors.New("asynq redis client unavailable")
	// ErrInvalidLease 表示租约请求无效。
	ErrInvalidLease = errors.New("asynq invalid lease")
	// ErrLeaseLost 表示调用方已不再持有 Redis 租约。
	ErrLeaseLost = errors.New("asynq lease lost")
	// ErrTaskNotFound 任务未找到
	ErrTaskNotFound = errors.New("task not found")
	// ErrInvalidPayload 无效的载荷
	ErrInvalidPayload = errors.New("invalid payload")
	// ErrQueueNotFound 队列未找到
	ErrQueueNotFound = errors.New("queue not found")
	// ErrHandlerNotRegistered 处理器未注册
	ErrHandlerNotRegistered = errors.New("handler not registered")
	// ErrInvalidHandler 表示处理器类型或函数无效。
	ErrInvalidHandler = errors.New("asynq invalid handler")
	// ErrHandlerAlreadyRegistered 表示任务类型已经绑定处理器。
	ErrHandlerAlreadyRegistered = errors.New("asynq handler already registered")
	// ErrInvalidMiddleware 表示中间件函数或其返回值无效。
	ErrInvalidMiddleware = errors.New("asynq invalid middleware")
)

func callbackPanicError(scope string, recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("asynq: %s panicked: %w", scope, err)
	}
	return fmt.Errorf("asynq: %s panicked: %v", scope, recovered)
}
