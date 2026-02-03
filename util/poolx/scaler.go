package poolx

import (
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Auto-Scaler Implementation
// ============================================================================

// ScalerConfig holds the configuration for auto-scaling
type ScalerConfig struct {
	// ScaleInterval is how often to check for scaling needs
	ScaleInterval time.Duration
	// ScaleUpThreshold triggers scale up when load exceeds this (0.0-1.0)
	ScaleUpThreshold float64
	// ScaleDownThreshold triggers scale down when load is below this (0.0-1.0)
	ScaleDownThreshold float64
	// ScaleUpStep is the number of workers to add when scaling up
	ScaleUpStep int32
	// ScaleDownStep is the number of workers to remove when scaling down
	ScaleDownStep int32
	// MinWorkers is the minimum number of workers to maintain
	MinWorkers int32
	// MaxWorkers is the maximum number of workers allowed
	MaxWorkers int32
	// CooldownPeriod prevents rapid scaling oscillations
	CooldownPeriod time.Duration
	// EMAAlpha is the smoothing factor for EMA calculation (0.0-1.0)
	// Higher values make the average more responsive to recent changes
	EMAAlpha float64
}

// DefaultScalerConfig returns sensible defaults for auto-scaling
func DefaultScalerConfig() ScalerConfig {
	return ScalerConfig{
		ScaleInterval:      time.Second,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		ScaleUpStep:        2,
		ScaleDownStep:      1,
		MinWorkers:         1,
		MaxWorkers:         100,
		CooldownPeriod:     5 * time.Second,
		EMAAlpha:           0.3,
	}
}

// AutoScaler manages automatic scaling of workers based on load
type AutoScaler struct {
	config ScalerConfig
	pool   *Pool

	// State
	running     atomic.Bool
	stopCh      chan struct{}
	mu          sync.Mutex
	lastScale   time.Time
	emaLoad     float64 // Exponential moving average of load
	initialized bool

	// Metrics
	scaleUpCount   atomic.Int64
	scaleDownCount atomic.Int64
}

// NewAutoScaler creates a new auto-scaler for the given pool
func NewAutoScaler(pool *Pool, config ScalerConfig) *AutoScaler {
	// Validate config
	if config.ScaleInterval <= 0 {
		config.ScaleInterval = time.Second
	}
	if config.ScaleUpThreshold <= 0 || config.ScaleUpThreshold > 1 {
		config.ScaleUpThreshold = 0.8
	}
	if config.ScaleDownThreshold < 0 || config.ScaleDownThreshold >= config.ScaleUpThreshold {
		config.ScaleDownThreshold = 0.2
	}
	if config.ScaleUpStep <= 0 {
		config.ScaleUpStep = 2
	}
	if config.ScaleDownStep <= 0 {
		config.ScaleDownStep = 1
	}
	if config.EMAAlpha <= 0 || config.EMAAlpha > 1 {
		config.EMAAlpha = 0.3
	}

	return &AutoScaler{
		config: config,
		pool:   pool,
		stopCh: make(chan struct{}),
	}
}

// Start begins the auto-scaling loop
func (s *AutoScaler) Start() {
	if s.running.Swap(true) {
		return // Already running
	}

	// 重新创建 stopCh 以支持重启
	s.mu.Lock()
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.scalingLoop()
}

// Stop stops the auto-scaling loop
func (s *AutoScaler) Stop() {
	if !s.running.Swap(false) {
		return // Not running
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Only close if not already closed
	select {
	case <-s.stopCh:
		// Already closed
	default:
		close(s.stopCh)
	}
}

// IsRunning returns true if the scaler is active
func (s *AutoScaler) IsRunning() bool {
	return s.running.Load()
}

// scalingLoop is the main auto-scaling goroutine
func (s *AutoScaler) scalingLoop() {
	ticker := time.NewTicker(s.config.ScaleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndScale()
		case <-s.stopCh:
			return
		}
	}
}

// checkAndScale evaluates the current load and scales if necessary
func (s *AutoScaler) checkAndScale() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check cooldown period
	if time.Since(s.lastScale) < s.config.CooldownPeriod {
		return
	}

	// Calculate current load
	currentLoad := s.calculateLoad()

	// Update EMA
	if !s.initialized {
		s.emaLoad = currentLoad
		s.initialized = true
	} else {
		s.emaLoad = s.config.EMAAlpha*currentLoad + (1-s.config.EMAAlpha)*s.emaLoad
	}

	// Determine scaling action
	currentWorkers := s.pool.workerCount.Load()

	if s.emaLoad > s.config.ScaleUpThreshold && currentWorkers < s.config.MaxWorkers {
		s.scaleUp(currentWorkers)
	} else if s.emaLoad < s.config.ScaleDownThreshold && currentWorkers > s.config.MinWorkers {
		s.scaleDown(currentWorkers)
	}
}

