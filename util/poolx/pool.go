package poolx

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Configuration Options
// ============================================================================

// Config holds pool configuration
type Config struct {
	// Basic configuration
	MaxWorkers   int32         // Maximum number of workers
	MinWorkers   int32         // Minimum number of workers (preheat)
	QueueSize    int32         // Task queue size
	WorkerExpiry time.Duration // Worker idle expiry time

	// Pre-allocation
	PreAlloc bool // Whether to pre-allocate worker resources

	// Auto-scaling configuration
	EnableAutoScale bool          // Enable auto-scaling
	ScaleInterval   time.Duration // Scaling check interval
	ScaleUpRatio    float64       // Scale up when load exceeds this ratio
	ScaleDownRatio  float64       // Scale down when load is below this ratio

	// Panic recovery
	PanicHandler func(any) // Panic handler function

	// Work stealing
	EnableWorkStealing bool  // Enable work stealing
	StealBatchSize     int32 // Number of tasks to steal at once

	// Blocking control
	MaxBlockingTasks int32 // Maximum blocking tasks (0 = unlimited)
	NonBlocking      bool  // Non-blocking mode (reject when full)

	// Priority queue
	EnablePriorityQueue bool // Enable priority-based scheduling

	// Hooks
	Hooks *Hooks // Lifecycle hooks

	// Logger
	Logger Logger // Logger interface
}

// Logger is the logging interface
type Logger interface {
	Printf(format string, args ...any)
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	numCPU := int32(runtime.NumCPU())
	// 确保 numCPU 至少为 1（防御性编程）
	if numCPU < 1 {
		numCPU = 1
	}
	return Config{
		MaxWorkers:          numCPU * 4,
		MinWorkers:          numCPU,
		QueueSize:           numCPU * 16,
		WorkerExpiry:        10 * time.Second,
		PreAlloc:            false,
		EnableAutoScale:     true,
		ScaleInterval:       time.Second,
		ScaleUpRatio:        0.8,
		ScaleDownRatio:      0.2,
		PanicHandler:        defaultPanicHandler,
		EnableWorkStealing:  true,
		StealBatchSize:      4,
		MaxBlockingTasks:    0,
		NonBlocking:         false,
		EnablePriorityQueue: false,
		Hooks:               nil,
		Logger:              nil,
	}
}

func defaultPanicHandler(v any) {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	fmt.Printf("[POOL PANIC] recovered: %v\n%s\n", v, buf[:n])
}

// Option is a configuration option function
type Option func(*Config)

// WithMaxWorkers sets the maximum number of workers
func WithMaxWorkers(n int32) Option {
	return func(c *Config) {
		c.MaxWorkers = n
	}
}

// WithMinWorkers sets the minimum number of workers (preheat)
func WithMinWorkers(n int32) Option {
	return func(c *Config) {
		c.MinWorkers = n
	}
}

// WithQueueSize sets the queue size
func WithQueueSize(n int32) Option {
	return func(c *Config) {
		c.QueueSize = n
	}
}

// WithWorkerExpiry sets the worker idle expiry time
func WithWorkerExpiry(d time.Duration) Option {
	return func(c *Config) {
		c.WorkerExpiry = d
	}
}

// WithPreAlloc enables/disables pre-allocation
func WithPreAlloc(enable bool) Option {
	return func(c *Config) {
		c.PreAlloc = enable
	}
}

// WithAutoScale enables/disables auto-scaling
func WithAutoScale(enable bool) Option {
	return func(c *Config) {
		c.EnableAutoScale = enable
	}
}

// WithScaleInterval sets the scaling check interval
func WithScaleInterval(d time.Duration) Option {
	return func(c *Config) {
		c.ScaleInterval = d
	}
}

// WithPanicHandler sets the panic handler function
func WithPanicHandler(h func(any)) Option {
	return func(c *Config) {
		c.PanicHandler = h
	}
}

// WithWorkStealing enables/disables work stealing
func WithWorkStealing(enable bool) Option {
	return func(c *Config) {
		c.EnableWorkStealing = enable
	}
}

// WithMaxBlockingTasks sets the maximum blocking tasks
func WithMaxBlockingTasks(n int32) Option {
	return func(c *Config) {
		c.MaxBlockingTasks = n
	}
}

// WithNonBlocking enables non-blocking mode
func WithNonBlocking(enable bool) Option {
	return func(c *Config) {
		c.NonBlocking = enable
	}
}

// WithPriorityQueue enables priority-based scheduling
func WithPriorityQueue(enable bool) Option {
	return func(c *Config) {
		c.EnablePriorityQueue = enable
	}
}

// WithHooks sets the lifecycle hooks
func WithHooks(hooks *Hooks) Option {
	return func(c *Config) {
		c.Hooks = hooks
	}
}

// WithLogger sets the logger interface
func WithLogger(l Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// ============================================================================
// Performance Metrics
// ============================================================================

// Metrics holds performance metrics
type Metrics struct {
	// Counters
	SubmittedTasks atomic.Int64 // Total submitted tasks
	CompletedTasks atomic.Int64 // Total completed tasks
	FailedTasks    atomic.Int64 // Failed tasks (panic)
	RejectedTasks  atomic.Int64 // Rejected tasks
	StolenTasks    atomic.Int64 // Stolen tasks (work stealing)

	// Time statistics
	TotalWaitTime atomic.Int64 // Total wait time (nanoseconds)
	TotalExecTime atomic.Int64 // Total execution time (nanoseconds)

	// Current state
	RunningWorkers atomic.Int32 // Currently running workers
	IdleWorkers    atomic.Int32 // Currently idle workers
	QueuedTasks    atomic.Int32 // Currently queued tasks
	BlockingTasks  atomic.Int32 // Currently blocking callers

	// Peak values
	PeakWorkers atomic.Int32 // Peak worker count
	PeakQueued  atomic.Int32 // Peak queued tasks
}

// Snapshot returns a metrics snapshot
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		SubmittedTasks: m.SubmittedTasks.Load(),
		CompletedTasks: m.CompletedTasks.Load(),
		FailedTasks:    m.FailedTasks.Load(),
		RejectedTasks:  m.RejectedTasks.Load(),
		StolenTasks:    m.StolenTasks.Load(),
		TotalWaitTime:  time.Duration(m.TotalWaitTime.Load()),
		TotalExecTime:  time.Duration(m.TotalExecTime.Load()),
		RunningWorkers: m.RunningWorkers.Load(),
		IdleWorkers:    m.IdleWorkers.Load(),
		QueuedTasks:    m.QueuedTasks.Load(),
		BlockingTasks:  m.BlockingTasks.Load(),
		PeakWorkers:    m.PeakWorkers.Load(),
		PeakQueued:     m.PeakQueued.Load(),
	}
}

// Reset resets all metrics
func (m *Metrics) Reset() {
	m.SubmittedTasks.Store(0)
	m.CompletedTasks.Store(0)
	m.FailedTasks.Store(0)
	m.RejectedTasks.Store(0)
	m.StolenTasks.Store(0)
	m.TotalWaitTime.Store(0)
	m.TotalExecTime.Store(0)
	m.PeakWorkers.Store(0)
	m.PeakQueued.Store(0)
}

