package asynq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	hibasynq "github.com/hibiken/asynq"
)

// =========================================
// 背压控制器
// 防止队列过载，实现自动限流
// =========================================
// BackpressureConfig 背压配置
type BackpressureConfig struct {
	// MaxQueueSize 队列最大长度阈值
	MaxQueueSize int
	// WarningThreshold 警告阈值（百分比）
	WarningThreshold float64
	// CriticalThreshold 危急阈值（百分比）
	CriticalThreshold float64
	// CheckInterval 检查间隔
	CheckInterval time.Duration
	// OnWarning 警告回调
	OnWarning func(queue string, size int, threshold int)
	// OnCritical 危急回调
	OnCritical func(queue string, size int, threshold int)
	// OnRecover 恢复回调
	OnRecover func(queue string, size int)
}

// DefaultBackpressureConfig 默认配置
func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		MaxQueueSize:      10000,
		WarningThreshold:  0.7, // 70%
		CriticalThreshold: 0.9, // 90%
		CheckInterval:     30 * time.Second,
	}
}

// BackpressureState 背压状态
type BackpressureState int

const (
	// StateNormal 正常状态
	StateNormal BackpressureState = iota
	// StateWarning 警告状态
	StateWarning
	// StateCritical 危急状态
	StateCritical
)

func (s BackpressureState) String() string {
	switch s {
	case StateNormal:
		return "NORMAL"
	case StateWarning:
		return "WARNING"
	case StateCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// QueueBackpressure 单个队列的背压信息
type QueueBackpressure struct {
	Queue         string            `json:"queue"`
	State         BackpressureState `json:"-"`
	StateStr      string            `json:"state"`
	CurrentSize   int               `json:"current_size"`
	MaxSize       int               `json:"max_size"`
	Utilization   float64           `json:"utilization"`
	RejectCount   int64             `json:"reject_count"`
	LastCheckTime time.Time         `json:"last_check_time"`
}

// BackpressureController 背压控制器
type BackpressureController struct {
	mu            sync.RWMutex
	config        BackpressureConfig
	manager       *Manager
	states        map[string]*QueueBackpressure
	rejectCounts  map[string]int64
	cancel        context.CancelFunc
	done          chan struct{}
	running       bool
	stopping      bool
	newTicker     func(time.Duration) backpressureTicker
	inspectQueues func(context.Context, *Manager, *hibasynq.Inspector) error
}

type backpressureTicker interface {
	Chan() <-chan time.Time
	Stop()
}

type systemBackpressureTicker struct {
	ticker *time.Ticker
}

func (t *systemBackpressureTicker) Chan() <-chan time.Time { return t.ticker.C }
func (t *systemBackpressureTicker) Stop()                  { t.ticker.Stop() }

func newSystemBackpressureTicker(interval time.Duration) backpressureTicker {
	return &systemBackpressureTicker{ticker: time.NewTicker(interval)}
}

var (
	backpressureController     *BackpressureController
	backpressureControllerOnce sync.Once
)

// GetBackpressureController 获取背压控制器
func GetBackpressureController() *BackpressureController {
	backpressureControllerOnce.Do(func() {
		backpressureController = &BackpressureController{
			config:       DefaultBackpressureConfig(),
			states:       make(map[string]*QueueBackpressure),
			rejectCounts: make(map[string]int64),
			newTicker:    newSystemBackpressureTicker,
		}
	})
	return backpressureController
}

// SetConfig 设置配置
func (bc *BackpressureController) SetConfig(config BackpressureConfig) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if config.CheckInterval <= 0 {
		return fmt.Errorf("%w: backpressure check interval must be positive", ErrInvalidConfig)
	}
	if bc.running || bc.stopping {
		return ErrBackpressureRunning
	}
	bc.config = config
	return nil
}

// SetManager 设置 Manager
func (bc *BackpressureController) SetManager(m *Manager) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.manager = m
}

// Start 启动背压监控
func (bc *BackpressureController) Start() error {
	bc.mu.Lock()
	if bc.running {
		if bc.stopping {
			bc.mu.Unlock()
			return ErrBackpressureRunning
		}
		bc.mu.Unlock()
		return nil
	}
	if bc.config.CheckInterval <= 0 {
		bc.mu.Unlock()
		return fmt.Errorf("%w: backpressure check interval must be positive", ErrInvalidConfig)
	}
	interval := bc.config.CheckInterval
	newTicker := bc.newTicker
	if newTicker == nil {
		newTicker = newSystemBackpressureTicker
	}
	ticker := newTicker(interval)
	bc.running = true
	bc.stopping = false
	ctx, cancel := context.WithCancel(context.Background())
	bc.cancel = cancel
	done := make(chan struct{})
	bc.done = done
	bc.mu.Unlock()
	go func() {
		defer close(done)
		bc.monitorLoop(ctx, ticker)
	}()
	GetLogger().Log("[Backpressure] Controller started")
	return nil
}