// calculateLoad returns the current load factor (0.0-1.0)
func (s *AutoScaler) calculateLoad() float64 {
	running := s.pool.workerCount.Load()
	if running == 0 {
		return 0
	}

	// Load = (active workers + queued tasks) / max workers
	// This gives us a sense of how busy the pool is
	idle := int32(s.pool.workers.size())
	active := running - idle
	queued := s.pool.metrics.BlockingTasks.Load()

	// Use atomic maxWorkers for thread-safe access
	maxWorkers := s.pool.maxWorkers.Load()
	if maxWorkers == 0 {
		maxWorkers = running
	}

	load := float64(active+queued) / float64(maxWorkers)
	if load > 1.0 {
		load = 1.0
	}
	return load
}

// scaleUp increases the number of workers
func (s *AutoScaler) scaleUp(currentWorkers int32) {
	newWorkers := currentWorkers + s.config.ScaleUpStep
	if newWorkers > s.config.MaxWorkers {
		newWorkers = s.config.MaxWorkers
	}

	if newWorkers <= currentWorkers {
		return
	}

	// Create additional workers
	added := int32(0)
	for i := currentWorkers; i < newWorkers; i++ {
		w := s.pool.createWorkerInternal()
		if w == nil {
			break
		}
		w.run()
		s.pool.workers.push(w)
		s.pool.metrics.IdleWorkers.Add(1)
		added++
	}

	if added > 0 {
		s.lastScale = time.Now()
		s.scaleUpCount.Add(1)

		// Trigger scale up hook
		if s.pool.hooks != nil && s.pool.hooks.HasHooks(HookOnScaleUp) {
			s.pool.hooks.Trigger(HookOnScaleUp, &ScaleInfo{
				PoolName:   s.pool.name,
				OldSize:    currentWorkers,
				NewSize:    currentWorkers + added,
				Reason:     "load threshold exceeded",
				LoadFactor: s.emaLoad,
				ScaledAt:   time.Now(),
			})
		}
	}
}

// scaleDown decreases the number of workers
func (s *AutoScaler) scaleDown(currentWorkers int32) {
	targetWorkers := currentWorkers - s.config.ScaleDownStep
	if targetWorkers < s.config.MinWorkers {
		targetWorkers = s.config.MinWorkers
	}

	if targetWorkers >= currentWorkers {
		return
	}

	// Remove idle workers
	removed := int32(0)
	toRemove := currentWorkers - targetWorkers

	for i := int32(0); i < toRemove; i++ {
		w := s.pool.workers.pop()
		if w == nil {
			break
		}
		s.pool.metrics.IdleWorkers.Add(-1)
		// Signal worker to stop
		w.finish()
		removed++
	}

	if removed > 0 {
		s.lastScale = time.Now()
		s.scaleDownCount.Add(1)

		// Trigger scale down hook
		if s.pool.hooks != nil && s.pool.hooks.HasHooks(HookOnScaleDown) {
			s.pool.hooks.Trigger(HookOnScaleDown, &ScaleInfo{
				PoolName:   s.pool.name,
				OldSize:    currentWorkers,
				NewSize:    currentWorkers - removed,
				Reason:     "load below threshold",
				LoadFactor: s.emaLoad,
				ScaledAt:   time.Now(),
			})
		}
	}
}

// GetStats returns scaling statistics
func (s *AutoScaler) GetStats() ScalerStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return ScalerStats{
		EMALoad:        s.emaLoad,
		LastScaleTime:  s.lastScale,
		ScaleUpCount:   s.scaleUpCount.Load(),
		ScaleDownCount: s.scaleDownCount.Load(),
		IsRunning:      s.running.Load(),
	}
}

// ScalerStats holds auto-scaler statistics
type ScalerStats struct {
	EMALoad        float64
	LastScaleTime  time.Time
	ScaleUpCount   int64
	ScaleDownCount int64
	IsRunning      bool
}

// Reset resets the scaler state
func (s *AutoScaler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastScale = time.Time{}
	s.emaLoad = 0
	s.initialized = false
	s.scaleUpCount.Store(0)
	s.scaleDownCount.Store(0)
}
