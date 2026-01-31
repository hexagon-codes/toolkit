package poolx

import (
	"sync"
	"time"
)

// ============================================================================
// Hook Types
// ============================================================================

// HookType defines the type of lifecycle hook
type HookType int

const (
	// HookBeforeSubmit is called before a task is submitted
	HookBeforeSubmit HookType = iota
	// HookAfterSubmit is called after a task is successfully submitted
	HookAfterSubmit
	// HookBeforeTask is called before a task starts executing
	HookBeforeTask
	// HookAfterTask is called after a task finishes executing (success or panic)
	HookAfterTask
	// HookOnPanic is called when a task panics
	HookOnPanic
	// HookOnReject is called when a task is rejected
	HookOnReject
	// HookOnTimeout is called when a task times out
	HookOnTimeout
	// HookOnWorkerStart is called when a worker starts
	HookOnWorkerStart
	// HookOnWorkerStop is called when a worker stops
	HookOnWorkerStop
	// HookOnScaleUp is called when the pool scales up
	HookOnScaleUp
	// HookOnScaleDown is called when the pool scales down
	HookOnScaleDown
)

// String returns the string representation of the hook type
func (h HookType) String() string {
	switch h {
	case HookBeforeSubmit:
		return "BeforeSubmit"
	case HookAfterSubmit:
		return "AfterSubmit"
	case HookBeforeTask:
		return "BeforeTask"
	case HookAfterTask:
		return "AfterTask"
	case HookOnPanic:
		return "OnPanic"
	case HookOnReject:
		return "OnReject"
	case HookOnTimeout:
		return "OnTimeout"
	case HookOnWorkerStart:
		return "OnWorkerStart"
	case HookOnWorkerStop:
		return "OnWorkerStop"
	case HookOnScaleUp:
		return "OnScaleUp"
	case HookOnScaleDown:
		return "OnScaleDown"
	default:
		return "Unknown"
	}
}

// ============================================================================
// Hook Data Types
// ============================================================================

// TaskInfo contains information about a task for hooks
type TaskInfo struct {
	ID          uint64        // Task unique ID
	PoolName    string        // Pool name
	WorkerID    int32         // Worker ID that executed the task (-1 if not assigned)
	Priority    int           // Task priority
	SubmittedAt time.Time     // When the task was submitted
	StartedAt   time.Time     // When execution started (zero if not started)
	FinishedAt  time.Time     // When execution finished (zero if not finished)
	WaitTime    time.Duration // Time spent waiting in queue
	ExecTime    time.Duration // Time spent executing
	Error       any           // Error or panic value
	Timeout     time.Duration // Task timeout (zero means no timeout)
}

// WorkerInfo contains information about a worker for hooks
type WorkerInfo struct {
	ID         int32         // Worker ID
	PoolName   string        // Pool name
	StartedAt  time.Time     // When the worker started
	StoppedAt  time.Time     // When the worker stopped (zero if running)
	TasksRun   int64         // Number of tasks run by this worker
	LastActive time.Time     // Last activity time
	IdleTime   time.Duration // Current idle time
}

// ScaleInfo contains information about scaling events
type ScaleInfo struct {
	PoolName   string    // Pool name
	OldSize    int32     // Previous pool size
	NewSize    int32     // New pool size
	Reason     string    // Reason for scaling
	LoadFactor float64   // Current load factor
	ScaledAt   time.Time // When scaling occurred
}

// ============================================================================
// Hook Function Types
// ============================================================================

// HookFunc is the type for hook callback functions
type HookFunc func(hookType HookType, data any)

// TypedTaskHook is a typed hook for task-related events
type TypedTaskHook func(info *TaskInfo)

// TypedWorkerHook is a typed hook for worker-related events
type TypedWorkerHook func(info *WorkerInfo)

// TypedScaleHook is a typed hook for scaling events
type TypedScaleHook func(info *ScaleInfo)

// ============================================================================
// Hooks Manager
// ============================================================================

// Hooks manages lifecycle callbacks for the pool
type Hooks struct {
	mu    sync.RWMutex
	hooks map[HookType][]HookFunc
}

// NewHooks creates a new Hooks manager
func NewHooks() *Hooks {
	return &Hooks{
		hooks: make(map[HookType][]HookFunc),
	}
}

// Register adds a hook callback for the specified hook type
func (h *Hooks) Register(hookType HookType, fn HookFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hooks[hookType] = append(h.hooks[hookType], fn)
}

// RegisterTask registers a typed hook for task-related events
func (h *Hooks) RegisterTask(hookType HookType, fn TypedTaskHook) {
	h.Register(hookType, func(_ HookType, data any) {
		if info, ok := data.(*TaskInfo); ok {
			fn(info)
		}
	})
}