// Stop 停止背压监控
func (bc *BackpressureController) Stop() {
	bc.mu.Lock()
	if !bc.running {
		bc.mu.Unlock()
		return
	}
	done := bc.done
	cancel := bc.cancel
	initiated := !bc.stopping
	if initiated {
		bc.stopping = true
	}
	bc.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	bc.mu.Lock()
	if bc.done == done {
		bc.cancel = nil
		bc.done = nil
		bc.running = false
		bc.stopping = false
	}
	bc.mu.Unlock()
	if initiated {
		GetLogger().Log("[Backpressure] Controller stopped")
	}
}

// monitorLoop 监控循环
func (bc *BackpressureController) monitorLoop(ctx context.Context, ticker backpressureTicker) {
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.Chan():
			bc.checkAllQueues(ctx)
		}
	}
}

// checkAllQueues 检查所有队列
func (bc *BackpressureController) checkAllQueues(ctx context.Context) {
	bc.mu.RLock()
	manager := bc.manager
	inspectQueues := bc.inspectQueues
	bc.mu.RUnlock()
	if manager == nil {
		return
	}
	err := manager.withInspector(func(inspector *hibasynq.Inspector) error {
		if inspectQueues != nil {
			return inspectQueues(ctx, manager, inspector)
		}
		for _, queue := range manager.QueueNames() {
			bc.checkQueue(queue, inspector)
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrManagerStopped) && !errors.Is(err, context.Canceled) {
		GetLogger().Error(fmt.Sprintf("[Backpressure] inspect queues: %v", err))
	}
}

// checkQueue 检查单个队列
func (bc *BackpressureController) checkQueue(queue string, inspector *hibasynq.Inspector) {
	queueInfo, err := inspector.GetQueueInfo(queue)
	if err != nil {
		return
	}
	currentSize := queueInfo.Pending + queueInfo.Active + queueInfo.Scheduled + queueInfo.Retry
	bc.mu.Lock()
	// 获取或创建队列背压信息
	bp, ok := bc.states[queue]
	if !ok {
		bp = &QueueBackpressure{
			Queue:   queue,
			State:   StateNormal,
			MaxSize: bc.config.MaxQueueSize,
		}
		bc.states[queue] = bp
	}
	oldState := bp.State
	bp.CurrentSize = currentSize
	bp.Utilization = float64(currentSize) / float64(bc.config.MaxQueueSize)
	bp.LastCheckTime = time.Now()
	bp.RejectCount = bc.rejectCounts[queue]
	// 判断新状态
	var newState BackpressureState
	if bp.Utilization >= bc.config.CriticalThreshold {
		newState = StateCritical
	} else if bp.Utilization >= bc.config.WarningThreshold {
		newState = StateWarning
	} else {
		newState = StateNormal
	}
	bp.State = newState
	bp.StateStr = newState.String()
	config := bc.config
	stateChanged := newState != oldState
	bc.mu.Unlock()
	// 状态变化时触发回调
	if stateChanged {
		handleBackpressureStateChange(config, queue, oldState, newState, currentSize)
	}
}

// handleBackpressureStateChange 处理状态变化
func handleBackpressureStateChange(
	config BackpressureConfig,
	queue string,
	oldState, newState BackpressureState,
	size int,
) {
	threshold := int(float64(config.MaxQueueSize) * config.WarningThreshold)
	switch newState {
	case StateWarning:
		GetLogger().Log(fmt.Sprintf("[Backpressure] Queue %s entering WARNING state: size=%d, threshold=%d",
			queue, size, threshold))
		if config.OnWarning != nil {
			// User callbacks are intentionally detached from controller shutdown.
			go config.OnWarning(queue, size, threshold)
		}
	case StateCritical:
		threshold = int(float64(config.MaxQueueSize) * config.CriticalThreshold)
		GetLogger().Log(fmt.Sprintf("[Backpressure] Queue %s entering CRITICAL state: size=%d, threshold=%d",
			queue, size, threshold))
		if config.OnCritical != nil {
			// User callbacks are intentionally detached from controller shutdown.
			go config.OnCritical(queue, size, threshold)
		}
	case StateNormal:
		if oldState != StateNormal {
			GetLogger().Log(fmt.Sprintf("[Backpressure] Queue %s recovered to NORMAL state: size=%d",
				queue, size))
			if config.OnRecover != nil {
				// User callbacks are intentionally detached from controller shutdown.
				go config.OnRecover(queue, size)
			}
		}
	}
}

// AllowEnqueue 检查是否允许入队
func (bc *BackpressureController) AllowEnqueue(queue string) error {
	bc.mu.RLock()
	bp, ok := bc.states[queue]
	if !ok {
		bc.mu.RUnlock()
		// 未监控的队列，允许入队
		return nil
	}
	// 在持有 RLock 时读取 state，避免 TOCTOU 竞态
	state := bp.State
	bc.mu.RUnlock()

	if state == StateCritical {
		bc.mu.Lock()
		bc.rejectCounts[queue]++
		bc.mu.Unlock()
		return fmt.Errorf("queue %s is in critical state, rejecting new tasks", queue)
	}
	return nil
}

// GetQueueState 获取队列状态
func (bc *BackpressureController) GetQueueState(queue string) *QueueBackpressure {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if bp, ok := bc.states[queue]; ok {
		// 返回副本
		copy := *bp
		return &copy
	}
	return nil
}

// GetAllStates 获取所有队列状态
func (bc *BackpressureController) GetAllStates() []QueueBackpressure {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	states := make([]QueueBackpressure, 0, len(bc.states))
	for _, bp := range bc.states {
		states = append(states, *bp)
	}
	return states
}

// =========================================
// 限流器
// =========================================
// RateLimiter 限流器
type RateLimiter struct {
	mu          sync.Mutex
	rate        int       // 每秒允许的请求数
	burst       int       // 突发容量
	tokens      int       // 当前令牌数
	lastRefresh time.Time // 上次刷新时间
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate, burst int) *RateLimiter {
	return &RateLimiter{
		rate:        rate,
		burst:       burst,
		tokens:      burst,
		lastRefresh: time.Now(),
	}
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(rl.lastRefresh).Seconds()
	// 补充令牌
	newTokens := int(elapsed * float64(rl.rate))
	if newTokens > 0 {
		rl.tokens = min(rl.burst, rl.tokens+newTokens)
		rl.lastRefresh = now
	}
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

// Wait 等待直到允许请求
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1000/rl.rate) * time.Millisecond):
			// 继续尝试
		}
	}
}

