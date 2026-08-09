package asynq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

// Manager owns all Asynq and Redis resources for one queue runtime.
type Manager struct {
	config      Config
	factory     *redisconn.Factory
	client      *asynq.Client
	server      *asynq.Server
	scheduler   *asynq.Scheduler
	mux         *asynq.ServeMux
	redisOpt    *redisConnOpt
	redisClient redis.UniversalClient
	inspector   *asynq.Inspector
	handlers    map[string]asynq.HandlerFunc
	schedules   []ScheduleEntry
	scheduleErr error
	middleware  MiddlewareFunc

	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	started         bool
	closed          bool
	logger          Logger
	stopCleanupHook func() error
	stopWaitHook    func()
	stopDone        chan struct{}
	stopErr         error
}

// Config contains the complete queue runtime configuration. Redis topology,
// credentials, TLS, and pool settings have one canonical owner.
type Config struct {
	Redis       redisconn.Config
	Concurrency int
	Queues      map[string]int
	LogLevel    asynq.LogLevel
	RetryDelay  func(int) time.Duration
	QueuePrefix string
}

// ScheduleEntry describes one recurring task.
type ScheduleEntry struct {
	Cronspec string
	Task     *asynq.Task
	Opts     []asynq.Option
}

// DefaultConfig returns queue defaults around an explicit Redis deployment.
func DefaultConfig(redisConfig redisconn.Config) Config {
	return Config{
		Redis:       redisConfig,
		Concurrency: 10,
		Queues:      defaultQueuesForPrefix(""),
		LogLevel:    asynq.InfoLevel,
		RetryDelay:  defaultRetryDelay,
	}
}

func (c Config) normalize() (Config, error) {
	c.Redis = c.Redis.Normalize()
	if c.Concurrency < 0 {
		return Config{}, fmt.Errorf("%w: concurrency must not be negative", ErrInvalidConfig)
	}
	if c.Concurrency == 0 {
		c.Concurrency = 10
	}
	if len(c.Queues) == 0 {
		c.Queues = defaultQueuesForPrefix(c.QueuePrefix)
	} else {
		var err error
		c.Queues, err = prefixQueues(c.Queues, c.QueuePrefix)
		if err != nil {
			return Config{}, err
		}
	}
	if c.LogLevel < 0 || c.LogLevel > asynq.FatalLevel {
		return Config{}, fmt.Errorf("%w: unsupported log level %d", ErrInvalidConfig, c.LogLevel)
	}
	if c.LogLevel == 0 {
		c.LogLevel = asynq.InfoLevel
	}
	if c.RetryDelay == nil {
		c.RetryDelay = defaultRetryDelay
	}
	return c, nil
}

func defaultRetryDelay(n int) time.Duration {
	if n < 0 {
		n = 0
	}
	if n > 30 {
		n = 30
	}
	return time.Duration(1<<uint(n)) * time.Second
}

var (
	globalManager *Manager
	managerMu     sync.RWMutex
)

// GetManager returns the process-wide manager, if initialized.
func GetManager() *Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return globalManager
}

// GetInspector returns the global manager's Inspector, if initialized.
func GetInspector() *asynq.Inspector {
	manager := GetManager()
	if manager == nil {
		return nil
	}
	return manager.GetInspector()
}

// InitManager initializes the process-wide manager exactly once. Repeated
// initialization is explicit instead of silently accepting a different spec.
func InitManager(ctx context.Context, config Config) (*Manager, error) {
	managerMu.RLock()
	if globalManager != nil {
		managerMu.RUnlock()
		return nil, ErrManagerAlreadyInitialized
	}
	managerMu.RUnlock()

	manager, err := NewManager(ctx, config)
	if err != nil {
		return nil, err
	}

	managerMu.Lock()
	defer managerMu.Unlock()
	if globalManager != nil {
		return nil, errors.Join(ErrManagerAlreadyInitialized, manager.Stop())
	}
	globalManager = manager
	return manager, nil
}

// ResetManagerForTesting clears and closes the process-wide manager.
func ResetManagerForTesting() {
	managerMu.Lock()
	manager := globalManager
	globalManager = nil
	managerMu.Unlock()
	if manager != nil {
		if err := manager.Stop(); err != nil {
			GetLogger().Error(fmt.Sprintf("[Asynq] reset manager: %v", err))
		}
	}
}

