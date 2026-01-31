package asynq

import (
	"fmt"
	"sync"
	"time"
)

// =========================================
// 任务状态机
// 管理任务状态转换，确保状态一致性
// =========================================
// TaskState 任务状态
type TaskState string

const (
	// StateCreated 已创建（初始状态）
	StateCreated TaskState = "CREATED"
	// StatePending 待处理
	StatePending TaskState = "PENDING"
	// StateQueued 已入队
	StateQueued TaskState = "QUEUED"
	// StateProcessing 处理中
	StateProcessing TaskState = "PROCESSING"
	// StatePolling 轮询中
	StatePolling TaskState = "POLLING"
	// StateSuccess 成功
	StateSuccess TaskState = "SUCCESS"
	// StateFailure 失败
	StateFailure TaskState = "FAILURE"
	// StateTimeout 超时
	StateTimeout TaskState = "TIMEOUT"
	// StateCancelled 已取消
	StateCancelled TaskState = "CANCELLED"
	// StateDeadLetter 死信
	StateDeadLetter TaskState = "DEAD_LETTER"
)

// TaskEvent 状态转换事件
type TaskEvent string

const (
	// EventSubmit 提交任务
	EventSubmit TaskEvent = "SUBMIT"
	// EventEnqueue 入队
	EventEnqueue TaskEvent = "ENQUEUE"
	// EventStart 开始处理
	EventStart TaskEvent = "START"
	// EventPoll 开始轮询
	EventPoll TaskEvent = "POLL"
	// EventProgress 进度更新
	EventProgress TaskEvent = "PROGRESS"
	// EventComplete 完成
	EventComplete TaskEvent = "COMPLETE"
	// EventFail 失败
	EventFail TaskEvent = "FAIL"
	// EventTimeout 超时
	EventTimeout TaskEvent = "TIMEOUT"
	// EventCancel 取消
	EventCancel TaskEvent = "CANCEL"
	// EventRetry 重试
	EventRetry TaskEvent = "RETRY"
	// EventDeadLetter 进入死信队列
	EventDeadLetter TaskEvent = "DEAD_LETTER"
)

// StateTransition 状态转换定义
type StateTransition struct {
	FromState TaskState
	Event     TaskEvent
	ToState   TaskState
	// 转换条件函数（可选）
	Condition func(ctx *TransitionContext) bool
	// 转换后回调（可选）
	OnTransition func(ctx *TransitionContext)
}

// TransitionContext 转换上下文
type TransitionContext struct {
	TaskID    string
	FromState TaskState
	ToState   TaskState
	Event     TaskEvent
	Data      map[string]any
	Timestamp time.Time
}

// TaskStateMachine 任务状态机
type TaskStateMachine struct {
	mu          sync.RWMutex
	transitions map[transitionKey]StateTransition
	hooks       []TransitionHook
}

// transitionKey 转换键
type transitionKey struct {
	FromState TaskState
	Event     TaskEvent
}

// TransitionHook 状态转换钩子
type TransitionHook func(ctx *TransitionContext)

// NewTaskStateMachine 创建状态机
func NewTaskStateMachine() *TaskStateMachine {
	sm := &TaskStateMachine{
		transitions: make(map[transitionKey]StateTransition),
		hooks:       make([]TransitionHook, 0),
	}
	// 注册默认状态转换
	sm.registerDefaultTransitions()
	return sm
}

