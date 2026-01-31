package asynq

import (
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// =========================================
// task_types.go 测试
// =========================================

func TestDefaultTaskConfig(t *testing.T) {
	cfg := DefaultTaskConfig()

	if cfg.InitialDelay != 3*time.Second {
		t.Errorf("expected InitialDelay 3s, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Errorf("expected MaxDelay 10s, got %v", cfg.MaxDelay)
	}
	if cfg.MaxRetry != 3 {
		t.Errorf("expected MaxRetry 3, got %d", cfg.MaxRetry)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("expected Timeout 60s, got %v", cfg.Timeout)
	}
	if cfg.SupportsWebhook {
		t.Error("expected SupportsWebhook false")
	}
	if cfg.BatchSupported {
		t.Error("expected BatchSupported false")
	}
	if cfg.MaxBatchSize != 0 {
		t.Errorf("expected MaxBatchSize 0, got %d", cfg.MaxBatchSize)
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name         string
		retryCount   int
		initialDelay time.Duration
		maxDelay     time.Duration
		expected     time.Duration
	}{
		{
			name:         "retry 0 returns initial delay",
			retryCount:   0,
			initialDelay: 1 * time.Second,
			maxDelay:     60 * time.Second,
			expected:     1 * time.Second,
		},
		{
			name:         "retry 1 doubles delay",
			retryCount:   1,
			initialDelay: 1 * time.Second,
			maxDelay:     60 * time.Second,
			expected:     2 * time.Second,
		},
		{
			name:         "retry 2 quadruples delay",
			retryCount:   2,
			initialDelay: 1 * time.Second,
			maxDelay:     60 * time.Second,
			expected:     4 * time.Second,
		},
		{
			name:         "retry 3",
			retryCount:   3,
			initialDelay: 1 * time.Second,
			maxDelay:     60 * time.Second,
			expected:     8 * time.Second,
		},
		{
			name:         "caps at max delay",
			retryCount:   10,
			initialDelay: 1 * time.Second,
			maxDelay:     60 * time.Second,
			expected:     60 * time.Second,
		},
		{
			name:         "custom initial delay",
			retryCount:   2,
			initialDelay: 5 * time.Second,
			maxDelay:     120 * time.Second,
			expected:     20 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateBackoff(tt.retryCount, tt.initialDelay, tt.maxDelay)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =========================================
// queues.go 测试
// =========================================

func TestFormatPollTaskID(t *testing.T) {
	tests := []struct {
		taskID     string
		retryCount int
		expected   string
	}{
		{"task123", 0, "poll:task123:0"},
		{"task123", 1, "poll:task123:1"},
		{"task123", 5, "poll:task123:5"},
		{"abc-def-ghi", 3, "poll:abc-def-ghi:3"},
	}

	for _, tt := range tests {
		result := FormatPollTaskID(tt.taskID, tt.retryCount)
		if result != tt.expected {
			t.Errorf("FormatPollTaskID(%q, %d) = %q, want %q",
				tt.taskID, tt.retryCount, result, tt.expected)
		}
	}
}

func TestFormatPollTaskIDInitial(t *testing.T) {
	result := FormatPollTaskIDInitial("task123")
	expected := "poll:task123:0"
	if result != expected {
		t.Errorf("FormatPollTaskIDInitial(%q) = %q, want %q", "task123", result, expected)
	}
}

func TestFormatRetryTaskID(t *testing.T) {
	result := FormatRetryTaskID("task123")
	// 格式: poll:{taskID}:retry:{timestamp}
	if !strings.HasPrefix(result, "poll:task123:retry:") {
		t.Errorf("FormatRetryTaskID result %q doesn't have expected prefix", result)
	}
}

func TestDefaultQueues(t *testing.T) {
	queues := DefaultQueues()

	// 验证所有队列都存在
	expectedQueues := []string{
		QueueCritical,
		QueueHigh,
		QueueDefault,
		QueueScheduled,
		QueueLow,
		QueueDeadLetter,
	}

	for _, q := range expectedQueues {
		if _, ok := queues[q]; !ok {
			t.Errorf("expected queue %q not found", q)
		}
	}

	// 验证优先级顺序
	if queues[QueueCritical] <= queues[QueueHigh] {
		t.Error("critical should have higher priority than high")
	}
	if queues[QueueHigh] <= queues[QueueDefault] {
		t.Error("high should have higher priority than default")
	}
}

func TestIsTaskConflictError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"duplicate task", asynq.ErrDuplicateTask, true},
		{"task id conflict", asynq.ErrTaskIDConflict, true},
		{"other error", asynq.ErrServerClosed, false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTaskConflictError(tt.err)
			if result != tt.expected {
				t.Errorf("IsTaskConflictError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

// =========================================
// state_machine.go 测试
// =========================================

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    TaskState
		expected bool
	}{
		{StateSuccess, true},
		{StateFailure, true},
		{StateTimeout, true},
		{StateCancelled, true},
		{StatePending, false},
		{StateQueued, false},
		{StateProcessing, false},
		{StatePolling, false},
		{TaskState("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := IsTerminalState(tt.state)
			if result != tt.expected {
				t.Errorf("IsTerminalState(%q) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestIsActiveState(t *testing.T) {
	tests := []struct {
		state    TaskState
		expected bool
	}{
		{StatePending, true},
		{StateQueued, true},
		{StateProcessing, true},
		{StatePolling, true},
		{StateSuccess, false},
		{StateFailure, false},
		{StateTimeout, false},
		{StateCancelled, false},
		{TaskState("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := IsActiveState(tt.state)
			if result != tt.expected {
				t.Errorf("IsActiveState(%q) = %v, want %v", tt.state, result, tt.expected)
			}
		})
	}
}

func TestNormalizeState(t *testing.T) {
	tests := []struct {
		input    string
		expected TaskState
	}{
		// Success variants
		{"SUCCESS", StateSuccess},
		{"success", StateSuccess},
		{"completed", StateSuccess},
		{"succeeded", StateSuccess},
		{"done", StateSuccess},
		// Failure variants
		{"FAILURE", StateFailure},
		{"failure", StateFailure},
		{"failed", StateFailure},
		{"error", StateFailure},
		// Processing variants
		{"PROCESSING", StateProcessing},
		{"processing", StateProcessing},
		{"running", StateProcessing},
		{"in_progress", StateProcessing},
		// Pending variants
		{"PENDING", StatePending},
		{"pending", StatePending},
		{"queued", StatePending},
		{"waiting", StatePending},
		// Timeout variants
		{"TIMEOUT", StateTimeout},
		{"timeout", StateTimeout},
		{"timed_out", StateTimeout},
		// Cancelled variants
		{"CANCELLED", StateCancelled},
		{"cancelled", StateCancelled},
		{"canceled", StateCancelled},
		// Unknown returns as-is
		{"unknown_state", TaskState("unknown_state")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeState(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeState(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// =========================================
// manager.go 测试
// =========================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if len(cfg.RedisAddrs) != 0 {
		t.Errorf("expected empty RedisAddrs, got %v", cfg.RedisAddrs)
	}
	if cfg.Concurrency != 10 {
		t.Errorf("expected Concurrency 10, got %d", cfg.Concurrency)
	}
	if cfg.LogLevel != asynq.InfoLevel {
		t.Errorf("expected LogLevel InfoLevel, got %v", cfg.LogLevel)
	}
	if cfg.Queues == nil {
		t.Error("expected non-nil Queues")
	}
	if cfg.RetryDelay == nil {
		t.Error("expected non-nil RetryDelay")
	}

	// Test retry delay function
	delay := cfg.RetryDelay(0)
	if delay != 1*time.Second {
		t.Errorf("RetryDelay(0) = %v, want 1s", delay)
	}
	delay = cfg.RetryDelay(2)
	if delay != 4*time.Second {
		t.Errorf("RetryDelay(2) = %v, want 4s", delay)
	}
}

func TestNewManager_NilConfig(t *testing.T) {
	_, err := NewManager(nil)
	if err == nil {
		t.Error("expected error for nil config with no redis addrs")
	}
}

func TestNewManager_EmptyRedisAddrs(t *testing.T) {
	cfg := &Config{
		RedisAddrs: []string{},
	}
	_, err := NewManager(cfg)
	if err == nil {
		t.Error("expected error for empty redis addrs")
	}
}

func TestGetManager_Nil(t *testing.T) {
	// Reset global manager
	managerMu.Lock()
	globalManager = nil
	managerMu.Unlock()

	m := GetManager()
	if m != nil {
		t.Error("expected nil manager when not initialized")
	}
}

func TestGetInspector_NoManager(t *testing.T) {
	// Reset global manager
	managerMu.Lock()
	globalManager = nil
	managerMu.Unlock()

	inspector := GetInspector()
	if inspector != nil {
		t.Error("expected nil inspector when manager not initialized")
	}
}

// =========================================
// interfaces.go 测试
// =========================================

func TestGetLogger(t *testing.T) {
	logger := GetLogger()
	if logger == nil {
		t.Error("GetLogger should not return nil")
	}
}

func TestStdLogger(t *testing.T) {
	logger := &StdLogger{}

	// These should not panic
	logger.Log("test message")
	logger.LogSkip(1, "test message skip")
	logger.Error("test error")
	logger.ErrorSkip(1, "test error skip")
}

// =========================================
// 队列名称常量测试
// =========================================

func TestQueueNameConstants(t *testing.T) {
	// 验证默认队列名称
	if QueueCritical == "" {
		t.Error("QueueCritical should not be empty")
	}
	if QueueHigh == "" {
		t.Error("QueueHigh should not be empty")
	}
	if QueueDefault == "" {
		t.Error("QueueDefault should not be empty")
	}
	if QueueScheduled == "" {
		t.Error("QueueScheduled should not be empty")
	}
	if QueueLow == "" {
		t.Error("QueueLow should not be empty")
	}
	if QueueDeadLetter == "" {
		t.Error("QueueDeadLetter should not be empty")
	}
}

func TestTaskTypeConstants(t *testing.T) {
	// 验证任务类型前缀
	if TaskPrefixTask == "" {
		t.Error("TaskPrefixTask should not be empty")
	}
	if TaskPrefixNotify == "" {
		t.Error("TaskPrefixNotify should not be empty")
	}
	if TaskPrefixDLQ == "" {
		t.Error("TaskPrefixDLQ should not be empty")
	}

	// 验证任务类型包含前缀
	if !strings.HasPrefix(TaskTypeTaskProcess, TaskPrefixTask) {
		t.Error("TaskTypeTaskProcess should have TaskPrefixTask")
	}
	if !strings.HasPrefix(TaskTypeDeadLetter, TaskPrefixDLQ) {
		t.Error("TaskTypeDeadLetter should have TaskPrefixDLQ")
	}
	if !strings.HasPrefix(TaskTypeNotifyEmail, TaskPrefixNotify) {
		t.Error("TaskTypeNotifyEmail should have TaskPrefixNotify")
	}
}

// =========================================
// 状态机测试
// =========================================

func TestTaskStateMachine(t *testing.T) {
	sm := NewTaskStateMachine()
	if sm == nil {
		t.Fatal("NewTaskStateMachine returned nil")
	}

	// 验证状态机可以进行基本转换
	if !sm.CanTransition(StatePending, EventEnqueue) {
		t.Error("should be able to transition from Pending to Queued")
	}

	// 验证终态不能转换
	if sm.CanTransition(StateSuccess, EventEnqueue) {
		t.Error("terminal state should not be able to transition")
	}
}

func TestTaskStateMachine_CanTransition(t *testing.T) {
	sm := NewTaskStateMachine()

	tests := []struct {
		from     TaskState
		event    TaskEvent
		expected bool
	}{
		{StatePending, EventEnqueue, true},
		{StateQueued, EventStart, true},
		{StateProcessing, EventComplete, true},
		{StateProcessing, EventFail, true},
		{StateSuccess, EventEnqueue, false}, // 终态不能转换
		{StateFailure, EventStart, false},   // 终态不能转换
	}

	for _, tt := range tests {
		name := string(tt.from) + "_" + string(tt.event)
		t.Run(name, func(t *testing.T) {
			result := sm.CanTransition(tt.from, tt.event)
			if result != tt.expected {
				t.Errorf("CanTransition(%q, %q) = %v, want %v",
					tt.from, tt.event, result, tt.expected)
			}
		})
	}
}

func TestTaskStateMachine_Transition(t *testing.T) {
	sm := NewTaskStateMachine()

	tests := []struct {
		from          TaskState
		event         TaskEvent
		expectedState TaskState
		expectErr     bool
	}{
		{StatePending, EventEnqueue, StateQueued, false},
		{StateQueued, EventStart, StateProcessing, false},
		{StateProcessing, EventComplete, StateSuccess, false},
		{StateProcessing, EventFail, StateFailure, false},
		{StateSuccess, EventEnqueue, StateSuccess, true}, // 终态转换应失败，返回原状态
	}

	for _, tt := range tests {
		name := string(tt.from) + "_" + string(tt.event)
		t.Run(name, func(t *testing.T) {
			newState, err := sm.Transition("test-task", tt.from, tt.event, nil)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if newState != tt.expectedState {
					t.Errorf("expected state %q, got %q", tt.expectedState, newState)
				}
			}
		})
	}
}

func TestGetTaskStateMachine_Singleton(t *testing.T) {
	sm1 := GetTaskStateMachine()
	sm2 := GetTaskStateMachine()

	if sm1 != sm2 {
		t.Error("GetTaskStateMachine should return the same instance")
	}
}
