package asynq

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	hibasynq "github.com/hibiken/asynq"
)

func TestManagerRejectsInvalidRuntimeConfigurationBeforeStart(t *testing.T) {
	redisServer := miniredis.RunT(t)
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "negative concurrency",
			mutate: func(config *Config) {
				config.Concurrency = -1
			},
		},
		{
			name: "invalid log level",
			mutate: func(config *Config) {
				config.LogLevel = hibasynq.LogLevel(100)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := managerTestConfig(redisServer.Addr())
			test.mutate(&config)
			manager, err := NewManager(context.Background(), config)
			if manager != nil {
				t.Cleanup(func() { _ = manager.Stop() })
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewManager() error = %v, want ErrInvalidConfig", err)
			}
			if manager != nil {
				t.Fatalf("NewManager() manager = %v, want nil", manager)
			}
		})
	}
}

func TestManagerHandlerRegistrationReturnsStableErrorsInsteadOfPanicking(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	handler := func(context.Context, *hibasynq.Task) error { return nil }
	if err := manager.RegisterHandler("", handler); !errors.Is(err, ErrInvalidHandler) {
		t.Fatalf("RegisterHandler(empty type) error = %v, want ErrInvalidHandler", err)
	}
	if err := manager.RegisterHandler("audit:handler", nil); !errors.Is(err, ErrInvalidHandler) {
		t.Fatalf("RegisterHandler(nil handler) error = %v, want ErrInvalidHandler", err)
	}
	if err := manager.RegisterHandler("audit:handler", handler); err != nil {
		t.Fatalf("first RegisterHandler() error = %v", err)
	}
	if err := manager.RegisterHandler("audit:handler", handler); !errors.Is(err, ErrHandlerAlreadyRegistered) {
		t.Fatalf("duplicate RegisterHandler() error = %v, want ErrHandlerAlreadyRegistered", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := manager.RegisterHandler("audit:late", handler); !errors.Is(err, ErrManagerStarted) {
		t.Fatalf("RegisterHandler() after Start error = %v, want ErrManagerStarted", err)
	}
}

func TestManagerMiddlewareRegistrationContainsCallbackPanics(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.WithMiddleware(nil); !errors.Is(err, ErrInvalidMiddleware) {
		t.Fatalf("WithMiddleware(nil) error = %v, want ErrInvalidMiddleware", err)
	}
	panicErr := errors.New("middleware callback failed")
	if err := manager.WithMiddleware(func(hibasynq.Handler) hibasynq.Handler {
		panic(panicErr)
	}); err != nil {
		t.Fatalf("WithMiddleware(panicking callback) error = %v", err)
	}
	handler := func(context.Context, *hibasynq.Task) error { return nil }
	if err := manager.RegisterHandlerWithMiddleware("audit:panic", handler); !errors.Is(err, panicErr) {
		t.Fatalf("RegisterHandlerWithMiddleware() error = %v, want panic error chain", err)
	}
	manager.mu.RLock()
	handlerCount := len(manager.handlers)
	manager.mu.RUnlock()
	if handlerCount != 0 {
		t.Fatalf("handler count after middleware panic = %d, want 0", handlerCount)
	}

	if err := manager.WithMiddleware(func(hibasynq.Handler) hibasynq.Handler {
		return hibasynq.HandlerFunc(nil)
	}); err != nil {
		t.Fatalf("WithMiddleware(typed nil result) error = %v", err)
	}
	if err := manager.RegisterHandlerWithMiddleware("audit:typed-nil", handler); !errors.Is(err, ErrInvalidMiddleware) {
		t.Fatalf("RegisterHandlerWithMiddleware(typed nil result) error = %v, want ErrInvalidMiddleware", err)
	}

	if err := manager.WithMiddleware(func(next hibasynq.Handler) hibasynq.Handler { return next }); err != nil {
		t.Fatalf("WithMiddleware(identity) error = %v", err)
	}
	if err := manager.RegisterHandlerWithMiddleware("audit:wrapped", handler); err != nil {
		t.Fatalf("first RegisterHandlerWithMiddleware() error = %v", err)
	}
	if err := manager.RegisterHandlerWithMiddleware("audit:wrapped", handler); !errors.Is(err, ErrHandlerAlreadyRegistered) {
		t.Fatalf("duplicate RegisterHandlerWithMiddleware() error = %v, want ErrHandlerAlreadyRegistered", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := manager.WithMiddleware(func(next hibasynq.Handler) hibasynq.Handler { return next }); !errors.Is(err, ErrManagerStarted) {
		t.Fatalf("WithMiddleware() after Start error = %v, want ErrManagerStarted", err)
	}
}

func TestDeadLetterManagerUsesStableLifecycleSentinel(t *testing.T) {
	manager := &DeadLetterManager{}
	err := manager.SendToDeadLetter(context.Background(), "task-1", nil, "audit")
	if !errors.Is(err, ErrManagerNotInitialized) {
		t.Fatalf("SendToDeadLetter() error = %v, want ErrManagerNotInitialized", err)
	}
}

func TestDeadLetterManagerEnqueuesThePublicDeadLetterTaskType(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	deadLetters := &DeadLetterManager{manager: manager}
	if sendErr := deadLetters.SendToDeadLetter(context.Background(), "task-1", map[string]bool{"ok": false}, "audit"); sendErr != nil {
		t.Fatalf("SendToDeadLetter() error = %v", sendErr)
	}
	tasks, err := manager.GetInspector().ListPendingTasks(manager.QueueName(QueueDeadLetter), hibasynq.PageSize(10))
	if err != nil {
		t.Fatalf("ListPendingTasks() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("pending dead-letter tasks = %d, want 1", len(tasks))
	}
	if tasks[0].Type != TaskTypeDeadLetter {
		t.Fatalf("dead-letter task type = %q, want %q", tasks[0].Type, TaskTypeDeadLetter)
	}
}

func TestHealthCheckerReturnsIndependentStatusSnapshots(t *testing.T) {
	checker := NewHealthChecker(nil)
	status := checker.Check(context.Background())
	status.Healthy = true
	status.Ready = true
	status.Details["manager"] = "mutated"

	stored := checker.GetLastStatus()
	if stored == nil {
		t.Fatal("GetLastStatus() = nil, want status")
	}
	if stored.Healthy || stored.Ready {
		t.Fatalf("stored status = healthy:%t ready:%t, want false/false", stored.Healthy, stored.Ready)
	}
	if got := stored.Details["manager"]; got != "not started" {
		t.Fatalf("stored manager detail = %q, want %q", got, "not started")
	}

	stored.Details["manager"] = "mutated again"
	again := checker.GetLastStatus()
	if got := again.Details["manager"]; got != "not started" {
		t.Fatalf("second manager detail = %q, want %q", got, "not started")
	}
}

func TestHealthCheckerQueueInspectionHonorsContext(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	checker := NewHealthChecker(manager)
	status := &HealthStatus{Details: make(map[string]string)}

	if err := checker.checkQueues(nil, status); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // 回归测试验证空 context 会被拒绝。
		t.Fatalf("checkQueues(nil) error = %v, want ErrInvalidContext", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := checker.checkQueues(canceled, status); !errors.Is(err, context.Canceled) {
		t.Fatalf("checkQueues(canceled) error = %v, want context.Canceled", err)
	}
}

func TestGracefulShutdownContainsCallbackPanicsAndStillStops(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	shutdown := NewGracefulShutdown(manager, time.Second)
	panicErr := errors.New("shutdown callback failed")
	shutdown.OnShutdown(func() { panic(panicErr) })
	secondCalled := false
	shutdown.OnShutdown(func() { secondCalled = true })

	err = shutdown.Shutdown(context.Background())
	if !errors.Is(err, panicErr) {
		t.Fatalf("Shutdown() error = %v, want callback panic error chain", err)
	}
	if !secondCalled {
		t.Fatal("Shutdown() skipped callbacks after a panic")
	}
	if _, enqueueErr := manager.Enqueue(context.Background(), hibasynq.NewTask("audit:stopped", nil)); !errors.Is(enqueueErr, ErrManagerStopped) {
		t.Fatalf("Enqueue() after Shutdown error = %v, want ErrManagerStopped", enqueueErr)
	}
}