// MetricsSnapshot is a point-in-time metrics snapshot
type MetricsSnapshot struct {
	SubmittedTasks int64
	CompletedTasks int64
	FailedTasks    int64
	RejectedTasks  int64
	StolenTasks    int64
	TotalWaitTime  time.Duration
	TotalExecTime  time.Duration
	RunningWorkers int32
	IdleWorkers    int32
	QueuedTasks    int32
	BlockingTasks  int32
	PeakWorkers    int32
	PeakQueued     int32
}

// AvgWaitTime returns the average wait time
func (s MetricsSnapshot) AvgWaitTime() time.Duration {
	if s.CompletedTasks == 0 {
		return 0
	}
	return time.Duration(int64(s.TotalWaitTime) / s.CompletedTasks)
}

// AvgExecTime returns the average execution time
func (s MetricsSnapshot) AvgExecTime() time.Duration {
	if s.CompletedTasks == 0 {
		return 0
	}
	return time.Duration(int64(s.TotalExecTime) / s.CompletedTasks)
}

// Throughput returns the throughput (tasks/second)
func (s MetricsSnapshot) Throughput(elapsed time.Duration) float64 {
	if elapsed == 0 {
		return 0
	}
	return float64(s.CompletedTasks) / elapsed.Seconds()
}

// SuccessRate returns the success rate
func (s MetricsSnapshot) SuccessRate() float64 {
	total := s.CompletedTasks + s.FailedTasks
	if total == 0 {
		return 1.0
	}
	return float64(s.CompletedTasks) / float64(total)
}

// ============================================================================
// Lock-Free Worker Stack (inspired by ants)
// ============================================================================

// workerStack is an optimized worker stack using spinlock for better performance
// in low-contention scenarios
type workerStack struct {
	_      CacheLinePad // Prevent false sharing
	items  []*worker
	expiry []*worker
	head   int
	len    int
	cap    int
	lock   Spinlock // Use spinlock instead of mutex for lower overhead
	_      CacheLinePad
}

func newWorkerStack(capacity int) *workerStack {
	return &workerStack{
		items: make([]*worker, capacity),
		cap:   capacity,
	}
}

func (s *workerStack) push(w *worker) bool {
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

func (s *workerStack) pop() *worker {
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

func (s *workerStack) size() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.len
}

// resize 调整空闲栈容量，并保持当前 Worker 的栈顺序。
func (s *workerStack) resize(capacity int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if capacity <= 0 || capacity == s.cap {
		return
	}
	if capacity < s.len {
		capacity = s.len
	}
	items := make([]*worker, capacity)
	for index := 0; index < s.len; index++ {
		oldIndex := (s.head - s.len + index + s.cap) % s.cap
		items[index] = s.items[oldIndex]
	}
	s.items = items
	s.cap = capacity
	s.head = s.len % capacity
	s.expiry = nil
}

func (s *workerStack) retrieveExpiry(duration time.Duration) []*worker {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.len == 0 {
		return nil
	}

	s.expiry = s.expiry[:0]
	now := time.Now()

	var surviving []*worker

	for i := 0; i < s.len; i++ {
		idx := (s.head - s.len + i + s.cap) % s.cap
		w := s.items[idx]
		if w != nil && now.Sub(time.Unix(0, w.lastActive.Load())) > duration {
			s.expiry = append(s.expiry, w)
		} else if w != nil {
			surviving = append(surviving, w)
		}
		s.items[idx] = nil
	}

	// Rebuild ring from surviving items
	copy(s.items, surviving)
	s.head = len(surviving)
	s.len = len(surviving)

	return s.expiry
}

// ============================================================================
// Task Definition
// ============================================================================

// task wraps a function to be executed
type task struct {
	fn        func()
	submitted int64 // UnixNano timestamp (lazy init, 0 = not set)
	priority  int
	id        uint64
}

var taskPool = sync.Pool{
	New: func() any {
		return &task{}
	},
}

func mustPoolValue[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic(fmt.Sprintf("poolx: unexpected pooled value type %T", value))
	}
	return typed
}

// acquireTaskFast is the fast path for simple task submission (no options, no hooks)
func acquireTaskFast(fn func()) *task {
	t := mustPoolValue[*task](taskPool.Get())
	t.fn = fn
	t.submitted = 0 // Lazy init - only set when needed
	t.priority = 0
	t.id = 0
	return t
}

func acquireTaskWithOptions(fn func(), opts *TaskOptions) *task {
	t := mustPoolValue[*task](taskPool.Get())
	t.fn = fn
	t.submitted = time.Now().UnixNano()
	if opts != nil {
		t.priority = opts.Priority
		t.id = opts.ID
	} else {
		t.priority = PriorityNormal
		t.id = 0
	}
	return t
}

// getSubmittedTime returns the submitted time, initializing if needed
func (t *task) getSubmittedTime() time.Time {
	if t.submitted == 0 {
		return time.Now() // Task was just created
	}
	return time.Unix(0, t.submitted)
}

func releaseTask(t *task) {
	t.fn = nil
	t.priority = 0
	t.id = 0
	taskPool.Put(t)
}

// ============================================================================
// Worker Definition
// ============================================================================

// worker represents a worker goroutine
type worker struct {
	pool       *Pool
	generation *poolGeneration
	taskCh     chan *task
	localQueue *WorkStealingDeque[task] // Local queue for work stealing
	lastActive atomic.Int64
	id         int32
}

func (w *worker) run() {
	go w.loop()
}

func (w *worker) loop() {
	pool := w.pool
	generation := w.generation
	defer func() {
		// Unregister from work stealing scheduler
		if pool.config.EnableWorkStealing && generation.stealingScheduler != nil && w.localQueue != nil {
			generation.stealingScheduler.Unregister(w.id)
		}

		generation.workerCount.Add(-1)
		generation.metrics.RunningWorkers.Add(-1)
		pool.lock.Lock()
		if pool.generationRunningLocked(generation) {
			pool.waiters.signalLocked()
		}
		pool.lock.Unlock()

		// Trigger worker stop hook before wg.Done to ensure
		// hooks complete before Release returns
		if pool.hooks != nil && pool.hooks.HasHooks(HookOnWorkerStop) {
			pool.hooks.Trigger(HookOnWorkerStop, &WorkerInfo{
				ID:        w.id,
				PoolName:  pool.name,
				StoppedAt: time.Now(),
			})
		}

		generation.wg.Done()
		pool.workerCache.Put(w)
	}()

	// Register with work stealing scheduler if enabled
	if pool.config.EnableWorkStealing && generation.stealingScheduler != nil && w.localQueue != nil {
		generation.stealingScheduler.Register(w.id, w.localQueue)
	}

	// Trigger worker start hook
	if pool.hooks != nil && pool.hooks.HasHooks(HookOnWorkerStart) {
		pool.hooks.Trigger(HookOnWorkerStart, &WorkerInfo{
			ID:        w.id,
			PoolName:  pool.name,
			StartedAt: time.Now(),
		})
	}

	for t := range w.taskCh {
		if t == nil {
			return
		}

		w.execute(t)

		// Try to process tasks from local queue (work stealing)
		if pool.config.EnableWorkStealing && w.localQueue != nil {
			w.processLocalQueue()
		}

		w.lastActive.Store(time.Now().UnixNano())

		// Return self to idle stack
		if !pool.revertWorker(w) {
			return
		}
	}
}