// =========================================
// 全局限流管理
// =========================================
// GlobalRateLimitManager 全局限流管理器
type GlobalRateLimitManager struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
}

var (
	globalRateLimitManager     *GlobalRateLimitManager
	globalRateLimitManagerOnce sync.Once
)

// GetGlobalRateLimitManager 获取全局限流管理器
func GetGlobalRateLimitManager() *GlobalRateLimitManager {
	globalRateLimitManagerOnce.Do(func() {
		globalRateLimitManager = &GlobalRateLimitManager{
			limiters: make(map[string]*RateLimiter),
		}
	})
	return globalRateLimitManager
}

// GetLimiter 获取或创建限流器
func (m *GlobalRateLimitManager) GetLimiter(key string, rate, burst int) *RateLimiter {
	m.mu.RLock()
	if limiter, ok := m.limiters[key]; ok {
		m.mu.RUnlock()
		return limiter
	}
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查
	if limiter, ok := m.limiters[key]; ok {
		return limiter
	}
	limiter := NewRateLimiter(rate, burst)
	m.limiters[key] = limiter
	return limiter
}

// AllowPlatform 检查平台是否允许请求
func (m *GlobalRateLimitManager) AllowPlatform(platform string) bool {
	// 不同平台有不同的限流配置
	var rate, burst int
	switch platform {
	case "sora", "sora_ch1", "sora_ch2":
		rate, burst = 10, 20 // Sora API 限流
	case "veo3":
		rate, burst = 5, 10 // Veo3 API 限流
	case "replicate_video", "replicate_image":
		rate, burst = 20, 50 // Replicate API 限流
	default:
		rate, burst = 50, 100 // 默认限流
	}
	limiter := m.GetLimiter(fmt.Sprintf("platform:%s", platform), rate, burst)
	return limiter.Allow()
}

// AllowChannel 检查渠道是否允许请求
func (m *GlobalRateLimitManager) AllowChannel(channelID int) bool {
	// 每个渠道的默认限流
	limiter := m.GetLimiter(fmt.Sprintf("channel:%d", channelID), 100, 200)
	return limiter.Allow()
}
