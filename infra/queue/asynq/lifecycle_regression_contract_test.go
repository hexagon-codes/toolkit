package asynq

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	hibasynq "github.com/hibiken/asynq"
)

func TestBackpressureStopWaitsForActiveMonitorOperation(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	ticker := newManualBackpressureTicker()
	operationEntered := make(chan struct{})
	stopObserved := make(chan struct{})
	releaseOperation := make(chan struct{})
	controller := newContractBackpressureController(manager, ticker)
	controller.inspectQueues = func(ctx context.Context, _ *Manager, _ *hibasynq.Inspector) error {
		close(operationEntered)
		<-ctx.Done()
		close(stopObserved)
		<-releaseOperation
		return ctx.Err()
	}

	if err := controller.Start(); err != nil {
		t.Fatalf("BackpressureController.Start() error = %v", err)
	}
	ticker.Tick()
	receiveContractSignal(t, operationEntered, "monitor operation entry")
	stopDone := make(chan struct{})
	go func() {
		controller.Stop()
		close(stopDone)
	}()
	receiveContractSignal(t, stopObserved, "monitor cancellation")

	controller.mu.RLock()
	runningWhileOperationBlocked := controller.running
	controller.mu.RUnlock()
	if !runningWhileOperationBlocked {
		close(releaseOperation)
		receiveContractSignal(t, stopDone, "premature Stop return")
		t.Fatal("BackpressureController.Stop() marked stopped before active monitor operation exited")
	}
	manager.mu.RLock()
	managerClosed := manager.closed
	manager.mu.RUnlock()
	if managerClosed {
		close(releaseOperation)
		receiveContractSignal(t, stopDone, "Stop return after manager close")
		t.Fatal("active backpressure monitor observed Manager closed before it exited")
	}
	if err := controller.Start(); !errors.Is(err, ErrBackpressureRunning) {
		close(releaseOperation)
		receiveContractSignal(t, stopDone, "Stop return after unsafe restart")
		t.Fatalf("Start() during Stop error = %v, want ErrBackpressureRunning", err)
	}

	close(releaseOperation)
	receiveContractSignal(t, stopDone, "BackpressureController.Stop completion")
	controller.mu.RLock()
	runningAfterStop := controller.running
	controller.mu.RUnlock()
	if runningAfterStop {
		t.Fatal("BackpressureController.Stop() returned while controller remained running")
	}
}

func TestBackpressureStartStopAreIdempotentAndRestartSafe(t *testing.T) {
	controller := &BackpressureController{
		config:       DefaultBackpressureConfig(),
		states:       make(map[string]*QueueBackpressure),
		rejectCounts: make(map[string]int64),
	}
	created := make(chan *manualBackpressureTicker, 2)
	var factoryCalls atomic.Int32
	controller.newTicker = func(time.Duration) backpressureTicker {
		factoryCalls.Add(1)
		ticker := newManualBackpressureTicker()
		created <- ticker
		return ticker
	}

	if err := controller.Start(); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := controller.Start(); err != nil {
		t.Fatalf("idempotent Start() error = %v", err)
	}
	first := receiveContractValue(t, created, "first ticker")
	if got := factoryCalls.Load(); got != 1 {
		t.Fatalf("two concurrent-safe Start calls created %d monitor loops, want 1", got)
	}
	controller.Stop()
	controller.Stop()
	receiveContractSignal(t, first.stopped, "first ticker stop")

	if err := controller.Start(); err != nil {
		t.Fatalf("restart Start() error = %v", err)
	}
	second := receiveContractValue(t, created, "second ticker")
	if got := factoryCalls.Load(); got != 2 {
		t.Fatalf("Start after completed Stop created %d total loops, want 2", got)
	}
	controller.Stop()
	receiveContractSignal(t, second.stopped, "second ticker stop")
}