// processLocalQueue processes tasks from local queue and tries stealing
func (w *worker) processLocalQueue() {
	// Process all tasks in local queue
	for {
		t := w.localQueue.PopBottom()
		if t == nil {
			break
		}
		w.execute(t)
	}

	// Try to steal from other workers if enabled
	if w.generation.stealingScheduler != nil {
		stolen := w.generation.stealingScheduler.steal(w.id)
		if stolen != nil {
			w.generation.metrics.StolenTasks.Add(1)
			w.execute(stolen)
		}
	}
}

func (w *worker) execute(t *task) {
	generation := w.generation
	startTime := time.Now()
	submittedTime := t.getSubmittedTime()
	waitTime := startTime.Sub(submittedTime)

	// Create task info for hooks
	var taskInfo *TaskInfo
	if w.pool.hooks != nil {
		taskInfo = &TaskInfo{
			ID:          t.id,
			PoolName:    w.pool.name,
			WorkerID:    w.id,
			Priority:    t.priority,
			SubmittedAt: submittedTime,
			StartedAt:   startTime,
			WaitTime:    waitTime,
		}
	}

	// Trigger before task hook
	if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookBeforeTask) {
		w.pool.hooks.Trigger(HookBeforeTask, taskInfo)
	}

	panicked, panicVal := w.executeDirect(t)

	execTime := time.Since(startTime)

	// Update metrics
	generation.metrics.TotalWaitTime.Add(int64(waitTime))
	generation.metrics.TotalExecTime.Add(int64(execTime))

	if panicked {
		generation.metrics.FailedTasks.Add(1)
		if handler := w.pool.panicHandler.Load(); handler != nil {
			// 包装 panic handler 调用，防止它本身 panic 导致 goroutine 崩溃
			func() {
				defer func() {
					if recover() != nil {
						return
					}
				}()
				handler.handle(panicVal)
			}()
		}

		// Trigger panic hook
		if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookOnPanic) {
			if taskInfo != nil {
				taskInfo.Error = panicVal
				taskInfo.FinishedAt = time.Now()
				taskInfo.ExecTime = execTime
			}
			w.pool.hooks.Trigger(HookOnPanic, taskInfo)
		}
	} else {
		generation.metrics.CompletedTasks.Add(1)
	}

	// Trigger after task hook
	if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookAfterTask) {
		if taskInfo != nil {
			taskInfo.FinishedAt = time.Now()
			taskInfo.ExecTime = execTime
		}
		w.pool.hooks.Trigger(HookAfterTask, taskInfo)
	}

	releaseTask(t)
}

func (w *worker) executeDirect(t *task) (panicked bool, panicVal any) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			panicVal = r
		}
	}()
	t.fn()
	return false, nil
}

func (w *worker) finish() {
	w.taskCh <- nil
}

// ============================================================================
// Pool Definition
// ============================================================================

// Pool is a high-performance goroutine pool
type Pool struct {
	config Config
	name   string

	// Task queue
	taskQueue chan *task

	// Worker management
	workerID    atomic.Int32
	workerCache sync.Pool

	// Auto-scaler
	scaler *AutoScaler

	// Blocking control
	waiters *waitNotifier

	// State
	state        atomic.Int32 // 0: running, 1: closed
	generationID atomic.Uint64
	generation   atomic.Pointer[poolGeneration]

	// Hooks
	hooks *Hooks

	// Task ID generator
	taskIDGen atomic.Uint64

	// Dynamic capacity (atomic for concurrent access)
	maxWorkers atomic.Int32

	panicHandler atomic.Pointer[panicHandlerSnapshot]

	lifecycleMu sync.Mutex
	lock        sync.Mutex
}

const (
	stateRunning = iota
	stateClosed
)

// ============================================================================
// Simplified Pool Constructors
// ============================================================================

// NewSimple creates a simple pool with the specified number of workers.
// This is the simplest way to create a pool.
//
// Example:
//
//	p := pool.NewSimple(4) // 4 workers
//	defer p.Release()
//	p.Submit(fn)
func NewSimple(maxWorkers int) *Pool {
	return New("", WithMaxWorkers(int32(maxWorkers)), WithAutoScale(false))
}

// NewAuto creates a pool with automatic scaling enabled.
// Workers will be automatically adjusted based on load.
//
// Example:
//
//	p := pool.NewAuto(10, 100) // min 10, max 100 workers
//	defer p.Release()
func NewAuto(minWorkers, maxWorkers int) *Pool {
	return New("",
		WithMinWorkers(int32(minWorkers)),
		WithMaxWorkers(int32(maxWorkers)),
		WithAutoScale(true),
	)
}

// NewWithName creates a named pool that can be retrieved later via GetPool.
//
// Example:
//
//	pool.NewWithName("http-pool", 100)
//	// later...
//	p, _ := pool.GetPool("http-pool")
func NewWithName(name string, maxWorkers int) *Pool {
	return New(name, WithMaxWorkers(int32(maxWorkers)), WithAutoScale(false))
}

// New creates a new pool
func New(name string, opts ...Option) *Pool {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	// Validate parameters
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = int32(runtime.NumCPU())
	}
	if config.MinWorkers < 0 {
		config.MinWorkers = 0
	}
	if config.MinWorkers > config.MaxWorkers {
		config.MinWorkers = config.MaxWorkers
	}
	if config.QueueSize <= 0 {
		config.QueueSize = config.MaxWorkers * 2
	}

	p := &Pool{
		config:    config,
		name:      name,
		taskQueue: make(chan *task, config.QueueSize),
		hooks:     config.Hooks,
		waiters:   newWaitNotifier(),
	}

	// Initialize atomic maxWorkers for concurrent access
	p.maxWorkers.Store(config.MaxWorkers)
	p.panicHandler.Store(newPanicHandlerSnapshot(config.PanicHandler))
	generation := newPoolGeneration(p.generationID.Add(1), config, config.MaxWorkers)
	p.generation.Store(generation)

	p.workerCache.New = func() any {
		return &worker{}
	}

	// 预分配 worker 和 task 对象缓存，减少 GC 压力。
	// 注意：这只是预热 sync.Pool 缓存，不会启动实际的 worker。
	// 使用 WithMinWorkers 来预启动实际的 worker。
	if config.PreAlloc {
		// 预分配 worker 对象到缓存
		for range config.MaxWorkers {
			w := mustPoolValue[*worker](p.workerCache.Get())
			p.workerCache.Put(w)
		}
		// 预分配 task 对象到缓存
		for range config.QueueSize {
			t := mustPoolValue[*task](taskPool.Get())
			taskPool.Put(t)
		}
	}

	// Preheat workers
	p.lock.Lock()
	p.preheatLocked(generation)
	p.lock.Unlock()

	// Start cleaner goroutine
	go p.purgeStaleWorkers(generation)

	// Start auto-scaler if enabled
	if config.EnableAutoScale {
		scalerConfig := ScalerConfig{
			ScaleInterval:      config.ScaleInterval,
			ScaleUpThreshold:   config.ScaleUpRatio,
			ScaleDownThreshold: config.ScaleDownRatio,
			MinWorkers:         config.MinWorkers,
			MaxWorkers:         config.MaxWorkers,
			ScaleUpStep:        2,
			ScaleDownStep:      1,
			CooldownPeriod:     5 * time.Second,
			EMAAlpha:           0.3,
		}
		p.scaler = NewAutoScaler(p, scalerConfig)
		p.scaler.Start()
	}

	// Register to named pools
	if name != "" {
		namedPools.Store(name, p)
	}

	return p
}

