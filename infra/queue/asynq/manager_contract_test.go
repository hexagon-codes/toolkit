package asynq

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	queue "github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

func TestNewManagerVerifiesACLBeforeReturning(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisServer.RequireUserAuth("queue-worker", "correct-secret")

	config := managerTestConfig(redisServer.Addr())
	config.Redis.DataCredentials = redisconn.Credentials{
		Username: "queue-worker",
		Password: "wrong-secret",
	}

	manager, err := NewManager(context.Background(), config)
	if err == nil {
		if manager != nil {
			_ = manager.Stop()
		}
		t.Fatal("NewManager() error = nil, want startup authentication failure")
	}
	if manager != nil {
		t.Fatalf("NewManager() manager = %v, want nil after failed Redis PING", manager)
	}
}

func TestNewManagerOwnsVerifiedRedisClientAndStopClosesIt(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisServer.RequireUserAuth("queue-worker", "correct-secret")

	config := managerTestConfig(redisServer.Addr())
	config.Redis.DataCredentials = redisconn.Credentials{
		Username: "queue-worker",
		Password: "correct-secret",
	}

	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.redisClient.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("manager Redis PING before Stop() error = %v", err)
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
	if err := manager.redisClient.Ping(context.Background()).Err(); err == nil {
		t.Fatal("manager Redis PING after Stop() error = nil, want closed client")
	}
}

func TestManagerACLWorksForAsynqEnqueueAndLease(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisServer.RequireUserAuth("queue-worker", "correct-secret")
	config := managerTestConfig(redisServer.Addr())
	config.Redis.DataCredentials = redisconn.Credentials{
		Username: "queue-worker",
		Password: "correct-secret",
	}

	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	info, err := manager.EnqueueTask(
		context.Background(),
		"contract:acl",
		[]byte(`{"ok":true}`),
		queue.Queue(QueueDefault),
	)
	if err != nil {
		t.Fatalf("EnqueueTask() error = %v", err)
	}
	if info.Queue != QueueDefault {
		t.Fatalf("EnqueueTask() queue = %q, want %q", info.Queue, QueueDefault)
	}
	lease, acquired, err := manager.AcquirePollingLease(context.Background(), "acl-task")
	if err != nil || !acquired || lease == nil {
		t.Fatalf("AcquirePollingLease() = (%v, %v, %v), want lease, true, nil", lease, acquired, err)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("lease Release() error = %v", err)
	}
}

func TestInitManagerRejectsSecondInitialization(t *testing.T) {
	ResetManagerForTesting()
	t.Cleanup(ResetManagerForTesting)
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())

	first, err := InitManager(context.Background(), config)
	if err != nil {
		t.Fatalf("first InitManager() error = %v", err)
	}
	second, err := InitManager(context.Background(), config)
	if !errors.Is(err, ErrManagerAlreadyInitialized) {
		t.Fatalf("second InitManager() error = %v, want ErrManagerAlreadyInitialized", err)
	}
	if second != nil {
		t.Fatalf("second InitManager() manager = %v, want nil", second)
	}
	if GetManager() != first {
		t.Fatal("failed second initialization replaced the global manager")
	}
}

func TestSetLoggerCanReplaceDifferentConcreteImplementations(t *testing.T) {
	first := &firstTestLogger{}
	second := &secondTestLogger{}
	SetLogger(first)
	SetLogger(second)
	if GetLogger() != second {
		t.Fatal("GetLogger() did not expose the replacement logger")
	}
}