func TestInitPollingCleanupWaitsForBackpressureMonitor(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	controller := resetBackpressureControllerForContract(t)
	ticker := newManualBackpressureTicker()
	controller.newTicker = func(time.Duration) backpressureTicker { return ticker }
	operationEntered := make(chan struct{})
	stopObserved := make(chan struct{})
	releaseOperation := make(chan struct{})
	controller.inspectQueues = func(ctx context.Context, _ *Manager, _ *hibasynq.Inspector) error {
		close(operationEntered)
		<-ctx.Done()
		close(stopObserved)
		<-releaseOperation
		return ctx.Err()
	}

	redisServer := miniredis.RunT(t)
	cleanup, err := InitPolling(
		context.Background(),
		PollingConfig{Enabled: true},
		managerTestConfig(redisServer.Addr()),
		WorkerDependencies{},
	)
	if err != nil {
		t.Fatalf("InitPolling() error = %v", err)
	}
	manager := GetManager()
	if manager == nil {
		t.Fatal("InitPolling() did not install Manager")
		return
	}

	ticker.Tick()
	receiveContractSignal(t, operationEntered, "polling monitor operation entry")
	cleanupDone := make(chan error, 1)
	go func() { cleanupDone <- cleanup() }()
	receiveContractSignal(t, stopObserved, "polling monitor cancellation")

	controller.mu.RLock()
	runningWhileOperationBlocked := controller.running
	controller.mu.RUnlock()
	manager.mu.RLock()
	managerClosed := manager.closed
	manager.mu.RUnlock()
	if !runningWhileOperationBlocked || managerClosed {
		close(releaseOperation)
		_ = receiveContractValue(t, cleanupDone, "polling cleanup after premature close")
		t.Fatalf("cleanup crossed active monitor: backpressure running=%v manager closed=%v", runningWhileOperationBlocked, managerClosed)
	}

	close(releaseOperation)
	if cleanupErr := receiveContractValue(t, cleanupDone, "polling cleanup completion"); cleanupErr != nil {
		t.Fatalf("cleanup() error = %v", cleanupErr)
	}
	manager.mu.RLock()
	managerClosed = manager.closed
	manager.mu.RUnlock()
	if !managerClosed {
		t.Fatal("cleanup() returned before closing Manager")
	}
}

func TestConcurrentManagerStopCallersShareCompletionAndError(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	wantErr := errors.New("contract stop cleanup failure")
	cleanupEntered := make(chan struct{})
	followerWaiting := make(chan struct{})
	releaseCleanup := make(chan struct{})
	manager.stopCleanupHook = func() error {
		close(cleanupEntered)
		<-releaseCleanup
		return wantErr
	}
	var followerOnce sync.Once
	manager.stopWaitHook = func() { followerOnce.Do(func() { close(followerWaiting) }) }
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- manager.Stop() }()
	receiveContractSignal(t, cleanupEntered, "manager stop cleanup entry")
	go func() { secondDone <- manager.Stop() }()
	receiveContractSignal(t, followerWaiting, "second Manager.Stop wait entry")
	select {
	case secondErr := <-secondDone:
		t.Fatalf("second Manager.Stop returned %v before first cleanup completed", secondErr)
	default:
	}
	close(releaseCleanup)

	firstErr := receiveContractValue(t, firstDone, "first Manager.Stop")
	secondErr := receiveContractValue(t, secondDone, "second Manager.Stop")
	if !errors.Is(firstErr, wantErr) || !errors.Is(secondErr, wantErr) {
		t.Fatalf("concurrent Stop errors = (%v, %v), want both errors.Is cleanup sentinel", firstErr, secondErr)
	}
	if thirdErr := manager.Stop(); !errors.Is(thirdErr, wantErr) {
		t.Fatalf("sequential Stop error = %v, want stored cleanup sentinel", thirdErr)
	}
}

func TestBackpressureConfigurationFailsFastAndFreezesWhileRunning(t *testing.T) {
	controller := &BackpressureController{
		config:       DefaultBackpressureConfig(),
		states:       make(map[string]*QueueBackpressure),
		rejectCounts: make(map[string]int64),
	}
	invalid := DefaultBackpressureConfig()
	invalid.CheckInterval = 0
	if err := controller.SetConfig(invalid); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("SetConfig(zero interval) error = %v, want ErrInvalidConfig", err)
	}
	controller.config.CheckInterval = 0
	if err := controller.Start(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Start(zero interval) error = %v, want ErrInvalidConfig", err)
	}

	controller.config = DefaultBackpressureConfig()
	controller.newTicker = func(time.Duration) backpressureTicker { return newManualBackpressureTicker() }
	if err := controller.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer controller.Stop()
	if err := controller.SetConfig(DefaultBackpressureConfig()); !errors.Is(err, ErrBackpressureRunning) {
		t.Fatalf("SetConfig() while running error = %v, want ErrBackpressureRunning", err)
	}
}