// RegisterWorker registers a typed hook for worker-related events
func (h *Hooks) RegisterWorker(hookType HookType, fn TypedWorkerHook) {
	h.Register(hookType, func(_ HookType, data any) {
		if info, ok := data.(*WorkerInfo); ok {
			fn(info)
		}
	})
}

// RegisterScale registers a typed hook for scaling events
func (h *Hooks) RegisterScale(hookType HookType, fn TypedScaleHook) {
	h.Register(hookType, func(_ HookType, data any) {
		if info, ok := data.(*ScaleInfo); ok {
			fn(info)
		}
	})
}

// Unregister removes all hooks for the specified hook type
func (h *Hooks) Unregister(hookType HookType) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.hooks, hookType)
}

// UnregisterAll removes all hooks
func (h *Hooks) UnregisterAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hooks = make(map[HookType][]HookFunc)
}

// Trigger calls all hooks registered for the specified hook type
func (h *Hooks) Trigger(hookType HookType, data any) {
	h.mu.RLock()
	hooks := h.hooks[hookType]
	h.mu.RUnlock()

	for _, fn := range hooks {
		// Call hook in a safe manner
		h.safeCall(hookType, fn, data)
	}
}

// TriggerAsync calls all hooks asynchronously
func (h *Hooks) TriggerAsync(hookType HookType, data any) {
	h.mu.RLock()
	hooks := h.hooks[hookType]
	h.mu.RUnlock()

	for _, fn := range hooks {
		go h.safeCall(hookType, fn, data)
	}
}

// safeCall calls a hook function with panic recovery
func (h *Hooks) safeCall(hookType HookType, fn HookFunc, data any) {
	defer func() {
		if r := recover(); r != nil {
			// Hook panicked, log but don't propagate
			// This prevents hooks from crashing the pool
		}
	}()
	fn(hookType, data)
}

// HasHooks returns true if any hooks are registered for the specified type
func (h *Hooks) HasHooks(hookType HookType) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.hooks[hookType]) > 0
}

// Count returns the number of hooks registered for the specified type
func (h *Hooks) Count(hookType HookType) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.hooks[hookType])
}

// ============================================================================
// Hook Builder for Fluent API
// ============================================================================

// HookBuilder provides a fluent API for registering hooks
type HookBuilder struct {
	hooks *Hooks
}

// NewHookBuilder creates a new HookBuilder
func NewHookBuilder() *HookBuilder {
	return &HookBuilder{
		hooks: NewHooks(),
	}
}

// BeforeSubmit registers a hook called before task submission
func (b *HookBuilder) BeforeSubmit(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookBeforeSubmit, fn)
	return b
}

// AfterSubmit registers a hook called after successful task submission
func (b *HookBuilder) AfterSubmit(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookAfterSubmit, fn)
	return b
}

// BeforeTask registers a hook called before task execution
func (b *HookBuilder) BeforeTask(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookBeforeTask, fn)
	return b
}

// AfterTask registers a hook called after task execution
func (b *HookBuilder) AfterTask(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookAfterTask, fn)
	return b
}

// OnPanic registers a hook called when a task panics
func (b *HookBuilder) OnPanic(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookOnPanic, fn)
	return b
}

// OnReject registers a hook called when a task is rejected
func (b *HookBuilder) OnReject(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookOnReject, fn)
	return b
}

// OnTimeout registers a hook called when a task times out
func (b *HookBuilder) OnTimeout(fn TypedTaskHook) *HookBuilder {
	b.hooks.RegisterTask(HookOnTimeout, fn)
	return b
}

// OnWorkerStart registers a hook called when a worker starts
func (b *HookBuilder) OnWorkerStart(fn TypedWorkerHook) *HookBuilder {
	b.hooks.RegisterWorker(HookOnWorkerStart, fn)
	return b
}

// OnWorkerStop registers a hook called when a worker stops
func (b *HookBuilder) OnWorkerStop(fn TypedWorkerHook) *HookBuilder {
	b.hooks.RegisterWorker(HookOnWorkerStop, fn)
	return b
}

// OnScaleUp registers a hook called when the pool scales up
func (b *HookBuilder) OnScaleUp(fn TypedScaleHook) *HookBuilder {
	b.hooks.RegisterScale(HookOnScaleUp, fn)
	return b
}

// OnScaleDown registers a hook called when the pool scales down
func (b *HookBuilder) OnScaleDown(fn TypedScaleHook) *HookBuilder {
	b.hooks.RegisterScale(HookOnScaleDown, fn)
	return b
}

// Build returns the configured Hooks
func (b *HookBuilder) Build() *Hooks {
	return b.hooks
}
