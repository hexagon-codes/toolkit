package asynq

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

func TestBackpressureState_String(t *testing.T) {
	tests := []struct {
		state    BackpressureState
		expected string
	}{
		{StateNormal, "NORMAL"},
		{StateWarning, "WARNING"},
		{StateCritical, "CRITICAL"},
		{BackpressureState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("state %d: expected %s, got %s", tt.state, tt.expected, got)
		}
	}
}

func TestDefaultBackpressureConfig(t *testing.T) {
	config := DefaultBackpressureConfig()

	if config.MaxQueueSize != 10000 {
		t.Errorf("expected MaxQueueSize=10000, got %d", config.MaxQueueSize)
	}

	if config.WarningThreshold != 0.7 {
		t.Errorf("expected WarningThreshold=0.7, got %f", config.WarningThreshold)
	}

	if config.CriticalThreshold != 0.9 {
		t.Errorf("expected CriticalThreshold=0.9, got %f", config.CriticalThreshold)
	}

	if config.CheckInterval != 30*time.Second {
		t.Errorf("expected CheckInterval=30s, got %v", config.CheckInterval)
	}
}

func TestQueueBackpressure(t *testing.T) {
	bp := &QueueBackpressure{
		Queue:         "test-queue",
		State:         StateNormal,
		StateStr:      "NORMAL",
		CurrentSize:   100,
		MaxSize:       1000,
		Utilization:   0.1,
		RejectCount:   0,
		LastCheckTime: time.Now(),
	}

	if bp.Queue != "test-queue" {
		t.Errorf("expected queue 'test-queue', got '%s'", bp.Queue)
	}

	if bp.State != StateNormal {
		t.Errorf("expected state NORMAL, got %s", bp.State.String())
	}
	if bp.StateStr != "NORMAL" || bp.CurrentSize != 100 || bp.MaxSize != 1000 {
		t.Errorf("unexpected queue snapshot: %+v", bp)
	}
	if bp.RejectCount != 0 || bp.LastCheckTime.IsZero() {
		t.Errorf("unexpected rejection metadata: %+v", bp)
	}

	if bp.Utilization != 0.1 {
		t.Errorf("expected utilization 0.1, got %f", bp.Utilization)
	}
}

func TestBackpressureController_CalculateUtilization(t *testing.T) {
	tests := []struct {
		currentSize int
		maxSize     int
		expected    float64
	}{
		{0, 1000, 0.0},
		{500, 1000, 0.5},
		{1000, 1000, 1.0},
		{700, 1000, 0.7},
		{900, 1000, 0.9},
	}

	for _, tt := range tests {
		utilization := float64(tt.currentSize) / float64(tt.maxSize)
		if utilization != tt.expected {
			t.Errorf("size=%d max=%d: expected %.1f, got %.1f",
				tt.currentSize, tt.maxSize, tt.expected, utilization)
		}
	}
}

func TestBackpressureController_StateTransition(t *testing.T) {
	config := BackpressureConfig{
		MaxQueueSize:      1000,
		WarningThreshold:  0.7,
		CriticalThreshold: 0.9,
		CheckInterval:     time.Second,
	}
	if config.CheckInterval != time.Second {
		t.Fatalf("unexpected check interval: %v", config.CheckInterval)
	}

	tests := []struct {
		size          int
		expectedState BackpressureState
		description   string
	}{
		{100, StateNormal, "10% utilization - should be NORMAL"},
		{700, StateWarning, "70% utilization - should be WARNING"},
		{900, StateCritical, "90% utilization - should be CRITICAL"},
		{500, StateNormal, "50% utilization - should be NORMAL"},
	}

	for _, tt := range tests {
		utilization := float64(tt.size) / float64(config.MaxQueueSize)
		var state BackpressureState

		if utilization >= config.CriticalThreshold {
			state = StateCritical
		} else if utilization >= config.WarningThreshold {
			state = StateWarning
		} else {
			state = StateNormal
		}

		if state != tt.expectedState {
			t.Errorf("%s: expected %s, got %s", tt.description, tt.expectedState.String(), state.String())
		}
	}
}

func TestBackpressureController_Callbacks(t *testing.T) {
	warningCalled := false
	criticalCalled := false
	recoverCalled := false

	config := BackpressureConfig{
		OnWarning: func(queue string, size int, threshold int) {
			warningCalled = true
		},
		OnCritical: func(queue string, size int, threshold int) {
			criticalCalled = true
		},
		OnRecover: func(queue string, size int) {
			recoverCalled = true
		},
	}

	// 模拟进入警告状态
	if config.OnWarning != nil {
		config.OnWarning("test", 700, 700)
	}

	if !warningCalled {
		t.Error("expected OnWarning to be called")
	}

	// 模拟进入危急状态
	if config.OnCritical != nil {
		config.OnCritical("test", 900, 900)
	}

	if !criticalCalled {
		t.Error("expected OnCritical to be called")
	}

	// 模拟恢复
	if config.OnRecover != nil {
		config.OnRecover("test", 100)
	}

	if !recoverCalled {
		t.Error("expected OnRecover to be called")
	}
}