func TestBackpressureStopLogsOutsideControllerLock(t *testing.T) {
	controller := &BackpressureController{
		config:       DefaultBackpressureConfig(),
		states:       make(map[string]*QueueBackpressure),
		rejectCounts: make(map[string]int64),
		newTicker:    func(time.Duration) backpressureTicker { return newManualBackpressureTicker() },
	}
	lockAvailable := make(chan bool, 1)
	SetLogger(&backpressureReentrantLogger{
		controller: controller, lockAvailable: lockAvailable, messagePart: "Controller stopped",
	})
	t.Cleanup(func() { SetLogger(&StdLogger{}) })
	if err := controller.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	controller.Stop()
	if available := receiveContractValue(t, lockAvailable, "stopped logger lock probe"); !available {
		t.Fatal("BackpressureController.Stop() called Logger while holding controller lock")
	}
}

func TestBackpressureStateChangeLogsOutsideControllerLock(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	if _, err := manager.EnqueueTask(context.Background(), "contract:backpressure", nil); err != nil {
		t.Fatalf("EnqueueTask() error = %v", err)
	}
	controller := &BackpressureController{
		config: BackpressureConfig{
			MaxQueueSize:      1,
			WarningThreshold:  0.5,
			CriticalThreshold: 1,
			CheckInterval:     time.Second,
		},
		states:       make(map[string]*QueueBackpressure),
		rejectCounts: make(map[string]int64),
	}
	lockAvailable := make(chan bool, 1)
	SetLogger(&backpressureReentrantLogger{
		controller: controller, lockAvailable: lockAvailable, messagePart: "entering CRITICAL",
	})
	t.Cleanup(func() { SetLogger(&StdLogger{}) })
	controller.checkQueue(QueueDefault, manager.GetInspector())
	if available := receiveContractValue(t, lockAvailable, "state-change logger lock probe"); !available {
		t.Fatal("BackpressureController.checkQueue() called Logger while holding controller lock")
	}
}

func TestHealthCheckFailsClosedWhenQueueInspectionDenied(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	if startErr := manager.Start(context.Background()); startErr != nil {
		t.Fatalf("Manager.Start() error = %v", startErr)
	}

	checker := NewHealthChecker(manager)
	checker.checkQueuesFn = func(context.Context, *HealthStatus) error {
		return errors.New("NOPERM queue inspection denied")
	}
	status := checker.Check(context.Background())
	if status.Details["redis"] != "connected" {
		t.Fatalf("Redis PING status = %q, want connected", status.Details["redis"])
	}
	if status.Healthy || status.Ready {
		t.Fatalf("queue inspection denial status = healthy:%v ready:%v, want both false", status.Healthy, status.Ready)
	}
	if status.Details["queues"] == "" {
		t.Fatal("queue inspection denial omitted queues detail")
	}
}

func TestRegisterScheduleAfterStartFailsWithoutMutation(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	if startErr := manager.Start(context.Background()); startErr != nil {
		t.Fatalf("Manager.Start() error = %v", startErr)
	}
	wantSchedules := len(manager.schedules)
	err = manager.RegisterSchedule("@every 1h", hibasynq.NewTask("contract:late-schedule", nil))
	if !errors.Is(err, ErrManagerStarted) {
		t.Fatalf("RegisterSchedule() after Start error = %v, want ErrManagerStarted", err)
	}
	if got := len(manager.schedules); got != wantSchedules {
		t.Fatalf("RegisterSchedule() after Start mutated schedules count to %d, want %d", got, wantSchedules)
	}
}

