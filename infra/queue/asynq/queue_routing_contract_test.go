package asynq

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	queue "github.com/hibiken/asynq"
)

func TestManagerEnqueueRoutesQueuesWithinOwnNamespace(t *testing.T) {
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "tenant-a:"
	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	// Manager 选项是路由权威；来自其他命名空间的任务内嵌选项不得绕过管理器的默认队列。
	embeddedForeignQueue := queue.NewTask(
		"contract:implicit-default",
		nil,
		queue.Queue("tenant-b:high"),
	)
	implicitInfo, err := manager.Enqueue(context.Background(), embeddedForeignQueue)
	if err != nil {
		t.Fatalf("implicit Enqueue() error = %v", err)
	}
	if implicitInfo.Queue != "tenant-a:default" {
		t.Fatalf("implicit Enqueue() queue = %q, want tenant-a:default", implicitInfo.Queue)
	}

	explicitInfo, err := manager.Enqueue(
		context.Background(),
		queue.NewTask("contract:explicit-base", nil),
		queue.Queue(QueueHigh),
	)
	if err != nil {
		t.Fatalf("explicit base Enqueue() error = %v", err)
	}
	if explicitInfo.Queue != "tenant-a:high" {
		t.Fatalf("explicit base Enqueue() queue = %q, want tenant-a:high", explicitInfo.Queue)
	}

	_, err = manager.Enqueue(
		context.Background(),
		queue.NewTask("contract:pre-namespaced", nil),
		queue.Queue(manager.QueueName(QueueHigh)),
	)
	if !errors.Is(err, ErrQueueNotFound) {
		t.Fatalf("pre-namespaced Enqueue() error = %v, want ErrQueueNotFound", err)
	}

	inspector := manager.GetInspector()
	defaultInfo, err := inspector.GetQueueInfo("tenant-a:default")
	if err != nil {
		t.Fatalf("GetQueueInfo(tenant-a:default) error = %v", err)
	}
	if defaultInfo.Pending != 1 {
		t.Fatalf("tenant-a:default pending = %d, want 1", defaultInfo.Pending)
	}
	highInfo, err := inspector.GetQueueInfo("tenant-a:high")
	if err != nil {
		t.Fatalf("GetQueueInfo(tenant-a:high) error = %v", err)
	}
	if highInfo.Pending != 1 {
		t.Fatalf("tenant-a:high pending = %d, want 1", highInfo.Pending)
	}
	foreignTasks, err := inspector.ListPendingTasks("tenant-b:high", queue.PageSize(1))
	if err != nil && !errors.Is(err, queue.ErrQueueNotFound) {
		t.Fatalf("ListPendingTasks(tenant-b:high) error = %v", err)
	}
	if len(foreignTasks) != 0 {
		t.Fatalf("tenant-b:high pending tasks = %d, want 0", len(foreignTasks))
	}
}

func TestTaskBuilderPassesBaseQueueToManagerCanonicalizer(t *testing.T) {
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "tenant-a:"
	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	info, err := NewTask("contract:builder").Queue(QueueHigh).EnqueueWith(context.Background(), manager)
	if err != nil {
		t.Fatalf("TaskBuilder.EnqueueWith() error = %v", err)
	}
	if info.Queue != "tenant-a:high" {
		t.Fatalf("TaskBuilder.EnqueueWith() queue = %q, want tenant-a:high", info.Queue)
	}
}

func TestManagersEnqueueSameBaseIntoIndependentQueues(t *testing.T) {
	redisServer := miniredis.RunT(t)
	newManager := func(t *testing.T, prefix string) *Manager {
		t.Helper()
		config := managerTestConfig(redisServer.Addr())
		config.QueuePrefix = prefix
		manager, err := NewManager(context.Background(), config)
		if err != nil {
			t.Fatalf("NewManager(%q) error = %v", prefix, err)
		}
		t.Cleanup(func() { _ = manager.Stop() })
		return manager
	}
	tenantA := newManager(t, "tenant-a:")
	tenantB := newManager(t, "tenant-b:")

	infoA, err := tenantA.Enqueue(context.Background(), queue.NewTask("contract:tenant-a", nil))
	if err != nil {
		t.Fatalf("tenant A Enqueue() error = %v", err)
	}
	infoB, err := tenantB.Enqueue(context.Background(), queue.NewTask("contract:tenant-b", nil))
	if err != nil {
		t.Fatalf("tenant B Enqueue() error = %v", err)
	}
	if infoA.Queue != "tenant-a:default" || infoB.Queue != "tenant-b:default" {
		t.Fatalf("tenant queues = (%q, %q), want independently prefixed defaults", infoA.Queue, infoB.Queue)
	}

	inspector := tenantA.GetInspector()
	for _, queueName := range []string{"tenant-a:default", "tenant-b:default"} {
		info, err := inspector.GetQueueInfo(queueName)
		if err != nil {
			t.Fatalf("GetQueueInfo(%q) error = %v", queueName, err)
		}
		if info.Pending != 1 {
			t.Fatalf("GetQueueInfo(%q) pending = %d, want 1", queueName, info.Pending)
		}
	}
}