// preheatLocked 在调用方持有生命周期锁时创建本代际的最小 Worker 集合。
func (p *Pool) preheatLocked(generation *poolGeneration) {
	for i := int32(0); i < p.config.MinWorkers; i++ {
		w := p.createWorkerLocked(generation)
		if w == nil {
			break
		}
		w.run()
		generation.workers.push(w)
		generation.metrics.IdleWorkers.Add(1)
	}
}

// createWorkerLocked 只为仍处于运行状态的指定代际登记 Worker。
func (p *Pool) createWorkerLocked(generation *poolGeneration) *worker {
	if generation == nil || p.generation.Load() != generation || generation.state.Load() == stateClosed {
		return nil
	}
	if generation.workerCount.Load() >= p.maxWorkers.Load() {
		return nil
	}
	generation.workerCount.Add(1)
	generation.wg.Add(1)

	id := p.workerID.Add(1)

	w := mustPoolValue[*worker](p.workerCache.Get())
	w.pool = p
	w.generation = generation
	w.id = id
	w.lastActive.Store(time.Now().UnixNano())
	w.taskCh = make(chan *task, 4)

	if p.config.EnableWorkStealing {
		w.localQueue = NewWorkStealingDeque[task](64)
	} else {
		w.localQueue = nil
	}

	generation.metrics.RunningWorkers.Add(1)

	for {
		peak := generation.metrics.PeakWorkers.Load()
		current := generation.metrics.RunningWorkers.Load()
		if current <= peak {
			break
		}
		if generation.metrics.PeakWorkers.CompareAndSwap(peak, current) {
			break
		}
	}

	return w
}

// retrieveWorkerLocked 从指定代际获取或创建一个 Worker。
func (p *Pool) retrieveWorkerLocked(generation *poolGeneration) *worker {
	if generation == nil || p.generation.Load() != generation || generation.state.Load() == stateClosed {
		return nil
	}
	if w := generation.workers.pop(); w != nil {
		generation.metrics.IdleWorkers.Add(-1)
		return w
	}

	if w := p.createWorkerLocked(generation); w != nil {
		w.run()
		return w
	}

	return nil
}

// revertWorker returns a worker to idle
func (p *Pool) revertWorker(w *worker) bool {
	w.lastActive.Store(time.Now().UnixNano())
	generation := w.generation

	p.lock.Lock()
	defer p.lock.Unlock()

	if generation.state.Load() == stateClosed {
		return false
	}
	if generation.workerCount.Load() > p.maxWorkers.Load() {
		return false
	}

	if generation.workers.push(w) {
		generation.metrics.IdleWorkers.Add(1)
		p.waiters.signalLocked()
		return true
	}

	return false
}

// purgeStaleWorkers periodically cleans up expired workers
func (p *Pool) purgeStaleWorkers(generation *poolGeneration) {
	ticker := time.NewTicker(p.config.WorkerExpiry)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if generation.state.Load() == stateClosed {
				return
			}
			p.cleanupExpiredWorkers(generation)
		case <-generation.heartbeat:
			return
		}
	}
}

func (p *Pool) cleanupExpiredWorkers(generation *poolGeneration) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if generation.state.Load() == stateClosed {
		return
	}

	expired := generation.workers.retrieveExpiry(p.config.WorkerExpiry)

	generation.metrics.IdleWorkers.Add(-int32(len(expired)))

	minToKeep := p.config.MinWorkers
	currentRunning := generation.workerCount.Load()

	for _, w := range expired {
		if currentRunning <= minToKeep {
			if generation.workers.push(w) {
				generation.metrics.IdleWorkers.Add(1)
			} else {
				w.finish()
			}
		} else {
			w.finish()
			currentRunning--
		}
	}
}

// Submit submits a task (blocks until accepted or pool closed)
func (p *Pool) Submit(fn func()) error {
	// Fast path: no hooks, no options
	if p.hooks == nil && !p.config.NonBlocking && p.config.MaxBlockingTasks == 0 {
		return p.submitFast(fn)
	}
	return p.SubmitWithOptions(fn)
}

func (p *Pool) loadRunningGeneration() *poolGeneration {
	generation := p.generation.Load()
	if generation == nil || generation.state.Load() == stateClosed {
		return nil
	}
	return generation
}

func (p *Pool) generationRunningLocked(generation *poolGeneration) bool {
	return generation != nil && p.generation.Load() == generation && generation.state.Load() == stateRunning
}

// submitFast is the optimized submit path for common case (no hooks, no special options)
func (p *Pool) submitFast(fn func()) error {
	generation := p.loadRunningGeneration()
	if generation == nil {
		return ErrPoolClosed
	}

	t := acquireTaskFast(fn)
	generation.metrics.SubmittedTasks.Add(1)

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		releaseTask(t)
		return ErrPoolClosed
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		w.taskCh <- t
		return nil
	}
	p.lock.Unlock()

	// Block and wait (no blocking limit check in fast path)
	generation.blockingCount.Add(1)
	generation.metrics.BlockingTasks.Add(1)
	defer func() {
		generation.blockingCount.Add(-1)
		generation.metrics.BlockingTasks.Add(-1)
	}()

	p.lock.Lock()
	for {
		if !p.generationRunningLocked(generation) {
			p.lock.Unlock()
			releaseTask(t)
			return ErrPoolClosed
		}

		if w := p.retrieveWorkerLocked(generation); w != nil {
			p.lock.Unlock()
			w.taskCh <- t
			return nil
		}

		// Wait
		p.waiters.waitLocked(&p.lock, nil)
	}
}