func TestMetricsContextAndStoppedManagerContracts(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := UpdateQueueMetrics(nil, manager); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // Contract test verifies nil-context rejection.
		t.Fatalf("UpdateQueueMetrics(nil) error = %v, want ErrInvalidContext", err)
	}
	if err := StartMetricsUpdater(nil, manager, time.Second); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // Contract test verifies nil-context rejection.
		t.Fatalf("StartMetricsUpdater(nil) error = %v, want ErrInvalidContext", err)
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := UpdateQueueMetrics(canceledCtx, manager); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateQueueMetrics(canceled) error = %v, want context.Canceled", err)
	}
	if err := StartMetricsUpdater(canceledCtx, manager, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("StartMetricsUpdater(canceled) error = %v, want context.Canceled", err)
	}
	if err := StartMetricsUpdater(context.Background(), manager, 0); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("StartMetricsUpdater(zero interval) error = %v, want ErrInvalidConfig", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Manager.Stop() error = %v", err)
	}
	if err := StartMetricsUpdater(context.Background(), manager, time.Second); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("StartMetricsUpdater(stopped manager) error = %v, want ErrManagerStopped", err)
	}
}

func TestSendToDeadLetterUsesManagerQueueNamespace(t *testing.T) {
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "tenant-a:"
	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	dlq := &DeadLetterManager{manager: manager}
	if sendErr := dlq.SendToDeadLetter(context.Background(), "task-1", map[string]string{"ok": "true"}, "contract"); sendErr != nil {
		t.Fatalf("SendToDeadLetter() error = %v", sendErr)
	}
	inspector := manager.GetInspector()
	info, err := inspector.GetQueueInfo("tenant-a:dead_letter")
	if err != nil {
		t.Fatalf("GetQueueInfo(prefixed dead-letter) error = %v", err)
	}
	if info.Pending != 1 {
		t.Fatalf("prefixed dead-letter pending = %d, want 1", info.Pending)
	}
	if _, err := inspector.ListPendingTasks(QueueDeadLetter, hibasynq.PageSize(1)); !errors.Is(err, hibasynq.ErrQueueNotFound) {
		t.Fatalf("unprefixed dead-letter queue error = %v, want ErrQueueNotFound", err)
	}
}

type manualBackpressureTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

type backpressureReentrantLogger struct {
	StdLogger
	controller    *BackpressureController
	lockAvailable chan<- bool
	messagePart   string
}

func (l *backpressureReentrantLogger) Log(msg string) {
	if !strings.Contains(msg, l.messagePart) {
		return
	}
	available := l.controller.mu.TryRLock()
	if available {
		l.controller.mu.RUnlock()
	}
	l.lockAvailable <- available
}

func newManualBackpressureTicker() *manualBackpressureTicker {
	return &manualBackpressureTicker{
		ticks:   make(chan time.Time, 1),
		stopped: make(chan struct{}),
	}
}

func (t *manualBackpressureTicker) Chan() <-chan time.Time { return t.ticks }

func (t *manualBackpressureTicker) Stop() {
	t.once.Do(func() { close(t.stopped) })
}

func (t *manualBackpressureTicker) Tick() {
	select {
	case t.ticks <- time.Now():
	case <-time.After(5 * time.Second):
		panic("timed out delivering manual backpressure tick")
	}
}

func newContractBackpressureController(manager *Manager, ticker backpressureTicker) *BackpressureController {
	return &BackpressureController{
		config:       DefaultBackpressureConfig(),
		manager:      manager,
		states:       make(map[string]*QueueBackpressure),
		rejectCounts: make(map[string]int64),
		newTicker:    func(time.Duration) backpressureTicker { return ticker },
	}
}

func resetBackpressureControllerForContract(t *testing.T) *BackpressureController {
	t.Helper()
	if backpressureController != nil {
		backpressureController.Stop()
	}
	backpressureController = nil
	backpressureControllerOnce = sync.Once{}
	controller := GetBackpressureController()
	t.Cleanup(func() {
		controller.Stop()
		controller.SetManager(nil)
		backpressureController = nil
		backpressureControllerOnce = sync.Once{}
	})
	return controller
}

func receiveContractSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func receiveContractValue[T any](t *testing.T, values <-chan T, description string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}