func TestManagersKeepQueuePrefixesIsolated(t *testing.T) {
	redisServer := miniredis.RunT(t)
	firstConfig := managerTestConfig(redisServer.Addr())
	firstConfig.QueuePrefix = "tenant-a:"
	secondConfig := managerTestConfig(redisServer.Addr())
	secondConfig.QueuePrefix = "tenant-b:"

	first, err := NewManager(context.Background(), firstConfig)
	if err != nil {
		t.Fatalf("first NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Stop() })
	second, err := NewManager(context.Background(), secondConfig)
	if err != nil {
		t.Fatalf("second NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Stop() })

	if got := first.QueueName(QueueHigh); got != "tenant-a:high" {
		t.Fatalf("first QueueName(high) = %q, want tenant-a:high", got)
	}
	if got := second.QueueName(QueueHigh); got != "tenant-b:high" {
		t.Fatalf("second QueueName(high) = %q, want tenant-b:high", got)
	}
	if QueueHigh != "high" {
		t.Fatalf("package queue base changed to %q, want immutable high", QueueHigh)
	}
	if _, ok := first.config.Queues["tenant-a:high"]; !ok {
		t.Fatalf("first manager queues = %v, want tenant-a:high", first.config.Queues)
	}
	if _, ok := second.config.Queues["tenant-b:high"]; !ok {
		t.Fatalf("second manager queues = %v, want tenant-b:high", second.config.Queues)
	}

	var waitGroup sync.WaitGroup
	for index := 0; index < 20; index++ {
		waitGroup.Add(2)
		go func() {
			defer waitGroup.Done()
			if got := first.QueueName(QueueDefault); got != "tenant-a:default" {
				t.Errorf("concurrent first QueueName(default) = %q", got)
			}
		}()
		go func() {
			defer waitGroup.Done()
			if got := second.QueueName(QueueDefault); got != "tenant-b:default" {
				t.Errorf("concurrent second QueueName(default) = %q", got)
			}
		}()
	}
	waitGroup.Wait()
}

func TestQueuePrefixAlwaysPrependsIntersectingBaseName(t *testing.T) {
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "prod"
	config.Queues = map[string]int{"production": 1}

	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	if got := manager.QueueName("production"); got != "prodproduction" {
		t.Fatalf("QueueName(production) = %q, want prodproduction", got)
	}
	if _, ok := manager.config.Queues["prodproduction"]; !ok {
		t.Fatalf("normalized queues = %v, want prodproduction", manager.config.Queues)
	}
	if _, escaped := manager.config.Queues["production"]; escaped {
		t.Fatalf("normalized queues = %v, unprefixed production escaped namespace", manager.config.Queues)
	}
}

func TestNewManagerRejectsInvalidQueueConfiguration(t *testing.T) {
	redisServer := miniredis.RunT(t)
	tests := []struct {
		name   string
		queues map[string]int
	}{
		{name: "blank name", queues: map[string]int{" ": 1}},
		{name: "zero priority", queues: map[string]int{"events": 0}},
		{name: "negative priority", queues: map[string]int{"events": -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := managerTestConfig(redisServer.Addr())
			config.Queues = test.queues
			manager, err := NewManager(context.Background(), config)
			if !errors.Is(err, ErrInvalidConfig) {
				if manager != nil {
					_ = manager.Stop()
				}
				t.Fatalf("NewManager() error = %v, want ErrInvalidConfig", err)
			}
			if manager != nil {
				t.Fatalf("NewManager() manager = %v, want nil", manager)
			}
		})
	}
}

func TestEnqueueRejectsInvalidInputsAndUseAfterStop(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	task := queue.NewTask("contract:enqueue", nil)

	if _, err := manager.Enqueue(nil, task); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // Contract test verifies nil-context rejection.
		t.Fatalf("Enqueue(nil context) error = %v, want ErrInvalidContext", err)
	}
	if _, err := manager.Enqueue(context.Background(), nil); !errors.Is(err, ErrInvalidTask) {
		t.Fatalf("Enqueue(nil task) error = %v, want ErrInvalidTask", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := manager.Enqueue(context.Background(), task); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("Enqueue() after Stop error = %v, want ErrManagerStopped", err)
	}
	if _, err := manager.EnqueueTask(context.Background(), "contract:enqueue", nil); !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("EnqueueTask() after Stop error = %v, want ErrManagerStopped", err)
	}
}

func TestRedisConnOptCannotCreateLiveClientAfterStop(t *testing.T) {
	redisServer := miniredis.RunT(t)
	manager, err := NewManager(context.Background(), managerTestConfig(redisServer.Addr()))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	redisOpt := manager.GetRedisOpt()
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	client, ok := redisOpt.MakeRedisClient().(redis.UniversalClient)
	if !ok {
		t.Fatalf("MakeRedisClient() type = %T, want redis.UniversalClient", client)
	}
	if err := client.Ping(context.Background()).Err(); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("client created after Stop PING error = %v, want redis.ErrClosed", err)
	}
}

func managerTestConfig(addr string) Config {
	config := DefaultConfig(redisconn.DefaultConfig(redisconn.ModeSingle, addr))
	config.Concurrency = 1
	return config
}

type firstTestLogger struct{ StdLogger }
type secondTestLogger struct{ StdLogger }
