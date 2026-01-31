package asynq

import (
	"testing"
	"time"
)

func TestTaskState_String(t *testing.T) {
	tests := []struct {
		state    TaskState
		expected string
	}{
		{StateCreated, "CREATED"},
		{StatePending, "PENDING"},
		{StateQueued, "QUEUED"},
		{StateProcessing, "PROCESSING"},
		{StatePolling, "POLLING"},
		{StateSuccess, "SUCCESS"},
		{StateFailure, "FAILURE"},
		{StateTimeout, "TIMEOUT"},
		{StateCancelled, "CANCELLED"},
		{StateDeadLetter, "DEAD_LETTER"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("state %s: expected %s, got %s", tt.state, tt.expected, string(tt.state))
		}
	}
}

func TestTaskEvent_String(t *testing.T) {
	tests := []struct {
		event    TaskEvent
		expected string
	}{
		{EventSubmit, "SUBMIT"},
		{EventEnqueue, "ENQUEUE"},
		{EventStart, "START"},
		{EventPoll, "POLL"},
		{EventProgress, "PROGRESS"},
		{EventComplete, "COMPLETE"},
		{EventFail, "FAIL"},
		{EventTimeout, "TIMEOUT"},
		{EventCancel, "CANCEL"},
		{EventRetry, "RETRY"},
		{EventDeadLetter, "DEAD_LETTER"},
	}

	for _, tt := range tests {
		if string(tt.event) != tt.expected {
			t.Errorf("event %s: expected %s, got %s", tt.event, tt.expected, string(tt.event))
		}
	}
}

func TestNewTaskStateMachine(t *testing.T) {
	sm := NewTaskStateMachine()

	if sm == nil {
		t.Fatal("state machine is nil")
	}

	// 验证默认转换规则已注册
	// CREATED -> PENDING (SUBMIT)
	canTransition := sm.CanTransition(StateCreated, EventSubmit)
	if !canTransition {
		t.Error("expected CREATED -> SUBMIT -> PENDING to be allowed")
	}
}

func TestTaskStateMachine_AddTransition(t *testing.T) {
	sm := NewTaskStateMachine()

	// 添加自定义转换
	sm.AddTransition(StateCreated, EventCancel, StateCancelled)

	// 验证转换已注册
	canTransition := sm.CanTransition(StateCreated, EventCancel)
	if !canTransition {
		t.Error("expected custom transition to be registered")
	}
}

func TestTaskStateMachine_CanTransition_Valid(t *testing.T) {
	sm := NewTaskStateMachine()

	// 测试有效的转换
	validTransitions := []struct {
		from  TaskState
		event TaskEvent
	}{
		{StateCreated, EventSubmit},      // CREATED -> PENDING
		{StatePending, EventEnqueue},     // PENDING -> QUEUED
		{StateQueued, EventStart},        // QUEUED -> PROCESSING
		{StateProcessing, EventComplete}, // PROCESSING -> SUCCESS
		{StateProcessing, EventFail},     // PROCESSING -> FAILURE
	}

	for _, tt := range validTransitions {
		if !sm.CanTransition(tt.from, tt.event) {
			t.Errorf("expected transition %s + %s to be valid", tt.from, tt.event)
		}
	}
}

func TestTaskStateMachine_CanTransition_Invalid(t *testing.T) {
	sm := NewTaskStateMachine()

	// 测试无效的转换
	if sm.CanTransition(StateSuccess, EventStart) {
		t.Error("expected SUCCESS -> START to be invalid")
	}

	if sm.CanTransition(StateFailure, EventComplete) {
		t.Error("expected FAILURE -> COMPLETE to be invalid")
	}
}

func TestTaskStateMachine_Transition_Valid(t *testing.T) {
	sm := NewTaskStateMachine()

	newState, err := sm.Transition("task-123", StateCreated, EventSubmit, make(map[string]any))
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if newState != StatePending {
		t.Errorf("expected new state PENDING, got %s", newState)
	}
}

func TestTaskStateMachine_Transition_Invalid(t *testing.T) {
	sm := NewTaskStateMachine()

	_, err := sm.Transition("task-123", StateSuccess, EventStart, make(map[string]any))
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestTaskStateMachine_Transition_WithCondition(t *testing.T) {
	sm := &TaskStateMachine{
		transitions: make(map[transitionKey]StateTransition),
		hooks:       make([]TransitionHook, 0),
	}

	conditionMet := false

	// 注册带条件的转换
	sm.transitions[transitionKey{FromState: StateCreated, Event: EventCancel}] = StateTransition{
		FromState: StateCreated,
		Event:     EventCancel,
		ToState:   StateCancelled,
		Condition: func(ctx *TransitionContext) bool {
			return conditionMet
		},
	}

	// 条件不满足，应该失败
	_, err := sm.Transition("task-123", StateCreated, EventCancel, make(map[string]any))
	if err == nil {
		t.Error("expected error when condition is not met")
	}

	// 条件满足，应该成功
	conditionMet = true
	newState, err := sm.Transition("task-123", StateCreated, EventCancel, make(map[string]any))
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if newState != StateCancelled {
		t.Errorf("expected new state CANCELLED, got %s", newState)
	}
}

func TestTaskStateMachine_Transition_WithCallback(t *testing.T) {
	sm := NewTaskStateMachine()

	callbackCalled := false

	// 使用 AddTransitionWithCallback
	sm.AddTransitionWithCallback(StateCreated, EventCancel, StateCancelled, func(ctx *TransitionContext) {
		callbackCalled = true
	})

	_, err := sm.Transition("task-123", StateCreated, EventCancel, make(map[string]any))
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if !callbackCalled {
		t.Error("expected OnTransition callback to be called")
	}
}

func TestTaskStateMachine_AddHook(t *testing.T) {
	sm := NewTaskStateMachine()

	hookCalled := false

	// 添加钩子
	sm.AddHook(func(ctx *TransitionContext) {
		hookCalled = true
	})

	_, err := sm.Transition("task-123", StateCreated, EventSubmit, make(map[string]any))
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if !hookCalled {
		t.Error("expected hook to be called")
	}
}

func TestTaskStateMachine_GetValidEvents(t *testing.T) {
	sm := NewTaskStateMachine()

	// 获取 CREATED 状态的有效事件
	events := sm.GetValidEvents(StateCreated)

	// 应该至少包含 SUBMIT
	found := false
	for _, e := range events {
		if e == EventSubmit {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected SUBMIT to be in valid events for CREATED state")
	}
}

func TestTransitionContext(t *testing.T) {
	ctx := &TransitionContext{
		TaskID:    "test-task",
		FromState: StateCreated,
		ToState:   StatePending,
		Event:     EventSubmit,
		Data:      map[string]any{"user_id": 123},
		Timestamp: time.Now(),
	}

	if ctx.TaskID != "test-task" {
		t.Errorf("expected TaskID 'test-task', got '%s'", ctx.TaskID)
	}

	if ctx.Data["user_id"] != 123 {
		t.Error("expected Data to be preserved")
	}
}

func TestTaskStateMachine_Concurrent(t *testing.T) {
	sm := NewTaskStateMachine()

	// 并发注册转换（测试线程安全）
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			sm.AddTransition(StateCreated, TaskEvent("CUSTOM_"+string(rune('0'+n))), StatePending)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 并发查询（测试读锁）
	for i := 0; i < 10; i++ {
		go func() {
			sm.CanTransition(StateCreated, EventSubmit)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