// SubmitWithOptions submits a task with optional settings
func (p *Pool) SubmitWithOptions(fn func(), opts ...TaskOption) error {
	generation := p.loadRunningGeneration()
	if generation == nil {
		return ErrPoolClosed
	}

	// Process options
	var taskOpts *TaskOptions
	if len(opts) > 0 {
		taskOpts = &TaskOptions{Priority: PriorityNormal}
		for _, opt := range opts {
			opt(taskOpts)
		}
		if taskOpts.ID == 0 {
			taskOpts.ID = p.taskIDGen.Add(1)
		}
	}

	t := acquireTaskWithOptions(fn, taskOpts)
	generation.metrics.SubmittedTasks.Add(1)

	// Trigger before submit hook
	if p.hooks != nil && p.hooks.HasHooks(HookBeforeSubmit) {
		p.hooks.Trigger(HookBeforeSubmit, &TaskInfo{
			ID:          t.id,
			PoolName:    p.name,
			Priority:    t.priority,
			SubmittedAt: t.getSubmittedTime(),
		})
	}

	// Capture task info before sending to worker (task may be released after send)
	var taskInfo *TaskInfo
	if p.hooks != nil && p.hooks.HasHooks(HookAfterSubmit) {
		taskInfo = &TaskInfo{
			ID:          t.id,
			PoolName:    p.name,
			Priority:    t.priority,
			SubmittedAt: t.getSubmittedTime(),
		}
	}

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		releaseTask(t)
		return ErrPoolClosed
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		w.taskCh <- t

		// Trigger after submit hook
		if taskInfo != nil {
			p.hooks.Trigger(HookAfterSubmit, taskInfo)
		}
		return nil
	}
	p.lock.Unlock()

	// Non-blocking mode
	if p.config.NonBlocking {
		rejectInfo := &TaskInfo{ID: t.id, PoolName: p.name, Priority: t.priority}
		releaseTask(t)
		generation.metrics.RejectedTasks.Add(1)

		// Trigger reject hook
		if p.hooks != nil && p.hooks.HasHooks(HookOnReject) {
			p.hooks.Trigger(HookOnReject, rejectInfo)
		}
		return ErrPoolOverload
	}

	// Check blocking limit
	if p.config.MaxBlockingTasks > 0 {
		if generation.blockingCount.Load() >= p.config.MaxBlockingTasks {
			rejectInfo := &TaskInfo{ID: t.id, PoolName: p.name, Priority: t.priority}
			releaseTask(t)
			generation.metrics.RejectedTasks.Add(1)

			if p.hooks != nil && p.hooks.HasHooks(HookOnReject) {
				p.hooks.Trigger(HookOnReject, rejectInfo)
			}
			return ErrPoolOverload
		}
	}

	// Block and wait
	generation.blockingCount.Add(1)
	generation.metrics.BlockingTasks.Add(1)
	defer func() {
		generation.blockingCount.Add(-1)
		generation.metrics.BlockingTasks.Add(-1)
	}()

	p.lock.Lock()
	for {
		if !p.generationRunningLocked(generation) {
			p.lock.Unlock()
			releaseTask(t)
			return ErrPoolClosed
		}

		if w := p.retrieveWorkerLocked(generation); w != nil {
			p.lock.Unlock()
			w.taskCh <- t

			// Trigger after submit hook (use captured taskInfo)
			if taskInfo != nil {
				p.hooks.Trigger(HookAfterSubmit, taskInfo)
			}
			return nil
		}

		// Wait
		p.waiters.waitLocked(&p.lock, nil)
	}
}

// TrySubmit attempts to submit a task (non-blocking)
func (p *Pool) TrySubmit(fn func()) bool {
	generation := p.loadRunningGeneration()
	if generation == nil {
		return false
	}

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		return false
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		t := acquireTaskFast(fn) // Use fast path - no timestamp needed
		generation.metrics.SubmittedTasks.Add(1)
		w.taskCh <- t
		return true
	}
	p.lock.Unlock()

	generation.metrics.RejectedTasks.Add(1)
	return false
}

// SubmitBatch submits multiple tasks at once, reducing lock overhead.
// Returns the number of successfully submitted tasks and an error if pool is closed.
func (p *Pool) SubmitBatch(fns []func()) (int, error) {
	n := len(fns)
	if n == 0 {
		return 0, nil
	}

	generation := p.loadRunningGeneration()
	if generation == nil {
		return 0, ErrPoolClosed
	}

	submitted := 0

	// Fast path: try to submit directly to available workers
	for i := 0; i < n; i++ {
		p.lock.Lock()
		if !p.generationRunningLocked(generation) {
			p.lock.Unlock()
			generation.metrics.SubmittedTasks.Add(int64(submitted))
			return submitted, ErrPoolClosed
		}
		w := p.retrieveWorkerLocked(generation)
		p.lock.Unlock()
		if w != nil {
			t := acquireTaskFast(fns[i])
			w.taskCh <- t
			submitted++
		} else {
			// No more available workers, switch to blocking mode for remaining
			remaining := n - i
			generation.blockingCount.Add(int32(remaining))
			generation.metrics.BlockingTasks.Add(int32(remaining))

			// Submit remaining tasks in blocking mode
			for j := i; j < n; j++ {
				if err := p.submitBlockingFast(generation, fns[j]); err != nil {
					generation.blockingCount.Add(-int32(remaining))
					generation.metrics.BlockingTasks.Add(-int32(remaining))
					generation.metrics.SubmittedTasks.Add(int64(submitted))
					return submitted, err
				}
				submitted++
			}

			generation.blockingCount.Add(-int32(remaining))
			generation.metrics.BlockingTasks.Add(-int32(remaining))
			generation.metrics.SubmittedTasks.Add(int64(submitted))
			return submitted, nil
		}
	}

	// All submitted via fast path
	generation.metrics.SubmittedTasks.Add(int64(submitted))
	return submitted, nil
}

// submitBlockingFast is optimized blocking submit without metrics overhead
func (p *Pool) submitBlockingFast(generation *poolGeneration, fn func()) error {
	t := acquireTaskFast(fn)

	p.lock.Lock()
	for {
		if !p.generationRunningLocked(generation) {
			p.lock.Unlock()
			releaseTask(t)
			return ErrPoolClosed
		}

		if w := p.retrieveWorkerLocked(generation); w != nil {
			p.lock.Unlock()
			w.taskCh <- t
			return nil
		}

		// Wait
		p.waiters.waitLocked(&p.lock, nil)
	}
}

// TrySubmitBatch attempts to submit multiple tasks without blocking.
// Returns the number of successfully submitted tasks.
func (p *Pool) TrySubmitBatch(fns []func()) int {
	n := len(fns)
	generation := p.loadRunningGeneration()
	if n == 0 || generation == nil {
		return 0
	}

	submitted := 0
	for _, fn := range fns {
		p.lock.Lock()
		if !p.generationRunningLocked(generation) {
			p.lock.Unlock()
			break
		}
		w := p.retrieveWorkerLocked(generation)
		p.lock.Unlock()
		if w != nil {
			t := acquireTaskFast(fn)
			w.taskCh <- t
			submitted++
		} else {
			break
		}
	}

	// Batch update metrics
	if submitted > 0 {
		generation.metrics.SubmittedTasks.Add(int64(submitted))
	}
	if submitted < n {
		generation.metrics.RejectedTasks.Add(int64(n - submitted))
	}
	return submitted
}