// NewManager validates the canonical Redis spec and verifies connectivity and
// authentication with PING before exposing any queue component.
func NewManager(ctx context.Context, config Config) (*Manager, error) {
	var err error
	config, err = config.normalize()
	if err != nil {
		return nil, err
	}
	factory, err := redisconn.NewFactory(config.Redis)
	if err != nil {
		return nil, fmt.Errorf("asynq: configure Redis: %w", err)
	}
	redisClient, err := factory.Open(ctx)
	if err != nil {
		return nil, fmt.Errorf("asynq: verify Redis: %w", err)
	}

	redisOpt := newRedisConnOpt(factory)
	manager := &Manager{
		config:      config,
		factory:     factory,
		client:      asynq.NewClient(redisOpt),
		mux:         asynq.NewServeMux(),
		redisOpt:    redisOpt,
		redisClient: redisClient,
		handlers:    make(map[string]asynq.HandlerFunc),
		schedules:   make([]ScheduleEntry, 0),
		logger:      GetLogger(),
	}
	manager.logger.Log(fmt.Sprintf(
		"[Asynq] Redis connection verified, mode=%s, endpoints=%d",
		config.Redis.Mode,
		len(config.Redis.Addrs),
	))
	return manager, nil
}

// SetLogger replaces this manager's logger.
func (m *Manager) SetLogger(logger Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if logger == nil {
		logger = GetLogger()
	}
	m.logger = logger
}

// RegisterHandler 注册任务处理器。
func (m *Manager) RegisterHandler(taskType string, handler asynq.HandlerFunc) error {
	if strings.TrimSpace(taskType) == "" || handler == nil {
		return fmt.Errorf("%w: task type and handler are required", ErrInvalidHandler)
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrManagerStopped
	}
	if m.started {
		m.mu.Unlock()
		return ErrManagerStarted
	}
	if _, exists := m.handlers[taskType]; exists {
		m.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrHandlerAlreadyRegistered, taskType)
	}
	m.handlers[taskType] = handler
	m.mux.HandleFunc(taskType, handler)
	logger := m.logger
	m.mu.Unlock()
	logger.Log(fmt.Sprintf("[Asynq] registered handler: %s", taskType))
	return nil
}

// RegisterSchedule registers a recurring task to be installed by Start. Queue
// options are base names and are resolved inside this Manager's namespace.
func (m *Manager) RegisterSchedule(cronspec string, task *asynq.Task, opts ...asynq.Option) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagerStopped
	}
	if m.started {
		return ErrManagerStarted
	}
	normalizedOpts, err := m.canonicalizeQueueOptions(opts)
	if err != nil {
		err = fmt.Errorf("asynq: register schedule %q: %w", cronspec, err)
		m.scheduleErr = errors.Join(m.scheduleErr, err)
		return err
	}
	m.schedules = append(m.schedules, ScheduleEntry{
		Cronspec: cronspec,
		Task:     task,
		Opts:     normalizedOpts,
	})
	if task != nil {
		m.logger.Log(fmt.Sprintf("[Asynq] registered schedule: %s -> %s", cronspec, task.Type()))
	}
	return nil
}

// Start starts the worker and scheduler synchronously. Startup errors are
// returned to the caller and any component resources created by this attempt
// are rolled back.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx == nil {
		return fmt.Errorf("%w: start requires a non-nil context", ErrInvalidContext)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("asynq: start: %w", err)
	}
	if m.closed {
		return ErrManagerStopped
	}
	if m.started {
		return nil
	}
	if m.scheduleErr != nil {
		return m.scheduleErr
	}

	resourceMark := m.redisOpt.mark()
	var scheduler *asynq.Scheduler
	if len(m.schedules) > 0 {
		scheduler = asynq.NewScheduler(m.redisOpt, &asynq.SchedulerOpts{
			LogLevel: m.config.LogLevel,
		})
		for _, entry := range m.schedules {
			if entry.Task == nil {
				return errors.Join(
					fmt.Errorf("asynq: register schedule %q: nil task", entry.Cronspec),
					m.redisOpt.closeFrom(resourceMark),
				)
			}
			entryID, err := scheduler.Register(entry.Cronspec, entry.Task, entry.Opts...)
			if err != nil {
				return errors.Join(
					fmt.Errorf("asynq: register schedule %q: %w", entry.Cronspec, err),
					m.redisOpt.closeFrom(resourceMark),
				)
			}
			m.logger.Log(fmt.Sprintf(
				"[Asynq] schedule registered: %s (entry_id=%s)",
				entry.Task.Type(),
				entryID,
			))
		}
	}

	server := asynq.NewServer(
		m.redisOpt,
		asynq.Config{
			Concurrency: m.config.Concurrency,
			Queues:      m.config.Queues,
			LogLevel:    m.config.LogLevel,
			RetryDelayFunc: func(n int, _ error, _ *asynq.Task) time.Duration {
				return m.config.RetryDelay(n)
			},
		},
	)
	if err := server.Start(m.mux); err != nil {
		return errors.Join(
			fmt.Errorf("asynq: start server: %w", err),
			m.redisOpt.closeFrom(resourceMark),
		)
	}
	if scheduler != nil {
		if err := scheduler.Start(); err != nil {
			server.Shutdown()
			return errors.Join(
				fmt.Errorf("asynq: start scheduler: %w", err),
				m.redisOpt.closeFrom(resourceMark),
			)
		}
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.server = server
	m.scheduler = scheduler
	m.started = true
	m.logger.Log(fmt.Sprintf(
		"[Asynq] started, concurrency=%d, handlers=%d, schedules=%d",
		m.config.Concurrency,
		len(m.handlers),
		len(m.schedules),
	))
	return nil
}