// registerDefaultTransitions 注册默认状态转换
func (sm *TaskStateMachine) registerDefaultTransitions() {
	// 创建 -> 待处理
	sm.AddTransition(StateCreated, EventSubmit, StatePending)
	// 待处理 -> 已入队
	sm.AddTransition(StatePending, EventEnqueue, StateQueued)
	// 已入队 -> 处理中
	sm.AddTransition(StateQueued, EventStart, StateProcessing)
	// 处理中 -> 轮询中
	sm.AddTransition(StateProcessing, EventPoll, StatePolling)
	// 轮询中 -> 成功
	sm.AddTransition(StatePolling, EventComplete, StateSuccess)
	// 轮询中 -> 失败
	sm.AddTransition(StatePolling, EventFail, StateFailure)
	// 轮询中 -> 超时
	sm.AddTransition(StatePolling, EventTimeout, StateTimeout)
	// 轮询中 -> 死信
	sm.AddTransition(StatePolling, EventDeadLetter, StateDeadLetter)
	// 轮询中 -> 重试（保持轮询状态）
	sm.AddTransition(StatePolling, EventRetry, StatePolling)
	// 待处理 -> 取消
	sm.AddTransition(StatePending, EventCancel, StateCancelled)
	// 已入队 -> 取消
	sm.AddTransition(StateQueued, EventCancel, StateCancelled)
	// 处理中 -> 取消
	sm.AddTransition(StateProcessing, EventCancel, StateCancelled)
	// 轮询中 -> 取消
	sm.AddTransition(StatePolling, EventCancel, StateCancelled)
	// 处理中 -> 成功（直接返回结果的情况）
	sm.AddTransition(StateProcessing, EventComplete, StateSuccess)
	// 处理中 -> 失败
	sm.AddTransition(StateProcessing, EventFail, StateFailure)
	// 超时 -> 死信
	sm.AddTransition(StateTimeout, EventDeadLetter, StateDeadLetter)
	// 死信 -> 重试（人工触发）
	sm.AddTransition(StateDeadLetter, EventRetry, StateQueued)
}

// AddTransition 添加状态转换
func (sm *TaskStateMachine) AddTransition(from TaskState, event TaskEvent, to TaskState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	key := transitionKey{FromState: from, Event: event}
	sm.transitions[key] = StateTransition{
		FromState: from,
		Event:     event,
		ToState:   to,
	}
}

// AddTransitionWithCallback 添加带回调的状态转换
func (sm *TaskStateMachine) AddTransitionWithCallback(from TaskState, event TaskEvent, to TaskState, callback func(ctx *TransitionContext)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	key := transitionKey{FromState: from, Event: event}
	sm.transitions[key] = StateTransition{
		FromState:    from,
		Event:        event,
		ToState:      to,
		OnTransition: callback,
	}
}

// AddHook 添加全局钩子
func (sm *TaskStateMachine) AddHook(hook TransitionHook) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.hooks = append(sm.hooks, hook)
}

// CanTransition 检查是否可以转换
func (sm *TaskStateMachine) CanTransition(from TaskState, event TaskEvent) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	key := transitionKey{FromState: from, Event: event}
	_, ok := sm.transitions[key]
	return ok
}

// Transition 执行状态转换
func (sm *TaskStateMachine) Transition(taskID string, from TaskState, event TaskEvent, data map[string]any) (TaskState, error) {
	sm.mu.RLock()
	key := transitionKey{FromState: from, Event: event}
	transition, ok := sm.transitions[key]
	hooks := sm.hooks
	sm.mu.RUnlock()
	if !ok {
		return from, fmt.Errorf("invalid transition: %s + %s", from, event)
	}
	ctx := &TransitionContext{
		TaskID:    taskID,
		FromState: from,
		ToState:   transition.ToState,
		Event:     event,
		Data:      data,
		Timestamp: time.Now(),
	}
	// 检查条件
	if transition.Condition != nil && !transition.Condition(ctx) {
		return from, fmt.Errorf("transition condition not met: %s + %s", from, event)
	}
	// 执行转换
	GetLogger().Log(fmt.Sprintf("[StateMachine] Task %s: %s -> %s (event=%s)",
		taskID, from, transition.ToState, event))
	// 执行转换回调
	if transition.OnTransition != nil {
		transition.OnTransition(ctx)
	}
	// 执行全局钩子
	for _, hook := range hooks {
		hook(ctx)
	}
	return transition.ToState, nil
}

// GetValidEvents 获取当前状态可用的事件
func (sm *TaskStateMachine) GetValidEvents(state TaskState) []TaskEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	events := make([]TaskEvent, 0)
	for key := range sm.transitions {
		if key.FromState == state {
			events = append(events, key.Event)
		}
	}
	return events
}