// SubmitWait submits a task and waits for completion
func (p *Pool) SubmitWait(fn func()) error {
	if fn == nil {
		return invalidArgumentError("task function must not be nil")
	}

	result := make(chan error, 1)
	err := p.Submit(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				result <- newTaskPanicError(recovered)
				panic(recovered)
			}
			result <- nil
		}()
		fn()
	})
	if err != nil {
		return err
	}
	return <-result
}

// SubmitWithContext 提交接收 context 的协作式任务。
// context 在排队期间取消时返回 ctx.Err()；任务开始后由任务自身响应取消。
func (p *Pool) SubmitWithContext(ctx context.Context, fn func(context.Context)) error {
	if ctx == nil {
		return invalidArgumentError("context must not be nil")
	}
	if fn == nil {
		return invalidArgumentError("task function must not be nil")
	}
	generation := p.loadRunningGeneration()
	if generation == nil {
		return ErrPoolClosed
	}
	taskFn := func() {
		fn(ctx)
	}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		return ErrPoolClosed
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		generation.metrics.SubmittedTasks.Add(1)
		w.taskCh <- acquireTaskFast(taskFn)
		return nil
	}
	p.lock.Unlock()

	generation.blockingCount.Add(1)
	generation.metrics.BlockingTasks.Add(1)
	defer func() {
		generation.blockingCount.Add(-1)
		generation.metrics.BlockingTasks.Add(-1)
	}()

	p.lock.Lock()
	for {
		select {
		case <-ctx.Done():
			p.lock.Unlock()
			return ctx.Err()
		default:
		}

		if !p.generationRunningLocked(generation) {
			p.lock.Unlock()
			return ErrPoolClosed
		}

		if w := p.retrieveWorkerLocked(generation); w != nil {
			p.lock.Unlock()
			generation.metrics.SubmittedTasks.Add(1)
			t := acquireTaskFast(taskFn)
			w.taskCh <- t
			return nil
		}

		p.waiters.waitLocked(&p.lock, ctx.Done())
	}
}

// Running returns the number of running workers
func (p *Pool) Running() int32 {
	generation := p.generation.Load()
	if generation == nil {
		return 0
	}
	return generation.workerCount.Load()
}

// Free returns the number of available worker slots
func (p *Pool) Free() int32 {
	return p.maxWorkers.Load() - p.Running()
}

// Waiting returns the number of waiting tasks
func (p *Pool) Waiting() int32 {
	generation := p.generation.Load()
	if generation == nil {
		return 0
	}
	return generation.metrics.BlockingTasks.Load()
}

// Idle returns the number of idle workers
func (p *Pool) Idle() int32 {
	generation := p.generation.Load()
	if generation == nil {
		return 0
	}
	return int32(generation.workers.size())
}

// Cap returns the pool capacity
func (p *Pool) Cap() int32 {
	return p.maxWorkers.Load()
}

// Name returns the pool name
func (p *Pool) Name() string {
	return p.name
}

// IsClosed returns true if the pool is closed
func (p *Pool) IsClosed() bool {
	return p.state.Load() == stateClosed
}

// Metrics returns performance metrics
func (p *Pool) Metrics() MetricsSnapshot {
	return p.generation.Load().metrics.Snapshot()
}

// ResetMetrics resets all metrics
func (p *Pool) ResetMetrics() {
	p.generation.Load().metrics.Reset()
}

// Uptime returns the running time
func (p *Pool) Uptime() time.Duration {
	return time.Since(p.generation.Load().createdAt)
}

// OnHook registers a hook callback
func (p *Pool) OnHook(hookType HookType, fn HookFunc) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.hooks == nil {
		p.hooks = NewHooks()
	}
	p.hooks.Register(hookType, fn)
}

// Tune dynamically adjusts pool capacity
func (p *Pool) Tune(newCap int32) {
	if newCap <= 0 {
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	generation := p.generation.Load()
	if !p.generationRunningLocked(generation) {
		return
	}

	p.maxWorkers.Store(newCap)
	p.waiters.broadcastLocked()

	excess := generation.workerCount.Load() - newCap
	for excess > 0 {
		if w := generation.workers.pop(); w != nil {
			generation.metrics.IdleWorkers.Add(-1)
			w.finish()
			excess--
		} else {
			break
		}
	}
	generation.workers.resize(int(newCap))
}

// closeCurrentGeneration 封口当前代际，并返回其唯一的退出通知。
func (p *Pool) closeCurrentGeneration() (generation *poolGeneration, done <-chan struct{}) {
	p.lock.Lock()
	defer p.lock.Unlock()
	generation = p.generation.Load()
	if generation == nil || generation.state.Load() == stateClosed {
		return nil, nil
	}

	generation.state.Store(stateClosed)
	p.state.Store(stateClosed)
	p.waiters.broadcastLocked()
	generation.stopCleaner()
	for {
		worker := generation.workers.pop()
		if worker == nil {
			break
		}
		generation.metrics.IdleWorkers.Add(-1)
		worker.finish()
	}
	done = generation.retire()
	return generation, done
}

func (p *Pool) unregisterClosedGeneration(generation *poolGeneration) {
	if p.name == "" {
		return
	}
	p.lock.Lock()
	if p.generation.Load() == generation && p.state.Load() == stateClosed {
		namedPools.Delete(p.name)
	}
	p.lock.Unlock()
}

// Release 释放池，并等待当前代际的所有任务结束。
func (p *Pool) Release() {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()

	generation, done := p.closeCurrentGeneration()
	if generation == nil {
		return
	}

	// 停止自动扩缩容器。
	if p.scaler != nil {
		p.scaler.Stop()
	}

	<-done
	p.unregisterClosedGeneration(generation)
}

// ReleaseTimeout releases with timeout
func (p *Pool) ReleaseTimeout(timeout time.Duration) error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	deadline := time.Now().Add(timeout)
	generation, done := p.closeCurrentGeneration()
	if generation == nil {
		return nil
	}

	if p.scaler != nil {
		p.scaler.Stop()
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		select {
		case <-done:
			p.unregisterClosedGeneration(generation)
			return nil
		default:
		}
		return ErrTimeout
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-done:
		p.unregisterClosedGeneration(generation)
		return nil
	case <-timer.C:
		return ErrTimeout
	}
}

// Reboot restarts a closed pool
func (p *Pool) Reboot() {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()

	p.lock.Lock()
	if p.state.Load() != stateClosed {
		p.lock.Unlock()
		return
	}
	generation := newPoolGeneration(p.generationID.Add(1), p.config, p.maxWorkers.Load())
	p.generation.Store(generation)
	p.state.Store(stateRunning)
	p.preheatLocked(generation)
	p.lock.Unlock()

	// Restart cleaner
	go p.purgeStaleWorkers(generation)

	// Restart auto-scaler
	if p.config.EnableAutoScale && p.scaler != nil {
		p.scaler.Reset()
		p.scaler.Start()
	}

	// Re-register
	if p.name != "" {
		namedPools.Store(p.name, p)
	}
}

