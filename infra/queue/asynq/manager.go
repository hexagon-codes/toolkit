package asynq

import (
	"context"
	"fmt"
	"github.com/hibiken/asynq"
	"sync"
	"time"
)

// =========================================
// Asynq 通用管理器
// 统一管理所有异步任务的入队、消费、调度
// =========================================
// Manager Asynq 全局管理器
type Manager struct {
	config     *Config
	client     *asynq.Client
	server     *asynq.Server
	scheduler  *asynq.Scheduler
	mux        *asynq.ServeMux
	redisOpt   asynq.RedisConnOpt // 改为接口类型，兼容单点和集群
	inspector  *asynq.Inspector   // 复用 Inspector 实例
	handlers   map[string]asynq.HandlerFunc
	schedules  []ScheduleEntry
	middleware MiddlewareFunc // 中间件
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	started    bool
	logger     Logger
}

// Config Asynq 配置（支持代理/单点模式和集群直连模式）
type Config struct {
	RedisAddrs  []string                // Redis 地址列表（1个=代理模式，多个=集群直连）
	Password    string                  // Redis 密码
	Username    string                  // Redis 6.0+ ACL 用户名
	Concurrency int                     // Worker 并发数
	Queues      map[string]int          // 队列优先级配置
	LogLevel    asynq.LogLevel          // 日志级别
	RetryDelay  func(int) time.Duration // 重试延迟函数
}

// ScheduleEntry 定时任务条目
type ScheduleEntry struct {
	Cronspec string         // cron 表达式，如 "@every 1m", "0 * * * *"
	Task     *asynq.Task    // 任务
	Opts     []asynq.Option // 任务选项
}

// Logger 接口现在定义在 interfaces.go 中
// DefaultConfig 默认配置（集群模式）
func DefaultConfig() *Config {
	return &Config{
		RedisAddrs:  []string{}, // 需要外部配置
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		LogLevel: asynq.InfoLevel,
		RetryDelay: func(n int) time.Duration {
			// 指数退避：1s, 2s, 4s, 8s...
			return time.Duration(1<<uint(n)) * time.Second
		},
	}
}

// 全局单例
var (
	globalManager *Manager
	managerMu     sync.RWMutex
)

// GetManager 获取全局管理器
func GetManager() *Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return globalManager
}

// GetInspector 获取 Inspector 实例（用于查询队列状态）
func GetInspector() *asynq.Inspector {
	manager := GetManager()
	if manager == nil {
		return nil
	}
	return manager.GetInspector()
}

// InitManager 初始化全局管理器
// 支持初始化失败后重试，已初始化则直接返回
func InitManager(config *Config) (*Manager, error) {
	managerMu.Lock()
	defer managerMu.Unlock()
	// 已初始化则直接返回
	if globalManager != nil {
		return globalManager, nil
	}
	m, err := NewManager(config)
	if err != nil {
		return nil, err // 允许重试
	}
	globalManager = m
	return m, nil
}

// InitWithRedisConfig 使用 Redis 配置初始化
// 自动根据地址数量选择模式：1个=代理模式，多个=集群直连
// 返回 error 而不是 panic，允许调用方决定如何处理
func InitWithRedisConfig(addrs []string, password, username string) error {
	configProvider := GetConfigProvider()
	config := &Config{
		RedisAddrs:  addrs,
		Password:    password,
		Username:    username, // ✅ 支持 Redis 6.0+ ACL
		Concurrency: configProvider.GetConcurrency(),
		Queues:      DefaultQueues(),
	}
	// 日志级别固定为 info（生产环境推荐）
	config.LogLevel = asynq.InfoLevel
	_, err := InitManager(config)
	if err != nil {
		return fmt.Errorf("asynq init failed: %w", err)
	}
	mode := "proxy/single"
	if len(addrs) > 1 {
		mode = "cluster direct"
	}
	GetLogger().Log(fmt.Sprintf("[Asynq] 初始化成功，模式: %s, 地址: %v, 并发数: %d", mode, addrs, configProvider.GetConcurrency()))
	return nil
}

// ResetManagerForTesting 重置单例（仅用于测试）
// 生产环境不应调用此方法
func ResetManagerForTesting() {
	managerMu.Lock()
	defer managerMu.Unlock()
	if globalManager != nil {
		globalManager.Stop()
		globalManager = nil
	}
}

// NewManager 创建新的管理器
// 支持两种模式：
// - 1 个地址：代理/单点模式（阿里云等云厂商的代理集群）
// - 多个地址：集群直连模式
func NewManager(config *Config) (*Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if len(config.RedisAddrs) == 0 {
		return nil, fmt.Errorf("redis addrs not configured")
	}
	// 根据地址数量选择 Redis 连接方式
	var redisOpt asynq.RedisConnOpt
	if len(config.RedisAddrs) == 1 {
		// 代理/单点模式
		redisOpt = asynq.RedisClientOpt{
			Addr:     config.RedisAddrs[0],
			Password: config.Password,
			Username: config.Username,
		}
		GetLogger().Log(fmt.Sprintf("[Asynq] Using Redis proxy/single mode, addr: %s", config.RedisAddrs[0]))
	} else {
		// 集群直连模式
		redisOpt = asynq.RedisClusterClientOpt{
			Addrs:    config.RedisAddrs,
			Password: config.Password,
			Username: config.Username,
		}
		GetLogger().Log(fmt.Sprintf("[Asynq] Using Redis cluster direct mode, nodes: %v", config.RedisAddrs))
	}
	return &Manager{
		config:    config,
		client:    asynq.NewClient(redisOpt),
		mux:       asynq.NewServeMux(),
		redisOpt:  redisOpt,
		handlers:  make(map[string]asynq.HandlerFunc),
		schedules: make([]ScheduleEntry, 0),
		logger:    GetLogger(),
	}, nil
}