// =========================================
// 全局状态机实例
// =========================================
var (
	globalStateMachine     *TaskStateMachine
	globalStateMachineOnce sync.Once
)

// GetTaskStateMachine 获取全局状态机
func GetTaskStateMachine() *TaskStateMachine {
	globalStateMachineOnce.Do(func() {
		globalStateMachine = NewTaskStateMachine()
	})
	return globalStateMachine
}

// =========================================
// 便捷函数
// =========================================
// IsTerminalState 检查是否是终态
func IsTerminalState(state TaskState) bool {
	switch state {
	case StateSuccess, StateFailure, StateTimeout, StateCancelled:
		return true
	default:
		return false
	}
}

// IsActiveState 检查是否是活跃状态
func IsActiveState(state TaskState) bool {
	switch state {
	case StatePending, StateQueued, StateProcessing, StatePolling:
		return true
	default:
		return false
	}
}

// NormalizeState 标准化状态字符串
func NormalizeState(status string) TaskState {
	switch status {
	case "SUCCESS", "success", "completed", "succeeded", "done":
		return StateSuccess
	case "FAILURE", "failure", "failed", "error":
		return StateFailure
	case "PROCESSING", "processing", "running", "in_progress":
		return StateProcessing
	case "PENDING", "pending", "queued", "waiting":
		return StatePending
	case "TIMEOUT", "timeout", "timed_out":
		return StateTimeout
	case "CANCELLED", "cancelled", "canceled":
		return StateCancelled
	default:
		return TaskState(status)
	}
}

// =========================================
// 任务状态追踪器
// =========================================
// TaskStateTracker 任务状态追踪器
type TaskStateTracker struct {
	mu      sync.RWMutex
	states  map[string]TaskState
	history map[string][]StateHistory
	maxHist int
}

// StateHistory 状态历史
type StateHistory struct {
	FromState TaskState      `json:"from_state"`
	ToState   TaskState      `json:"to_state"`
	Event     TaskEvent      `json:"event"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

var (
	stateTracker     *TaskStateTracker
	stateTrackerOnce sync.Once
)

// GetTaskStateTracker 获取状态追踪器
func GetTaskStateTracker() *TaskStateTracker {
	stateTrackerOnce.Do(func() {
		stateTracker = &TaskStateTracker{
			states:  make(map[string]TaskState),
			history: make(map[string][]StateHistory),
			maxHist: 100, // 每个任务保留最多 100 条历史
		}
	})
	return stateTracker
}

// GetState 获取任务状态
func (t *TaskStateTracker) GetState(taskID string) TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.states[taskID]
}

// SetState 设置任务状态
func (t *TaskStateTracker) SetState(taskID string, state TaskState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[taskID] = state
}

// RecordTransition 记录状态转换
func (t *TaskStateTracker) RecordTransition(taskID string, from, to TaskState, event TaskEvent, data map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.states[taskID] = to
	hist := StateHistory{
		FromState: from,
		ToState:   to,
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}
	t.history[taskID] = append(t.history[taskID], hist)
	// 限制历史记录数量
	if len(t.history[taskID]) > t.maxHist {
		t.history[taskID] = t.history[taskID][len(t.history[taskID])-t.maxHist:]
	}
}

// GetHistory 获取状态历史
func (t *TaskStateTracker) GetHistory(taskID string) []StateHistory {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if hist, ok := t.history[taskID]; ok {
		result := make([]StateHistory, len(hist))
		copy(result, hist)
		return result
	}
	return nil
}

// Cleanup 清理过期任务状态（保留 24 小时）
func (t *TaskStateTracker) Cleanup(maxAge time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for taskID, hist := range t.history {
		if len(hist) > 0 {
			lastTime := hist[len(hist)-1].Timestamp
			if lastTime.Before(cutoff) {
				delete(t.history, taskID)
				delete(t.states, taskID)
			}
		}
	}
}