// Close is an alias for Release (backward compatibility)
func (p *Pool) Close() {
	p.Release()
}

// CloseNow is an alias for Release (backward compatibility)
func (p *Pool) CloseNow() {
	p.Release()
}

// ============================================================================
// Global Default Pool & Simple API
// ============================================================================
//
// 简单用法 (类似 ByteDance gopool):
//
//	pool.Go(func() { /* task */ })           // 异步执行
//	pool.GoCtx(ctx, func(ctx context.Context) { /* 协作式任务 */ }) // 带 Context
//	pool.TryGo(func() { /* task */ })        // 非阻塞
//	pool.GoWait(func() { /* task */ })       // 同步等待
//
// 创建自定义池:
//
//	p := pool.NewSimple(4)                   // 4 workers
//	p := pool.NewAuto(10, 100)               // 自动扩缩容
//
// ============================================================================

var (
	defaultPool   atomic.Pointer[Pool]
	defaultPoolMu sync.Mutex
)

func initDefaultPool() *Pool {
	if p := defaultPool.Load(); p != nil {
		return p
	}

	defaultPoolMu.Lock()
	defer defaultPoolMu.Unlock()
	if p := defaultPool.Load(); p != nil {
		return p
	}

	p := New("default")
	defaultPool.Store(p)
	return p
}

// Go executes a function asynchronously using the default pool.
// This is the simplest way to run a task in the pool.
func Go(fn func()) error {
	return initDefaultPool().Submit(fn)
}

// GoCtx 使用默认池提交接收 context 的协作式任务。
func GoCtx(ctx context.Context, fn func(context.Context)) error {
	return initDefaultPool().SubmitWithContext(ctx, fn)
}

// TryGo attempts to execute without blocking. Returns false if pool is busy.
func TryGo(fn func()) bool {
	return initDefaultPool().TrySubmit(fn)
}

// GoWait executes a function and waits for completion.
func GoWait(fn func()) error {
	return initDefaultPool().SubmitWait(fn)
}

// GoBatch submits multiple functions efficiently. Returns number of submitted tasks.
func GoBatch(fns []func()) (int, error) {
	return initDefaultPool().SubmitBatch(fns)
}

// Parallel executes multiple functions in parallel and waits for all to complete.
func Parallel(fns ...func()) error {
	if len(fns) == 0 {
		return nil
	}
	p := initDefaultPool()
	results := make(chan error, len(fns))
	submitted := 0
	var submitErr error
	for _, fn := range fns {
		f := fn
		if err := p.Submit(func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- newTaskPanicError(recovered)
					panic(recovered)
				}
				results <- nil
			}()
			f()
		}); err != nil {
			submitErr = err
			break
		}
		submitted++
	}

	errList := make([]error, 0, submitted+1)
	if submitErr != nil {
		errList = append(errList, submitErr)
	}
	for range submitted {
		if err := <-results; err != nil {
			errList = append(errList, err)
		}
	}
	return errors.Join(errList...)
}

// SetCap sets the default pool capacity
func SetCap(capacity int32) {
	initDefaultPool().Tune(capacity)
}

// SetPanicHandler sets the panic handler for the default pool
func SetPanicHandler(handler func(any)) {
	initDefaultPool().panicHandler.Store(newPanicHandlerSnapshot(handler))
}

// Running returns the number of running workers in the default pool
func Running() int32 {
	return initDefaultPool().Running()
}

// Free returns available worker slots in the default pool
func Free() int32 {
	return initDefaultPool().Free()
}

// Cap returns the capacity of the default pool
func Cap() int32 {
	return initDefaultPool().Cap()
}

// DefaultPool returns the default pool instance
func DefaultPool() *Pool {
	return initDefaultPool()
}

// SetDefaultPool 替换默认池，但不接管旧池的关闭责任。
func SetDefaultPool(p *Pool) error {
	if p == nil {
		return invalidArgumentError("default pool must not be nil")
	}

	defaultPoolMu.Lock()
	defaultPool.Store(p)
	defaultPoolMu.Unlock()
	return nil
}

// ============================================================================
// Named Pool Management
// ============================================================================

var namedPools sync.Map

// GetPool gets a named pool
func GetPool(name string) (*Pool, bool) {
	v, ok := namedPools.Load(name)
	if !ok {
		return nil, false
	}
	return mustPoolValue[*Pool](v), true
}

// MustGetPool gets a named pool (panics if not found)
func MustGetPool(name string) *Pool {
	p, ok := GetPool(name)
	if !ok {
		panic(fmt.Sprintf("pool not found: %s", name))
	}
	return p
}

// RegisterPool registers a named pool
func RegisterPool(name string, p *Pool) {
	p.name = name
	namedPools.Store(name, p)
}

// UnregisterPool unregisters a named pool
func UnregisterPool(name string) {
	namedPools.Delete(name)
}

// RangePool iterates over all named pools
func RangePool(fn func(name string, p *Pool) bool) {
	namedPools.Range(func(key, value any) bool {
		return fn(mustPoolValue[string](key), mustPoolValue[*Pool](value))
	})
}

// ============================================================================
// MultiPool Load Balancing (inspired by ants)
// ============================================================================

// LoadBalancingStrategy defines the load balancing strategy
type LoadBalancingStrategy int

const (
	// RoundRobin uses round-robin strategy
	RoundRobin LoadBalancingStrategy = iota
	// LeastTasks selects the pool with least tasks
	LeastTasks
)

// MultiPool manages multiple pools for load balancing
type MultiPool struct {
	pools    []*Pool
	index    atomic.Int64
	strategy LoadBalancingStrategy
}

// NewMultiPool 创建多池实例，配置无效时返回错误。
func NewMultiPool(size int, poolSize int32, strategy LoadBalancingStrategy, opts ...Option) (*MultiPool, error) {
	if size <= 0 {
		return nil, invalidConfigurationError("multi-pool size must be greater than zero")
	}

	pools := make([]*Pool, size)
	for i := range size {
		pools[i] = New(fmt.Sprintf("multipool-%d", i), append(opts, WithMaxWorkers(poolSize))...)
	}
	return &MultiPool{
		pools:    pools,
		strategy: strategy,
	}, nil
}

// Submit submits a task
func (mp *MultiPool) Submit(fn func()) error {
	return mp.next().Submit(fn)
}

// TrySubmit attempts to submit a task
func (mp *MultiPool) TrySubmit(fn func()) bool {
	return mp.next().TrySubmit(fn)
}

// next gets the next pool
func (mp *MultiPool) next() *Pool {
	switch mp.strategy {
	case LeastTasks:
		return mp.leastTasks()
	default:
		return mp.roundRobin()
	}
}

func (mp *MultiPool) roundRobin() *Pool {
	idx := mp.index.Add(1) - 1
	return mp.pools[idx%int64(len(mp.pools))]
}