// SetLogger 设置日志器
func (m *Manager) SetLogger(logger Logger) {
	m.logger = logger
}

// RegisterHandler 注册任务处理器
func (m *Manager) RegisterHandler(taskType string, handler asynq.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[taskType] = handler
	m.mux.HandleFunc(taskType, handler)
	m.logger.Log(fmt.Sprintf("[Asynq] registered handler: %s", taskType))
}

// RegisterSchedule 注册定时任务
func (m *Manager) RegisterSchedule(cronspec string, task *asynq.Task, opts ...asynq.Option) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedules = append(m.schedules, ScheduleEntry{
		Cronspec: cronspec,
		Task:     task,
		Opts:     opts,
	})
	m.logger.Log(fmt.Sprintf("[Asynq] registered schedule: %s -> %s", cronspec, task.Type()))
}

// Start 启动服务（Worker + Scheduler）
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	// 创建 Server
	m.server = asynq.NewServer(
		m.redisOpt,
		asynq.Config{
			Concurrency: m.config.Concurrency,
			Queues:      m.config.Queues,
			LogLevel:    m.config.LogLevel,
			RetryDelayFunc: func(n int, e error, t *asynq.Task) time.Duration {
				if m.config.RetryDelay != nil {
					return m.config.RetryDelay(n)
				}
				return time.Duration(1<<uint(n)) * time.Second
			},
		},
	)
	// 启动 Server
	go func() {
		if err := m.server.Run(m.mux); err != nil {
			m.logger.Error(fmt.Sprintf("[Asynq] server error: %v", err))
		}
	}()
	// 如果有定时任务，启动 Scheduler
	if len(m.schedules) > 0 {
		m.scheduler = asynq.NewScheduler(m.redisOpt, &asynq.SchedulerOpts{
			LogLevel: m.config.LogLevel,
		})
		for _, entry := range m.schedules {
			entryID, err := m.scheduler.Register(entry.Cronspec, entry.Task, entry.Opts...)
			if err != nil {
				m.logger.Error(fmt.Sprintf("[Asynq] register schedule failed: %v", err))
				continue
			}
			m.logger.Log(fmt.Sprintf("[Asynq] schedule registered: %s (entry_id=%s)", entry.Task.Type(), entryID))
		}
		go func() {
			if err := m.scheduler.Run(); err != nil {
				m.logger.Error(fmt.Sprintf("[Asynq] scheduler error: %v", err))
			}
		}()
	}
	m.logger.Log(fmt.Sprintf("[Asynq] started, concurrency=%d, handlers=%d, schedules=%d",
		m.config.Concurrency, len(m.handlers), len(m.schedules)))
	return nil
}

// Stop 停止服务
// 可以安全地重复调用，只有第一次调用会执行停止操作
func (m *Manager) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil // 未启动或已停止，直接返回
	}
	m.started = false
	m.mu.Unlock()

	m.logger.Log("[Asynq] stopping...")
	if m.cancel != nil {
		m.cancel()
	}
	if m.scheduler != nil {
		m.scheduler.Shutdown()
	}
	if m.server != nil {
		m.server.Shutdown()
	}
	if m.client != nil {
		m.client.Close()
	}
	// 关闭 Inspector
	m.mu.Lock()
	if m.inspector != nil {
		m.inspector.Close()
		m.inspector = nil
	}
	m.mu.Unlock()
	m.logger.Log("[Asynq] stopped")
	return nil
}

// Enqueue 入队任务
func (m *Manager) Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return m.client.EnqueueContext(ctx, task, opts...)
}

// EnqueueTask 入队任务（简化版）
func (m *Manager) EnqueueTask(ctx context.Context, taskType string, payload []byte, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	task := asynq.NewTask(taskType, payload, opts...)
	return m.client.EnqueueContext(ctx, task, opts...)
}

// GetClient 获取原生 Client
func (m *Manager) GetClient() *asynq.Client {
	return m.client
}

// GetServer 获取原生 Server
func (m *Manager) GetServer() *asynq.Server {
	return m.server
}

// GetScheduler 获取原生 Scheduler
func (m *Manager) GetScheduler() *asynq.Scheduler {
	return m.scheduler
}

// GetInspector 获取 Inspector（复用实例）
func (m *Manager) GetInspector() *asynq.Inspector {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inspector == nil {
		m.inspector = asynq.NewInspector(m.redisOpt)
	}
	return m.inspector
}

// IsStarted 是否已启动
func (m *Manager) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// GetRedisOpt 获取 Redis 配置（用于 Asynqmon，兼容单点和集群）
func (m *Manager) GetRedisOpt() asynq.RedisConnOpt {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.redisOpt
}

// 默认日志器现在定义在 interfaces.go 中