// Stop shuts down every resource owned by this manager. It is idempotent and
// also closes clients when Start was never called.
func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.closed {
		done := m.stopDone
		stopWaitHook := m.stopWaitHook
		m.mu.Unlock()
		if done != nil {
			if stopWaitHook != nil {
				stopWaitHook()
			}
			<-done
		}
		m.mu.RLock()
		stopErr := m.stopErr
		m.mu.RUnlock()
		return stopErr
	}
	m.closed = true
	m.started = false
	done := make(chan struct{})
	m.stopDone = done
	cancel := m.cancel
	scheduler := m.scheduler
	server := m.server
	redisOpt := m.redisOpt
	redisClient := m.redisClient
	logger := m.logger
	stopCleanupHook := m.stopCleanupHook
	m.inspector = nil
	m.mu.Unlock()

	logger.Log("[Asynq] stopping...")
	if cancel != nil {
		cancel()
	}
	if scheduler != nil {
		scheduler.Shutdown()
	}
	if server != nil {
		server.Shutdown()
	}

	var stopErr error
	if stopCleanupHook != nil {
		stopErr = errors.Join(stopErr, stopCleanupHook())
	}
	if redisOpt != nil {
		stopErr = errors.Join(stopErr, redisOpt.closeAll())
	}
	if redisClient != nil {
		if err := redisClient.Close(); err != nil && !errors.Is(err, redis.ErrClosed) {
			stopErr = errors.Join(stopErr, err)
		}
	}
	m.mu.Lock()
	m.stopErr = stopErr
	close(done)
	m.mu.Unlock()
	logger.Log("[Asynq] stopped")
	return stopErr
}

// Enqueue enqueues an Asynq task.
func (m *Manager) Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: enqueue requires a non-nil context", ErrInvalidContext)
	}
	if task == nil {
		return nil, ErrInvalidTask
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, ErrManagerStopped
	}
	if m.client == nil {
		return nil, ErrManagerNotInitialized
	}
	normalizedOpts, err := m.canonicalizeQueueOptions(opts)
	if err != nil {
		return nil, err
	}
	return m.client.EnqueueContext(ctx, task, normalizedOpts...)
}

// EnqueueTask constructs and enqueues an Asynq task.
func (m *Manager) EnqueueTask(ctx context.Context, taskType string, payload []byte, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	task := asynq.NewTask(taskType, payload)
	return m.Enqueue(ctx, task, opts...)
}

// GetClient returns the native Asynq client.
func (m *Manager) GetClient() *asynq.Client {
	return m.client
}

// GetServer returns the worker server, once started.
func (m *Manager) GetServer() *asynq.Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server
}

// GetScheduler returns the scheduler, once started with registered schedules.
func (m *Manager) GetScheduler() *asynq.Scheduler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.scheduler
}

// GetInspector lazily creates and reuses an Inspector.
func (m *Manager) GetInspector() *asynq.Inspector {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.redisOpt == nil {
		return nil
	}
	if m.inspector == nil {
		m.inspector = asynq.NewInspector(m.redisOpt)
	}
	return m.inspector
}

// withInspector keeps Stop from closing the Inspector's Redis connection
// while an operation is in flight. Operations linearize either before Stop or
// return ErrManagerStopped after shutdown begins.
func (m *Manager) withInspector(operation func(*asynq.Inspector) error) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrManagerStopped
	}
	if m.redisOpt == nil {
		m.mu.Unlock()
		return ErrManagerNotInitialized
	}
	if m.inspector == nil {
		m.inspector = asynq.NewInspector(m.redisOpt)
	}
	inspector := m.inspector
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.inspector != inspector {
		return ErrManagerStopped
	}
	return operation(inspector)
}

func (m *Manager) pingRedis(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: Redis health check requires a non-nil context", ErrInvalidContext)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrManagerStopped
	}
	client := m.redisClient
	m.mu.RUnlock()
	if client == nil {
		return ErrRedisClientUnavailable
	}

	err := client.Ping(ctx).Err()
	if err == nil {
		return nil
	}
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return ErrManagerStopped
	}
	return err
}

// IsStarted reports whether all configured queue components started.
func (m *Manager) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// GetRedisOpt exposes the canonical Asynq connection option for integrations
// such as Asynqmon.
func (m *Manager) GetRedisOpt() asynq.RedisConnOpt {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.redisOpt
}