func (mp *MultiPool) leastTasks() *Pool {
	leastPool := mp.pools[0]
	minTasks := leastPool.Running() + leastPool.Waiting()

	for _, p := range mp.pools[1:] {
		tasks := p.Running() + p.Waiting()
		if tasks < minTasks {
			leastPool = p
			minTasks = tasks
		}
	}
	return leastPool
}

// Running returns total running workers across all pools
func (mp *MultiPool) Running() int32 {
	var total int32
	for _, p := range mp.pools {
		total += p.Running()
	}
	return total
}

// Free returns total free slots across all pools
func (mp *MultiPool) Free() int32 {
	var total int32
	for _, p := range mp.pools {
		total += p.Free()
	}
	return total
}

// Release 释放全部池并等待各自当前代际收敛。
func (mp *MultiPool) Release() {
	for _, p := range mp.pools {
		p.Release()
	}
}

// Reboot reboots all pools
func (mp *MultiPool) Reboot() {
	for _, p := range mp.pools {
		p.Reboot()
	}
}

// ============================================================================
// Backward Compatible WorkerPool API
// ============================================================================

// WorkerPool provides backward compatibility
type WorkerPool struct {
	*Pool
}

// NewWorkerPool creates a compatible WorkerPool
func NewWorkerPool(maxWorkers int) *WorkerPool {
	return &WorkerPool{
		Pool: New("", WithMaxWorkers(int32(maxWorkers)), WithAutoScale(false)),
	}
}

// Submit submits a task
func (p *WorkerPool) Submit(task func()) bool {
	return p.Pool.Submit(task) == nil
}

// TrySubmit attempts to submit a task
func (p *WorkerPool) TrySubmit(task func()) bool {
	return p.Pool.TrySubmit(task)
}

// SubmitWait submits a task and waits
func (p *WorkerPool) SubmitWait(task func()) bool {
	return p.Pool.SubmitWait(task) == nil
}

// Running returns running workers
func (p *WorkerPool) Running() int {
	return int(p.Pool.Running())
}

// Waiting returns waiting tasks
func (p *WorkerPool) Waiting() int {
	return int(p.Pool.Waiting())
}

// ============================================================================
// Object Pool
// ============================================================================

// ObjectPool is a generic object pool
type ObjectPool[T any] struct {
	pool    sync.Pool
	factory func() T
	reset   func(*T)
}

// NewObjectPool 创建对象池，factory 配置无效时返回错误。
func NewObjectPool[T any](factory func() T, reset func(*T)) (*ObjectPool[T], error) {
	if factory == nil {
		return nil, invalidConfigurationError("object pool factory must not be nil")
	}

	initial, err := objectPoolFactoryValue(factory)
	if err != nil {
		return nil, err
	}

	p := &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any {
				return factory()
			},
		},
		factory: factory,
		reset:   reset,
	}
	p.pool.Put(initial)
	return p, nil
}

func objectPoolFactoryValue[T any](factory func() T) (T, error) {
	value := factory()
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return value, invalidConfigurationError("object pool factory must return a non-nil value")
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if reflected.IsNil() {
			return value, invalidConfigurationError("object pool factory must return a non-nil value")
		}
	}
	return value, nil
}

// Get gets an object
func (p *ObjectPool[T]) Get() T {
	return mustPoolValue[T](p.pool.Get())
}

// Put returns an object
func (p *ObjectPool[T]) Put(obj T) {
	if p.reset != nil {
		p.reset(&obj)
	}
	p.pool.Put(obj)
}

// ============================================================================
// Byte Slice Pool
// ============================================================================

// ByteSlicePool is a byte slice pool
type ByteSlicePool struct {
	pool sync.Pool
	size int
}

// NewByteSlicePool creates a byte slice pool
func NewByteSlicePool(size int) *ByteSlicePool {
	return &ByteSlicePool{
		pool: sync.Pool{
			New: func() any {
				buffer := make([]byte, size)
				return &buffer
			},
		},
		size: size,
	}
}

// Get gets a byte slice
func (p *ByteSlicePool) Get() []byte {
	buffer := mustPoolValue[*[]byte](p.pool.Get())
	return (*buffer)[:p.size]
}

// Put returns a byte slice
func (p *ByteSlicePool) Put(b []byte) {
	if cap(b) >= p.size {
		b = b[:p.size]
		p.pool.Put(&b)
	}
}

// ============================================================================
// Buffer Pool
// ============================================================================

// BufferPool is a buffer pool
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a buffer pool
func NewBufferPool(initialSize int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				buffer := make([]byte, 0, initialSize)
				return &buffer
			},
		},
	}
}

// Get gets a buffer
func (p *BufferPool) Get() []byte {
	buffer := mustPoolValue[*[]byte](p.pool.Get())
	return (*buffer)[:0]
}

// Put returns a buffer
func (p *BufferPool) Put(b []byte) {
	b = b[:0]
	p.pool.Put(&b)
}

// ============================================================================
// Parallel Executor
// ============================================================================

// ParallelExecutor executes tasks in parallel
type ParallelExecutor struct {
	maxConcurrency int
	sem            chan struct{}
}

// NewParallelExecutor creates a parallel executor
func NewParallelExecutor(maxConcurrency int) *ParallelExecutor {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	return &ParallelExecutor{
		maxConcurrency: maxConcurrency,
		sem:            make(chan struct{}, maxConcurrency),
	}
}

// Execute executes multiple tasks in parallel
func (e *ParallelExecutor) Execute(ctx context.Context, tasks ...func() error) []error {
	errs := make([]error, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		select {
		case <-ctx.Done():
			errs[i] = ctx.Err()
			continue
		case e.sem <- struct{}{}:
		}

		wg.Add(1)
		go func(idx int, t func() error) {
			defer func() {
				<-e.sem
				wg.Done()
			}()
			errs[idx] = t()
		}(i, task)
	}

	wg.Wait()
	return errs
}

// ExecuteAll executes all tasks and returns first error
func (e *ParallelExecutor) ExecuteAll(ctx context.Context, tasks ...func() error) error {
	errs := e.Execute(ctx, tasks...)
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// ============================================================================
// Parallel Map/ForEach
// ============================================================================

// Map applies a function to each item in parallel
func Map[T, R any](ctx context.Context, items []T, maxConcurrency int, fn func(T) (R, error)) ([]R, error) {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	results := make([]R, len(items))
	errs := make([]error, len(items))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(idx int, it T) {
			defer func() {
				<-sem
				wg.Done()
			}()
			results[idx], errs[idx] = fn(it)
		}(i, item)
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// ForEach applies a function to each item in parallel
func ForEach[T any](ctx context.Context, items []T, maxConcurrency int, fn func(T) error) error {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	if len(items) == 0 {
		return nil
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	for _, item := range items {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case err := <-errChan:
			wg.Wait()
			return err
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(it T) {
			defer func() {
				<-sem
				wg.Done()
			}()
			if err := fn(it); err != nil {
				select {
				case errChan <- err:
				default:
				}
			}
		}(item)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Check for any errors
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}
