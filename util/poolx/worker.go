package poolx

import (
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Enhanced Task Options
// ============================================================================

// TaskOptions contains optional settings for a task
type TaskOptions struct {
	Priority int           // Task priority (higher = more important)
	Timeout  time.Duration // Per-task timeout (0 = no timeout)
	ID       uint64        // Task ID (auto-generated if 0)
}

// TaskOption is a function that configures TaskOptions
type TaskOption func(*TaskOptions)

// WithTaskPriority sets the task priority
func WithTaskPriority(priority int) TaskOption {
	return func(o *TaskOptions) {
		o.Priority = priority
	}
}

// WithTaskTimeout sets the per-task timeout
func WithTaskTimeout(timeout time.Duration) TaskOption {
	return func(o *TaskOptions) {
		o.Timeout = timeout
	}
}

// WithTaskID sets the task ID
func WithTaskID(id uint64) TaskOption {
	return func(o *TaskOptions) {
		o.ID = id
	}
}

// ============================================================================
// Work-Stealing Scheduler
// ============================================================================

// StealingScheduler manages work stealing between workers
type StealingScheduler struct {
	mu      sync.RWMutex
	workers map[int32]*WorkStealingDeque[task]
	ids     []int32 // Cached list of worker IDs for random selection
}

// NewStealingScheduler creates a new work stealing scheduler
func NewStealingScheduler() *StealingScheduler {
	return &StealingScheduler{
		workers: make(map[int32]*WorkStealingDeque[task]),
	}
}

// Register adds a worker's queue to the scheduler
func (s *StealingScheduler) Register(id int32, queue *WorkStealingDeque[task]) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workers[id] = queue
	s.ids = append(s.ids, id)
}

// Unregister removes a worker from the scheduler
func (s *StealingScheduler) Unregister(id int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.workers, id)

	// Remove from ids slice
	for i, wid := range s.ids {
		if wid == id {
			s.ids = append(s.ids[:i], s.ids[i+1:]...)
			break
		}
	}
}

// Steal attempts to steal a task from another worker
func (s *StealingScheduler) Steal(thiefID int32) *task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.workers) <= 1 {
		return nil
	}

	// Try to steal from each worker (round-robin starting from a position based on thief ID)
	start := int(thiefID) % len(s.ids)
	for i := 0; i < len(s.ids); i++ {
		idx := (start + i) % len(s.ids)
		victimID := s.ids[idx]

		if victimID == thiefID {
			continue
		}

		if queue, ok := s.workers[victimID]; ok {
			if t := queue.Steal(); t != nil {
				return t
			}
		}
	}

	return nil
}

// TotalTasks returns the total number of tasks across all local queues
func (s *StealingScheduler) TotalTasks() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	for _, queue := range s.workers {
		total += queue.Len()
	}
	return total
}

// ============================================================================
// Worker Interface (for extensibility)
// ============================================================================

// WorkerInterface represents a worker that executes tasks.
// This interface can be used for custom worker implementations.
type WorkerInterface interface {
	// Run starts the worker's main loop
	Run()
	// Stop signals the worker to stop
	Stop()
	// ID returns the worker's unique identifier
	ID() int32
	// LastActive returns the last activity timestamp (UnixNano)
	LastActive() int64
	// IsRunning returns true if the worker is running
	IsRunning() bool
}

// ============================================================================
// Enhanced Worker Stack (with Spinlock)
// ============================================================================

// WorkerStack is an enhanced worker stack using spinlock for better performance
// in low-contention scenarios. This can be used as an alternative to the
// mutex-based workerStack in pool.go.
type WorkerStack struct {
	items  []WorkerInterface
	expiry []WorkerInterface
	head   int
	len    int
	cap    int
	lock   Spinlock
}

// NewWorkerStack creates a new worker stack with the given capacity
func NewWorkerStack(cap int) *WorkerStack {
	return &WorkerStack{
		items: make([]WorkerInterface, cap),
		cap:   cap,
	}
}

// Push adds a worker to the stack
func (s *WorkerStack) Push(w WorkerInterface) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.len >= s.cap {
		return false
	}
	s.items[s.head] = w
	s.head = (s.head + 1) % s.cap
	s.len++
	return true
}

// Pop removes and returns a worker from the stack
func (s *WorkerStack) Pop() WorkerInterface {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.len == 0 {
		return nil
	}
	s.head = (s.head - 1 + s.cap) % s.cap
	w := s.items[s.head]
	s.items[s.head] = nil
	s.len--
	return w
}

// Size returns the number of workers in the stack
func (s *WorkerStack) Size() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.len
}