func TestIntersectingQueuePrefixRoutesBaseExactlyOnce(t *testing.T) {
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "prod"
	config.Queues = map[string]int{"production": 1}
	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	info, err := manager.Enqueue(
		context.Background(),
		queue.NewTask("contract:intersecting-prefix", nil),
		queue.Queue("production"),
	)
	if err != nil {
		t.Fatalf("Enqueue(base production) error = %v", err)
	}
	if info.Queue != "prodproduction" {
		t.Fatalf("Enqueue(base production) queue = %q, want prodproduction", info.Queue)
	}

	_, err = manager.Enqueue(
		context.Background(),
		queue.NewTask("contract:already-prefixed", nil),
		queue.Queue("prodproduction"),
	)
	if !errors.Is(err, ErrQueueNotFound) {
		t.Fatalf("Enqueue(already-prefixed) error = %v, want ErrQueueNotFound", err)
	}
}

func TestMissingDefaultQueueRejectsImplicitEnqueueAndSchedule(t *testing.T) {
	redisServer := miniredis.RunT(t)
	newManager := func(t *testing.T) *Manager {
		t.Helper()
		config := managerTestConfig(redisServer.Addr())
		config.QueuePrefix = "tenant-a:"
		config.Queues = map[string]int{"jobs": 1}
		manager, err := NewManager(context.Background(), config)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}
		t.Cleanup(func() { _ = manager.Stop() })
		return manager
	}

	t.Run("enqueue", func(t *testing.T) {
		manager := newManager(t)
		_, err := manager.Enqueue(context.Background(), queue.NewTask("contract:no-default", nil))
		if !errors.Is(err, ErrQueueNotFound) {
			t.Fatalf("implicit Enqueue() error = %v, want ErrQueueNotFound", err)
		}
	})

	t.Run("scheduler", func(t *testing.T) {
		manager := newManager(t)
		registerErr := manager.RegisterSchedule("@every 1h", queue.NewTask("contract:no-default-schedule", nil))
		if !errors.Is(registerErr, ErrQueueNotFound) {
			t.Fatalf("RegisterSchedule() error = %v, want ErrQueueNotFound", registerErr)
		}
		err := manager.Start(context.Background())
		if !errors.Is(err, ErrQueueNotFound) {
			t.Fatalf("Start() error = %v, want ErrQueueNotFound for implicit scheduler queue", err)
		}
	})
}

func TestRegisterScheduleCanonicalizesBaseQueue(t *testing.T) {
	redisServer := miniredis.RunT(t)
	config := managerTestConfig(redisServer.Addr())
	config.QueuePrefix = "tenant-a:"
	manager, err := NewManager(context.Background(), config)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RegisterSchedule(
		"@every 1h",
		queue.NewTask("contract:schedule", nil, queue.Queue("tenant-b:high")),
		queue.Queue(QueueHigh),
	); err != nil {
		t.Fatalf("RegisterSchedule() error = %v", err)
	}
	if len(manager.schedules) != 1 {
		t.Fatalf("registered schedules = %d, want 1", len(manager.schedules))
	}
	if got := effectiveQueueOption(manager.schedules[0].Opts); got != "tenant-a:high" {
		t.Fatalf("registered schedule queue = %q, want tenant-a:high", got)
	}
}

func effectiveQueueOption(opts []queue.Option) string {
	var name string
	for _, opt := range opts {
		if opt != nil && opt.Type() == queue.QueueOpt {
			value, _ := opt.Value().(string)
			name = value
		}
	}
	return name
}
