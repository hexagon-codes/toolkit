package asynq

import "errors"

// =========================================
// 错误定义
// =========================================
var (
	// ErrManagerNotInitialized 管理器未初始化
	ErrManagerNotInitialized = errors.New("asynq manager not initialized")
	// ErrInvalidConfig reports invalid queue runtime configuration.
	ErrInvalidConfig = errors.New("asynq invalid config")
	// ErrInvalidContext reports a nil context passed to a blocking operation.
	ErrInvalidContext = errors.New("asynq invalid context")
	// ErrInvalidTask reports a nil task passed for enqueue.
	ErrInvalidTask = errors.New("asynq invalid task")
	// ErrManagerAlreadyInitialized reports an explicit duplicate global init.
	ErrManagerAlreadyInitialized = errors.New("asynq manager already initialized")
	// ErrManagerStarted reports a mutation that is unsafe after runtime startup.
	ErrManagerStarted = errors.New("asynq manager already started")
	// ErrBackpressureRunning reports an unsafe live configuration update.
	ErrBackpressureRunning = errors.New("asynq backpressure controller running")
	// ErrManagerStopped reports use after the manager has released its resources.
	ErrManagerStopped = errors.New("asynq manager stopped")
	// ErrRedisClientUnavailable reports that lease safety cannot be established.
	ErrRedisClientUnavailable = errors.New("asynq redis client unavailable")
	// ErrInvalidLease reports an invalid lease request.
	ErrInvalidLease = errors.New("asynq invalid lease")
	// ErrLeaseLost reports that a caller no longer owns the Redis lease.
	ErrLeaseLost = errors.New("asynq lease lost")
	// ErrTaskNotFound 任务未找到
	ErrTaskNotFound = errors.New("task not found")
	// ErrInvalidPayload 无效的载荷
	ErrInvalidPayload = errors.New("invalid payload")
	// ErrQueueNotFound 队列未找到
	ErrQueueNotFound = errors.New("queue not found")
	// ErrHandlerNotRegistered 处理器未注册
	ErrHandlerNotRegistered = errors.New("handler not registered")
)
