package asynq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	queue "github.com/hibiken/asynq"
	dto "github.com/prometheus/client_model/go"
)

func TestInspectorHelpersReturnManagerStoppedAfterStop(t *testing.T) {
	tests := []struct {
		name string
		call func(manager *Manager) error
	}{
		{
			name: "queue stats",
			call: func(_ *Manager) error {
				_, err := GetQueueStats()
				return err
			},
		},
		{
			name: "list dead letter",
			call: func(_ *Manager) error {
				_, err := GetDeadLetterTasks(QueueDefault, 10)
				return err
			},
		},
		{
			name: "retry dead letter",
			call: func(_ *Manager) error {
				return RetryDeadLetterTask(QueueDefault, "task-1")
			},
		},
		{
			name: "delete dead letter",
			call: func(_ *Manager) error {
				return DeleteDeadLetterTask(QueueDefault, "task-1")
			},
		},
		{
			name: "metrics",
			call: func(manager *Manager) error {
				return UpdateQueueMetrics(context.Background(), manager)
			},
		},
		{
			name: "health redis",
			call: func(manager *Manager) error {
				return NewHealthChecker(manager).checkRedis(context.Background())
			},
		},
		{
			name: "health queues",
			call: func(manager *Manager) error {
				status := &HealthStatus{Details: make(map[string]string)}
				return NewHealthChecker(manager).checkQueues(context.Background(), status)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newStoppedGlobalManager(t)
			if err := test.call(manager); !errors.Is(err, ErrManagerStopped) {
				t.Fatalf("helper error = %v, want ErrManagerStopped", err)
			}
		})
	}
}

func TestQueueStatsHealthAndMetricsStayInsideManagerNamespace(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "tenant-a:"
	manager, err := InitManager(context.Background(), config)
	if err != nil {
		t.Fatalf("InitManager() error = %v", err)
	}

	nativeClient := queue.NewClient(manager.GetRedisOpt())
	t.Cleanup(func() { _ = nativeClient.Close() })
	for _, queueName := range []string{"tenant-a:default", "tenant-b:default"} {
		_, enqueueErr := nativeClient.Enqueue(
			queue.NewTask("contract:namespace-stats", nil),
			queue.Queue(queueName),
		)
		if enqueueErr != nil {
			t.Fatalf("native enqueue to %q error = %v", queueName, enqueueErr)
		}
	}

	stats, err := GetQueueStats()
	if err != nil {
		t.Fatalf("GetQueueStats() error = %v", err)
	}
	if len(stats) != len(manager.config.Queues) {
		t.Fatalf("GetQueueStats() count = %d, want %d configured queues", len(stats), len(manager.config.Queues))
	}
	statsByName := make(map[string]QueueStats, len(stats))
	for _, stat := range stats {
		if !strings.HasPrefix(stat.Name, "tenant-a:") {
			t.Fatalf("GetQueueStats() leaked foreign queue %q", stat.Name)
		}
		statsByName[stat.Name] = stat
	}
	for queueName := range manager.config.Queues {
		stat, ok := statsByName[queueName]
		if !ok {
			t.Fatalf("GetQueueStats() omitted configured queue %q", queueName)
		}
		if queueName == "tenant-a:default" {
			if stat.Pending != 1 {
				t.Fatalf("tenant-a:default pending = %d, want 1", stat.Pending)
			}
			continue
		}
		if stat.Pending != 0 || stat.Active != 0 || stat.Scheduled != 0 || stat.Retry != 0 || stat.Archived != 0 || stat.Completed != 0 {
			t.Fatalf("uncreated configured queue %q stats = %+v, want zero", queueName, stat)
		}
	}

	status := &HealthStatus{Details: make(map[string]string)}
	if err := NewHealthChecker(manager).checkQueues(context.Background(), status); err != nil {
		t.Fatalf("HealthChecker.checkQueues() error = %v", err)
	}
	for key := range status.Details {
		if strings.Contains(key, "tenant-b:") {
			t.Fatalf("health details leaked foreign queue key %q", key)
		}
	}

	foreignGauge := QueueSizeGauge.WithLabelValues("tenant-b:default")
	foreignGauge.Set(777)
	if err := UpdateQueueMetrics(context.Background(), manager); err != nil {
		t.Fatalf("UpdateQueueMetrics() error = %v", err)
	}
	metric := &dto.Metric{}
	if err := foreignGauge.Write(metric); err != nil {
		t.Fatalf("foreign gauge Write() error = %v", err)
	}
	if got := metric.GetGauge().GetValue(); got != 777 {
		t.Fatalf("foreign queue gauge = %v, want untouched sentinel 777", got)
	}
}