// RetrieveExpiry retrieves and removes workers that have been idle longer than duration
func (s *WorkerStack) RetrieveExpiry(duration time.Duration) []WorkerInterface {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.len == 0 {
		return nil
	}

	s.expiry = s.expiry[:0]
	now := time.Now()

	// Check for expired workers
	for i := 0; i < s.len; i++ {
		idx := (s.head - s.len + i + s.cap) % s.cap
		w := s.items[idx]
		if w != nil && now.Sub(time.Unix(0, w.LastActive())) > duration {
			s.expiry = append(s.expiry, w)
			s.items[idx] = nil
		}
	}

	// Compact the array
	newItems := make([]WorkerInterface, s.cap)
	newHead := 0
	for i := 0; i < s.len; i++ {
		idx := (s.head - s.len + i + s.cap) % s.cap
		if s.items[idx] != nil {
			newItems[newHead] = s.items[idx]
			newHead++
		}
	}
	s.items = newItems
	s.len = newHead
	s.head = newHead

	return s.expiry
}

// ============================================================================
// Load Calculator for Auto-Scaling
// ============================================================================

// LoadCalculator calculates the load factor using EMA (Exponential Moving Average)
type LoadCalculator struct {
	mu          sync.Mutex
	emaLoad     float64
	alpha       float64
	initialized bool
}

// NewLoadCalculator creates a new load calculator with the given EMA alpha
func NewLoadCalculator(alpha float64) *LoadCalculator {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3 // Default
	}
	return &LoadCalculator{
		alpha: alpha,
	}
}

// Update updates the EMA with a new load value
func (c *LoadCalculator) Update(load float64) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		c.emaLoad = load
		c.initialized = true
	} else {
		c.emaLoad = c.alpha*load + (1-c.alpha)*c.emaLoad
	}
	return c.emaLoad
}

// Get returns the current EMA load value
func (c *LoadCalculator) Get() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emaLoad
}

// Reset resets the calculator state
func (c *LoadCalculator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emaLoad = 0
	c.initialized = false
}

// ============================================================================
// Task Metrics Collector
// ============================================================================

// TaskMetrics collects per-task metrics with atomic operations
type TaskMetrics struct {
	_          CacheLinePad
	Count      atomic.Int64
	TotalTime  atomic.Int64
	MinTime    atomic.Int64
	MaxTime    atomic.Int64
	_          CacheLinePad
	ErrorCount atomic.Int64
	_          CacheLinePad
}

// Record records a task execution
func (m *TaskMetrics) Record(duration time.Duration, err bool) {
	m.Count.Add(1)
	m.TotalTime.Add(int64(duration))

	// Update min/max atomically
	nanos := int64(duration)
	for {
		min := m.MinTime.Load()
		if min != 0 && min <= nanos {
			break
		}
		if m.MinTime.CompareAndSwap(min, nanos) {
			break
		}
	}
	for {
		max := m.MaxTime.Load()
		if max >= nanos {
			break
		}
		if m.MaxTime.CompareAndSwap(max, nanos) {
			break
		}
	}

	if err {
		m.ErrorCount.Add(1)
	}
}

// Snapshot returns a snapshot of the metrics
func (m *TaskMetrics) Snapshot() TaskMetricsSnapshot {
	count := m.Count.Load()
	totalTime := m.TotalTime.Load()
	var avgTime int64
	if count > 0 {
		avgTime = totalTime / count
	}
	return TaskMetricsSnapshot{
		Count:      count,
		TotalTime:  time.Duration(totalTime),
		AvgTime:    time.Duration(avgTime),
		MinTime:    time.Duration(m.MinTime.Load()),
		MaxTime:    time.Duration(m.MaxTime.Load()),
		ErrorCount: m.ErrorCount.Load(),
	}
}

// Reset resets all metrics
func (m *TaskMetrics) Reset() {
	m.Count.Store(0)
	m.TotalTime.Store(0)
	m.MinTime.Store(0)
	m.MaxTime.Store(0)
	m.ErrorCount.Store(0)
}

// TaskMetricsSnapshot is a point-in-time snapshot of task metrics
type TaskMetricsSnapshot struct {
	Count      int64
	TotalTime  time.Duration
	AvgTime    time.Duration
	MinTime    time.Duration
	MaxTime    time.Duration
	ErrorCount int64
}

// ErrorRate returns the error rate as a percentage
func (s TaskMetricsSnapshot) ErrorRate() float64 {
	if s.Count == 0 {
		return 0
	}
	return float64(s.ErrorCount) / float64(s.Count) * 100
}