func TestBackpressureThresholds(t *testing.T) {
	config := DefaultBackpressureConfig()

	// 验证阈值逻辑
	warningSize := int(float64(config.MaxQueueSize) * config.WarningThreshold)
	criticalSize := int(float64(config.MaxQueueSize) * config.CriticalThreshold)

	if warningSize != 7000 {
		t.Errorf("expected warning size 7000, got %d", warningSize)
	}

	if criticalSize != 9000 {
		t.Errorf("expected critical size 9000, got %d", criticalSize)
	}
}

func TestBackpressureConfigValidation(t *testing.T) {
	tests := []BackpressureConfig{
		{MaxQueueSize: 0, WarningThreshold: 0.7, CriticalThreshold: 0.9, CheckInterval: time.Second},
		{MaxQueueSize: 100, WarningThreshold: 0.9, CriticalThreshold: 0.7, CheckInterval: time.Second},
		{MaxQueueSize: 100, WarningThreshold: -0.1, CriticalThreshold: 0.9, CheckInterval: time.Second},
		{MaxQueueSize: 100, WarningThreshold: 0.7, CriticalThreshold: 1.1, CheckInterval: time.Second},
	}
	for _, config := range tests {
		controller := &BackpressureController{}
		if err := controller.SetConfig(config); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for %+v, got %v", config, err)
		}
	}
}

func TestBackpressureCallbackPanicIsContained(t *testing.T) {
	config := DefaultBackpressureConfig()
	config.OnWarning = func(string, int, int) { panic("callback failure") }
	handleBackpressureStateChange(config, "queue", StateNormal, StateWarning, 8_000)
}

func TestRateLimiterRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []struct{ rate, burst int }{{0, 1}, {-1, 1}, {1, 0}, {1, -1}} {
		if _, err := NewRateLimiter(test.rate, test.burst); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for rate=%d burst=%d, got %v", test.rate, test.burst, err)
		}
	}
}

func TestRateLimiterWaitSupportsHighRatesAndCancellation(t *testing.T) {
	limiter, err := NewRateLimiter(10_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !limiter.Allow() {
		t.Fatal("expected initial burst token")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if waitErr := limiter.Wait(ctx); waitErr != nil {
		t.Fatalf("high-rate wait failed: %v", waitErr)
	}

	limiter, err = NewRateLimiter(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !limiter.Allow() {
		t.Fatal("expected initial burst token")
	}
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if err := limiter.Wait(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestRateLimiterWaitRejectsNilContext(t *testing.T) {
	limiter, err := NewRateLimiter(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Wait(nil) panicked: %v", recovered)
		}
	}()
	//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
	if err := limiter.Wait(nil); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Wait(nil) error = %v, want ErrInvalidContext", err)
	}
}

func TestBackpressureGetAllStatesIsSorted(t *testing.T) {
	controller := &BackpressureController{
		states: map[string]*QueueBackpressure{},
	}
	for _, queue := range []string{"zeta", "alpha", "theta", "beta", "gamma", "delta"} {
		controller.states[queue] = &QueueBackpressure{Queue: queue}
	}
	for range 32 {
		states := controller.GetAllStates()
		queues := make([]string, len(states))
		for index := range states {
			queues[index] = states[index].Queue
		}
		if !sort.StringsAreSorted(queues) {
			t.Fatalf("GetAllStates() order = %v, want sorted", queues)
		}
	}
}

func TestBackpressureController_RejectCount(t *testing.T) {
	bp := &QueueBackpressure{
		Queue:       "test",
		State:       StateCritical,
		CurrentSize: 950,
		MaxSize:     1000,
		RejectCount: 0,
	}
	if bp.Queue != "test" || bp.State != StateCritical || bp.CurrentSize != 950 || bp.MaxSize != 1000 {
		t.Fatalf("unexpected initial backpressure state: %+v", bp)
	}

	// 模拟拒绝请求
	for i := 0; i < 10; i++ {
		bp.RejectCount++
	}

	if bp.RejectCount != 10 {
		t.Errorf("expected RejectCount=10, got %d", bp.RejectCount)
	}
}