func TestInspectorHelpersDoNotRaceManagerStop(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	manager, err := InitManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("InitManager() error = %v", err)
	}
	if manager.GetInspector() == nil {
		t.Fatal("GetInspector() = nil before Stop")
	}

	const readers = 32
	start := make(chan struct{})
	results := make(chan error, readers)
	var waitGroup sync.WaitGroup
	for index := 0; index < readers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- fmt.Errorf("panic: %v", recovered)
				}
			}()
			<-start
			_, helperErr := GetQueueStats()
			results <- helperErr
		}()
	}
	stopDone := make(chan error, 1)
	go func() {
		<-start
		stopDone <- manager.Stop()
	}()
	close(start)
	waitGroup.Wait()
	close(results)
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	for err := range results {
		if err != nil && !errors.Is(err, ErrManagerStopped) {
			t.Errorf("concurrent GetQueueStats() error = %v, want nil or ErrManagerStopped", err)
		}
	}
	if _, err := GetQueueStats(); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("GetQueueStats() after concurrent Stop error = %v, want ErrManagerStopped", err)
	}
}

func TestInitPollingCleanupDoesNotRaceInspectorHelpers(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
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

	const readers = 16
	start := make(chan struct{})
	results := make(chan error, readers)
	var waitGroup sync.WaitGroup
	for index := 0; index < readers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- fmt.Errorf("panic: %v", recovered)
				}
			}()
			<-start
			_, helperErr := GetQueueStats()
			results <- helperErr
		}()
	}
	cleanupDone := make(chan error, 1)
	go func() {
		<-start
		cleanupDone <- cleanup()
	}()
	close(start)
	waitGroup.Wait()
	close(results)
	if err := <-cleanupDone; err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	for err := range results {
		if err != nil && !errors.Is(err, ErrManagerStopped) && !errors.Is(err, ErrManagerNotInitialized) {
			t.Errorf("concurrent GetQueueStats() error = %v, want nil or stable lifecycle sentinel", err)
		}
	}
}

func TestDeadLetterHelpersResolveOnlyConfiguredBaseQueues(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "tenant-a:"
	manager, err := InitManager(context.Background(), config)
	if err != nil {
		t.Fatalf("InitManager() error = %v", err)
	}

	tasks, err := GetDeadLetterTasks(QueueDefault, 10)
	if err != nil {
		t.Fatalf("GetDeadLetterTasks(base default) error = %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("GetDeadLetterTasks(base default) count = %d, want 0", len(tasks))
	}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "list foreign namespace",
			call: func() error {
				_, err := GetDeadLetterTasks("tenant-b:default", 10)
				return err
			},
		},
		{
			name: "retry foreign namespace",
			call: func() error {
				return RetryDeadLetterTask("tenant-b:default", "task-1")
			},
		},
		{
			name: "delete foreign namespace",
			call: func() error {
				return DeleteDeadLetterTask("tenant-b:default", "task-1")
			},
		},
		{
			name: "reject pre-resolved own queue",
			call: func() error {
				_, err := GetDeadLetterTasks(manager.QueueName(QueueDefault), 10)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrQueueNotFound) {
				t.Fatalf("dead-letter helper error = %v, want ErrQueueNotFound", err)
			}
		})
	}
}

func TestLifecycleNilContextsWrapErrInvalidContext(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if startErr := manager.Start(nil); !errors.Is(startErr, ErrInvalidContext) { //nolint:staticcheck // Contract test verifies nil-context rejection.
		t.Fatalf("Manager.Start(nil) error = %v, want ErrInvalidContext", startErr)
	}
	shutdown := NewGracefulShutdown(manager, 0)
	callbackCalled := false
	shutdown.OnShutdown(func() { callbackCalled = true })
	if shutdownErr := shutdown.Shutdown(nil); !errors.Is(shutdownErr, ErrInvalidContext) { //nolint:staticcheck // Contract test verifies nil-context rejection.
		t.Fatalf("GracefulShutdown.Shutdown(nil) error = %v, want ErrInvalidContext", shutdownErr)
	}
	if callbackCalled {
		t.Fatal("GracefulShutdown.Shutdown(nil) executed shutdown callback")
	}

	ResetManagerForTesting()
	_, err = InitPolling(
		nil, //nolint:staticcheck // Contract test verifies nil-context rejection.
		PollingConfig{Enabled: true},
		managerTestConfig(redisServer.Addr()),
		WorkerDependencies{},
	)
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("InitPolling(nil) error = %v, want ErrInvalidContext", err)
	}
	_, err = InitPolling(
		nil, //nolint:staticcheck // Contract test verifies nil-context rejection before disabled fast-path.
		PollingConfig{Enabled: false},
		managerTestConfig(redisServer.Addr()),
		WorkerDependencies{},
	)
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("InitPolling(nil, disabled) error = %v, want ErrInvalidContext", err)
	}
}

func TestHealthRedisCheckHonorsCanceledContext(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = NewHealthChecker(manager).checkRedis(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("HealthChecker.checkRedis(canceled) error = %v, want context.Canceled", err)
	}
}

func newStoppedGlobalManager(t *testing.T) *Manager {
	t.Helper()
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	manager, err := InitManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("InitManager() error = %v", err)
	}
	if manager.GetInspector() == nil {
		t.Fatal("GetInspector() = nil before Stop")
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	return manager
}
