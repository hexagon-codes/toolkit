package poolx

import (
	"context"
	"fmt"
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

func newWorkerStack(cap int) *workerStack {
	return &workerStack{
		items: make([]*worker, cap),
		cap:   cap,
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

func (s *workerStack) retrieveExpiry(duration time.Duration) []*worker {
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
		if w != nil && now.Sub(time.Unix(0, w.lastActive.Load())) > duration {
			s.expiry = append(s.expiry, w)
			s.items[idx] = nil
		}
	}

	// Compact the array
	newItems := make([]*worker, s.cap)
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
// Task Definition
// ============================================================================

// task wraps a function to be executed
type task struct {
	fn        func()
	submitted int64 // UnixNano timestamp (lazy init, 0 = not set)
	priority  int
	timeout   time.Duration
	id        uint64
}

var taskPool = sync.Pool{
	New: func() any {
		return &task{}
	},
}

// acquireTaskFast is the fast path for simple task submission (no options, no hooks)
func acquireTaskFast(fn func()) *task {
	t := taskPool.Get().(*task)
	t.fn = fn
	t.submitted = 0 // Lazy init - only set when needed
	t.priority = 0
	t.timeout = 0
	t.id = 0
	return t
}

func acquireTaskWithOptions(fn func(), opts *TaskOptions) *task {
	t := taskPool.Get().(*task)
	t.fn = fn
	t.submitted = time.Now().UnixNano()
	if opts != nil {
		t.priority = opts.Priority
		t.timeout = opts.Timeout
		t.id = opts.ID
	} else {
		t.priority = PriorityNormal
		t.timeout = 0
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
	t.timeout = 0
	t.id = 0
	taskPool.Put(t)
}

// ============================================================================
// Worker Definition
// ============================================================================

// worker represents a worker goroutine
type worker struct {
	pool       *Pool
	taskCh     chan *task
	localQueue *WorkStealingDeque[task] // Local queue for work stealing
	lastActive atomic.Int64
	id         int32
}

func (w *worker) run() {
	w.pool.wg.Add(1)
	go w.loop()
}

func (w *worker) loop() {
	defer func() {
		// Unregister from work stealing scheduler
		if w.pool.config.EnableWorkStealing && w.pool.stealingScheduler != nil && w.localQueue != nil {
			w.pool.stealingScheduler.Unregister(w.id)
		}

		w.pool.workerCount.Add(-1)
		w.pool.metrics.RunningWorkers.Add(-1)

		// Trigger worker stop hook before wg.Done to ensure
		// hooks complete before Release returns
		if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookOnWorkerStop) {
			w.pool.hooks.Trigger(HookOnWorkerStop, &WorkerInfo{
				ID:        w.id,
				PoolName:  w.pool.name,
				StoppedAt: time.Now(),
			})
		}

		// Put back to cache before wg.Done to avoid race with Reboot
		w.pool.workerCache.Put(w)
		w.pool.wg.Done()
	}()

	// Register with work stealing scheduler if enabled
	if w.pool.config.EnableWorkStealing && w.pool.stealingScheduler != nil && w.localQueue != nil {
		w.pool.stealingScheduler.Register(w.id, w.localQueue)
	}

	// Trigger worker start hook
	if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookOnWorkerStart) {
		w.pool.hooks.Trigger(HookOnWorkerStart, &WorkerInfo{
			ID:        w.id,
			PoolName:  w.pool.name,
			StartedAt: time.Now(),
		})
	}

	for t := range w.taskCh {
		if t == nil {
			return
		}

		w.execute(t)

		// Try to process tasks from local queue (work stealing)
		if w.pool.config.EnableWorkStealing && w.localQueue != nil {
			w.processLocalQueue()
		}

		w.lastActive.Store(time.Now().UnixNano())

		// Return self to idle stack
		if !w.pool.revertWorker(w) {
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
	if w.pool.stealingScheduler != nil {
		stolen := w.pool.stealingScheduler.Steal(w.id)
		if stolen != nil {
			w.pool.metrics.StolenTasks.Add(1)
			w.execute(stolen)
		}
	}
}

func (w *worker) execute(t *task) {
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
			Timeout:     t.timeout,
		}
	}

	// Trigger before task hook
	if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookBeforeTask) {
		w.pool.hooks.Trigger(HookBeforeTask, taskInfo)
	}

	// Execute with timeout if specified
	var panicked bool
	var panicVal any

	if t.timeout > 0 {
		panicked, panicVal = w.executeWithTimeout(t)
	} else {
		panicked, panicVal = w.executeDirect(t)
	}

	execTime := time.Since(startTime)

	// Update metrics
	w.pool.metrics.TotalWaitTime.Add(int64(waitTime))
	w.pool.metrics.TotalExecTime.Add(int64(execTime))

	if panicked {
		w.pool.metrics.FailedTasks.Add(1)
		if w.pool.config.PanicHandler != nil {
			// 包装 panic handler 调用，防止它本身 panic 导致 goroutine 崩溃
			func() {
				defer func() {
					if r := recover(); r != nil {
						// PanicHandler 本身 panic 了，静默处理
					}
				}()
				w.pool.config.PanicHandler(panicVal)
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
		w.pool.metrics.CompletedTasks.Add(1)
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

// executeWithTimeout 执行带超时的任务。
// 注意：如果超时触发，任务 goroutine 会在后台继续运行，但结果会被忽略。
// 这是 Go 的基本限制 - goroutine 无法被强制终止。
// 如果任务需要提前停止，应在任务函数中检查取消信号。
// 建议使用 SubmitWithContext 来支持可取消的任务。
func (w *worker) executeWithTimeout(t *task) (panicked bool, panicVal any) {
	type result struct {
		panicked bool
		panicVal any
	}
	resultCh := make(chan result, 1)

	// Copy the function to avoid race when task is released
	fn := t.fn

	go func() {
		var r result
		defer func() {
			if rec := recover(); rec != nil {
				r.panicked = true
				r.panicVal = rec
			}
			// Use select to avoid blocking if nobody is listening
			select {
			case resultCh <- r:
			default:
			}
		}()
		fn()
	}()

	select {
	case res := <-resultCh:
		return res.panicked, res.panicVal
	case <-time.After(t.timeout):
		// Trigger timeout hook
		if w.pool.hooks != nil && w.pool.hooks.HasHooks(HookOnTimeout) {
			w.pool.hooks.Trigger(HookOnTimeout, &TaskInfo{
				ID:          t.id,
				PoolName:    w.pool.name,
				WorkerID:    w.id,
				SubmittedAt: t.getSubmittedTime(),
				Timeout:     t.timeout,
			})
		}
		return false, nil
	}
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

	// Priority queue (optional)
	priorityQueue *PriorityQueue

	// Worker management
	workers     *workerStack
	workerCount atomic.Int32
	workerID    atomic.Int32
	workerCache sync.Pool

	// Work stealing scheduler
	stealingScheduler *StealingScheduler

	// Auto-scaler
	scaler *AutoScaler

	// Blocking control
	blockingCount atomic.Int32
	cond          *sync.Cond

	// State
	state     atomic.Int32 // 0: running, 1: closed
	heartbeat chan struct{}
	wg        sync.WaitGroup

	// Metrics
	metrics *Metrics

	// Hooks
	hooks *Hooks

	// Task ID generator
	taskIDGen atomic.Uint64

	// Creation time
	createdAt time.Time

	// Dynamic capacity (atomic for concurrent access)
	maxWorkers atomic.Int32

	lock sync.Mutex
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
		workers:   newWorkerStack(int(config.MaxWorkers)),
		heartbeat: make(chan struct{}),
		metrics:   &Metrics{},
		hooks:     config.Hooks,
		createdAt: time.Now(),
	}

	// Initialize atomic maxWorkers for concurrent access
	p.maxWorkers.Store(config.MaxWorkers)

	p.cond = sync.NewCond(&p.lock)

	p.workerCache.New = func() any {
		w := &worker{
			pool:   p,
			taskCh: make(chan *task, 4), // Increased buffer for better throughput
		}
		// Pre-allocate local queue for work stealing
		if config.EnableWorkStealing {
			w.localQueue = NewWorkStealingDeque[task](64)
		}
		return w
	}

	// Initialize priority queue if enabled
	if config.EnablePriorityQueue {
		p.priorityQueue = NewPriorityQueue(int(config.QueueSize))
	}

	// Initialize work stealing scheduler if enabled
	if config.EnableWorkStealing {
		p.stealingScheduler = NewStealingScheduler()
	}

	// 预分配 worker 和 task 对象缓存，减少 GC 压力。
	// 注意：这只是预热 sync.Pool 缓存，不会启动实际的 worker。
	// 使用 WithMinWorkers 来预启动实际的 worker。
	if config.PreAlloc {
		p.workers = newWorkerStack(int(config.MaxWorkers))
		// 预分配 worker 对象到缓存
		for range config.MaxWorkers {
			w := p.workerCache.Get().(*worker)
			p.workerCache.Put(w)
		}
		// 预分配 task 对象到缓存
		for range config.QueueSize {
			t := taskPool.Get().(*task)
			taskPool.Put(t)
		}
	}

	// Preheat workers
	p.preheat()

	// Start cleaner goroutine
	go p.purgeStaleWorkers()

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

// preheat creates the minimum workers
func (p *Pool) preheat() {
	for i := int32(0); i < p.config.MinWorkers; i++ {
		w := p.createWorker()
		if w == nil {
			break
		}
		w.run()
		p.workers.push(w)
		p.metrics.IdleWorkers.Add(1)
	}
}

// createWorker creates a new worker
func (p *Pool) createWorker() *worker {
	if p.workerCount.Load() >= p.config.MaxWorkers {
		return nil
	}

	// Atomic increment
	for {
		current := p.workerCount.Load()
		if current >= p.config.MaxWorkers {
			return nil
		}
		if p.workerCount.CompareAndSwap(current, current+1) {
			break
		}
	}

	id := p.workerID.Add(1)

	// Get from cache or create new
	w := p.workerCache.Get().(*worker)
	w.pool = p
	w.id = id
	w.lastActive.Store(time.Now().UnixNano())

	// Ensure taskCh is available (increased buffer for better throughput)
	if w.taskCh == nil {
		w.taskCh = make(chan *task, 4)
	}

	// Initialize local queue for work stealing if enabled
	if p.config.EnableWorkStealing && w.localQueue == nil {
		w.localQueue = NewWorkStealingDeque[task](64)
	}

	p.metrics.RunningWorkers.Add(1)

	// Update peak
	for {
		peak := p.metrics.PeakWorkers.Load()
		current := p.metrics.RunningWorkers.Load()
		if current <= peak {
			break
		}
		if p.metrics.PeakWorkers.CompareAndSwap(peak, current) {
			break
		}
	}

	return w
}

// createWorkerInternal is used by the auto-scaler
func (p *Pool) createWorkerInternal() *worker {
	return p.createWorker()
}

// retrieveWorker gets an available worker
func (p *Pool) retrieveWorker() *worker {
	// Try idle stack first
	if w := p.workers.pop(); w != nil {
		p.metrics.IdleWorkers.Add(-1)
		return w
	}

	// Try to create new worker
	if w := p.createWorker(); w != nil {
		w.run()
		return w
	}

	return nil
}

// revertWorker returns a worker to idle
func (p *Pool) revertWorker(w *worker) bool {
	w.lastActive.Store(time.Now().UnixNano())

	p.lock.Lock()
	defer p.lock.Unlock()

	// Check if pool is closed under lock to avoid race with Release
	if p.state.Load() == stateClosed {
		return false
	}

	// Try to push to idle stack
	if p.workers.push(w) {
		p.metrics.IdleWorkers.Add(1)
		// Wake up waiting submitters
		p.cond.Signal()
		return true
	}

	return false
}

// purgeStaleWorkers periodically cleans up expired workers
func (p *Pool) purgeStaleWorkers() {
	ticker := time.NewTicker(p.config.WorkerExpiry)
	defer ticker.Stop()

	// Capture heartbeat channel under lock to avoid race with Reboot
	p.lock.Lock()
	heartbeat := p.heartbeat
	p.lock.Unlock()

	for {
		select {
		case <-ticker.C:
			if p.state.Load() == stateClosed {
				return
			}
			p.cleanupExpiredWorkers()
		case <-heartbeat:
			return
		}
	}
}

func (p *Pool) cleanupExpiredWorkers() {
	// Get expired workers
	expired := p.workers.retrieveExpiry(p.config.WorkerExpiry)

	// Decrease idle count
	p.metrics.IdleWorkers.Add(-int32(len(expired)))

	// Close expired workers (keep minimum)
	minToKeep := p.config.MinWorkers
	currentRunning := p.workerCount.Load()

	for _, w := range expired {
		if currentRunning <= minToKeep {
			// Put back
			if p.workers.push(w) {
				p.metrics.IdleWorkers.Add(1)
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

// submitFast is the optimized submit path for common case (no hooks, no special options)
func (p *Pool) submitFast(fn func()) error {
	if p.state.Load() == stateClosed {
		return ErrPoolClosed
	}

	t := acquireTaskFast(fn)
	p.metrics.SubmittedTasks.Add(1)

	// Try to get a worker
	if w := p.retrieveWorker(); w != nil {
		w.taskCh <- t
		return nil
	}

	// Block and wait (no blocking limit check in fast path)
	p.blockingCount.Add(1)
	p.metrics.BlockingTasks.Add(1)
	defer func() {
		p.blockingCount.Add(-1)
		p.metrics.BlockingTasks.Add(-1)
	}()

	p.lock.Lock()
	for {
		if p.state.Load() == stateClosed {
			p.lock.Unlock()
			releaseTask(t)
			return ErrPoolClosed
		}

		if w := p.workers.pop(); w != nil {
			p.metrics.IdleWorkers.Add(-1)
			p.lock.Unlock()
			w.taskCh <- t
			return nil
		}

		// Try to create new worker
		if w := p.createWorker(); w != nil {
			p.lock.Unlock()
			w.run()
			w.taskCh <- t
			return nil
		}

		// Wait
		p.cond.Wait()
	}
}

// SubmitWithOptions submits a task with optional settings
func (p *Pool) SubmitWithOptions(fn func(), opts ...TaskOption) error {
	if p.state.Load() == stateClosed {
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
	p.metrics.SubmittedTasks.Add(1)

	// Trigger before submit hook
	if p.hooks != nil && p.hooks.HasHooks(HookBeforeSubmit) {
		p.hooks.Trigger(HookBeforeSubmit, &TaskInfo{
			ID:          t.id,
			PoolName:    p.name,
			Priority:    t.priority,
			SubmittedAt: t.getSubmittedTime(),
			Timeout:     t.timeout,
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

	// Try to get a worker
	if w := p.retrieveWorker(); w != nil {
		w.taskCh <- t

		// Trigger after submit hook
		if taskInfo != nil {
			p.hooks.Trigger(HookAfterSubmit, taskInfo)
		}
		return nil
	}

	// Non-blocking mode
	if p.config.NonBlocking {
		releaseTask(t)
		p.metrics.RejectedTasks.Add(1)

		// Trigger reject hook
		if p.hooks != nil && p.hooks.HasHooks(HookOnReject) {
			p.hooks.Trigger(HookOnReject, &TaskInfo{
				ID:       t.id,
				PoolName: p.name,
				Priority: t.priority,
			})
		}
		return ErrPoolOverload
	}

	// Check blocking limit
	if p.config.MaxBlockingTasks > 0 {
		if p.blockingCount.Load() >= p.config.MaxBlockingTasks {
			releaseTask(t)
			p.metrics.RejectedTasks.Add(1)

			if p.hooks != nil && p.hooks.HasHooks(HookOnReject) {
				p.hooks.Trigger(HookOnReject, &TaskInfo{
					ID:       t.id,
					PoolName: p.name,
					Priority: t.priority,
				})
			}
			return ErrPoolOverload
		}
	}

	// Block and wait
	p.blockingCount.Add(1)
	p.metrics.BlockingTasks.Add(1)
	defer func() {
		p.blockingCount.Add(-1)
		p.metrics.BlockingTasks.Add(-1)
	}()

	p.lock.Lock()
	for {
		if p.state.Load() == stateClosed {
			p.lock.Unlock()
			releaseTask(t)
			return ErrPoolClosed
		}

		if w := p.workers.pop(); w != nil {
			p.metrics.IdleWorkers.Add(-1)
			p.lock.Unlock()
			w.taskCh <- t

			// Trigger after submit hook (use captured taskInfo)
			if taskInfo != nil {
				p.hooks.Trigger(HookAfterSubmit, taskInfo)
			}
			return nil
		}

		// Try to create new worker
		if w := p.createWorker(); w != nil {
			p.lock.Unlock()
			w.run()
			w.taskCh <- t

			// Trigger after submit hook (use captured taskInfo)
			if taskInfo != nil {
				p.hooks.Trigger(HookAfterSubmit, taskInfo)
			}
			return nil
		}

		// Wait
		p.cond.Wait()
	}
}

// TrySubmit attempts to submit a task (non-blocking)
func (p *Pool) TrySubmit(fn func()) bool {
	if p.state.Load() == stateClosed {
		return false
	}

	// Try to get a worker
	if w := p.retrieveWorker(); w != nil {
		t := acquireTaskFast(fn) // Use fast path - no timestamp needed
		p.metrics.SubmittedTasks.Add(1)
		w.taskCh <- t
		return true
	}

	p.metrics.RejectedTasks.Add(1)
	return false
}

// SubmitBatch submits multiple tasks at once, reducing lock overhead.
// Returns the number of successfully submitted tasks and an error if pool is closed.
func (p *Pool) SubmitBatch(fns []func()) (int, error) {
	n := len(fns)
	if n == 0 {
		return 0, nil
	}

	if p.state.Load() == stateClosed {
		return 0, ErrPoolClosed
	}

	submitted := 0

	// Fast path: try to submit directly to available workers
	for i := 0; i < n; i++ {
		if w := p.retrieveWorker(); w != nil {
			t := acquireTaskFast(fns[i])
			w.taskCh <- t
			submitted++
		} else {
			// No more available workers, switch to blocking mode for remaining
			remaining := n - i
			p.blockingCount.Add(int32(remaining))
			p.metrics.BlockingTasks.Add(int32(remaining))

			// Submit remaining tasks in blocking mode
			for j := i; j < n; j++ {
				if err := p.submitBlockingFast(fns[j]); err != nil {
					p.blockingCount.Add(-int32(n - j))
					p.metrics.BlockingTasks.Add(-int32(n - j))
					p.metrics.SubmittedTasks.Add(int64(submitted))
					return submitted, err
				}
				submitted++
			}

			p.blockingCount.Add(-int32(remaining))
			p.metrics.BlockingTasks.Add(-int32(remaining))
			p.metrics.SubmittedTasks.Add(int64(submitted))
			return submitted, nil
		}
	}

	// All submitted via fast path
	p.metrics.SubmittedTasks.Add(int64(submitted))
	return submitted, nil
}

// submitBlockingFast is optimized blocking submit without metrics overhead
func (p *Pool) submitBlockingFast(fn func()) error {
	t := acquireTaskFast(fn)

	p.lock.Lock()
	for {
		if p.state.Load() == stateClosed {
			p.lock.Unlock()
			releaseTask(t)
			return ErrPoolClosed
		}

		if w := p.workers.pop(); w != nil {
			p.metrics.IdleWorkers.Add(-1)
			p.lock.Unlock()
			w.taskCh <- t
			return nil
		}

		// Try to create new worker
		if w := p.createWorker(); w != nil {
			p.lock.Unlock()
			w.run()
			w.taskCh <- t
			return nil
		}

		// Wait
		p.cond.Wait()
	}
}

// TrySubmitBatch attempts to submit multiple tasks without blocking.
// Returns the number of successfully submitted tasks.
func (p *Pool) TrySubmitBatch(fns []func()) int {
	n := len(fns)
	if n == 0 || p.state.Load() == stateClosed {
		return 0
	}

	submitted := 0
	for _, fn := range fns {
		if w := p.retrieveWorker(); w != nil {
			t := acquireTaskFast(fn)
			w.taskCh <- t
			submitted++
		} else {
			break
		}
	}

	// Batch update metrics
	if submitted > 0 {
		p.metrics.SubmittedTasks.Add(int64(submitted))
	}
	if submitted < n {
		p.metrics.RejectedTasks.Add(int64(n - submitted))
	}
	return submitted
}

// SubmitWait submits a task and waits for completion
func (p *Pool) SubmitWait(fn func()) error {
	done := make(chan struct{})
	err := p.Submit(func() {
		defer close(done)
		fn()
	})
	if err != nil {
		return err
	}
	<-done
	return nil
}

// SubmitWithContext submits a task with context support
// 注意：如果 context 被取消，任务可能已经被提交到队列中。
// 建议在任务函数内部检查 context 状态以支持取消。
func (p *Pool) SubmitWithContext(ctx context.Context, fn func()) error {
	if p.state.Load() == stateClosed {
		return ErrPoolClosed
	}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Try non-blocking first
	if p.TrySubmit(fn) {
		return nil
	}

	// Block with context cancellation
	// 使用带缓冲的 channel 避免 goroutine 泄漏
	done := make(chan error, 1)
	go func() {
		err := p.Submit(fn)
		// 使用 select 避免阻塞，即使没人接收
		select {
		case done <- err:
		default:
			// context 已取消，没人等待结果
			// 任务可能已提交，这是预期行为
		}
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// 注意：此时后台 goroutine 可能仍在等待 Submit
		// 但由于 done channel 有缓冲，goroutine 最终会退出
		return ctx.Err()
	}
}

// Running returns the number of running workers
func (p *Pool) Running() int32 {
	return p.workerCount.Load()
}

// Free returns the number of available worker slots
func (p *Pool) Free() int32 {
	return p.config.MaxWorkers - p.workerCount.Load()
}

// Waiting returns the number of waiting tasks
func (p *Pool) Waiting() int32 {
	return p.metrics.BlockingTasks.Load()
}

// Idle returns the number of idle workers
func (p *Pool) Idle() int32 {
	return int32(p.workers.size())
}

// Cap returns the pool capacity
func (p *Pool) Cap() int32 {
	return p.config.MaxWorkers
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
	return p.metrics.Snapshot()
}

// ResetMetrics resets all metrics
func (p *Pool) ResetMetrics() {
	p.metrics.Reset()
}

// Uptime returns the running time
func (p *Pool) Uptime() time.Duration {
	return time.Since(p.createdAt)
}

// OnHook registers a hook callback
func (p *Pool) OnHook(hookType HookType, fn HookFunc) {
	if p.hooks == nil {
		p.hooks = NewHooks()
	}
	p.hooks.Register(hookType, fn)
}

// Tune dynamically adjusts pool capacity
func (p *Pool) Tune(newCap int32) {
	if newCap <= 0 || p.state.Load() == stateClosed {
		return
	}

	// Update atomic value first (for concurrent readers like AutoScaler)
	p.maxWorkers.Store(newCap)

	p.lock.Lock()
	defer p.lock.Unlock()

	p.config.MaxWorkers = newCap

	// Shrink if needed
	for p.workerCount.Load() > newCap {
		if w := p.workers.pop(); w != nil {
			p.metrics.IdleWorkers.Add(-1)
			w.finish()
		} else {
			break
		}
	}
}

// Release releases the pool (waits for all tasks to complete)
func (p *Pool) Release() {
	if !p.state.CompareAndSwap(stateRunning, stateClosed) {
		return
	}

	// Stop auto-scaler
	if p.scaler != nil {
		p.scaler.Stop()
	}

	p.lock.Lock()
	// Wake up all waiting submitters
	p.cond.Broadcast()
	p.lock.Unlock()

	// Stop cleaner goroutine
	close(p.heartbeat)

	// Close all idle workers
	// Need to keep trying because workers that were busy might
	// finish and push themselves back to the stack after we pop.
	// Once all workers have exited (workerCount == 0), we're done.
	for p.workerCount.Load() > 0 {
		for {
			if w := p.workers.pop(); w != nil {
				w.finish()
			} else {
				break
			}
		}
		// Small sleep to reduce CPU spinning while waiting for workers
		time.Sleep(time.Millisecond)
	}

	// Wait for all workers to complete their cleanup
	p.wg.Wait()

	// Remove from named pools
	if p.name != "" {
		namedPools.Delete(p.name)
	}
}

// ReleaseTimeout releases with timeout
func (p *Pool) ReleaseTimeout(timeout time.Duration) error {
	if !p.state.CompareAndSwap(stateRunning, stateClosed) {
		return nil
	}

	if p.scaler != nil {
		p.scaler.Stop()
	}

	p.lock.Lock()
	p.cond.Broadcast()
	p.lock.Unlock()

	close(p.heartbeat)

	// Close all idle workers with timeout
	deadline := time.Now().Add(timeout)
	for p.workerCount.Load() > 0 && time.Now().Before(deadline) {
		for {
			if w := p.workers.pop(); w != nil {
				w.finish()
			} else {
				break
			}
		}
		time.Sleep(time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return ErrTimeout
	}

	select {
	case <-done:
		if p.name != "" {
			namedPools.Delete(p.name)
		}
		return nil
	case <-time.After(remaining):
		return ErrTimeout
	}
}

// Reboot restarts a closed pool
func (p *Pool) Reboot() {
	if !p.state.CompareAndSwap(stateClosed, stateRunning) {
		return
	}

	p.lock.Lock()
	p.heartbeat = make(chan struct{})
	p.workers = newWorkerStack(int(p.config.MaxWorkers))
	p.metrics.Reset()
	p.createdAt = time.Now()

	// Reinitialize priority queue if enabled
	if p.config.EnablePriorityQueue {
		p.priorityQueue = NewPriorityQueue(int(p.config.QueueSize))
	}

	// Reinitialize work stealing scheduler if enabled
	if p.config.EnableWorkStealing {
		p.stealingScheduler = NewStealingScheduler()
	}
	p.lock.Unlock()

	// Preheat
	p.preheat()

	// Restart cleaner
	go p.purgeStaleWorkers()

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
//	pool.GoCtx(ctx, func() { /* task */ })   // 带 Context
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
	defaultPool *Pool
	defaultOnce sync.Once
)

func initDefaultPool() {
	defaultOnce.Do(func() {
		defaultPool = New("default")
	})
}

// Go executes a function asynchronously using the default pool.
// This is the simplest way to run a task in the pool.
func Go(fn func()) {
	initDefaultPool()
	_ = defaultPool.Submit(fn)
}

// GoCtx executes a function with context support.
// Returns error if context is cancelled before submission.
func GoCtx(ctx context.Context, fn func()) error {
	initDefaultPool()
	return defaultPool.SubmitWithContext(ctx, fn)
}

// TryGo attempts to execute without blocking. Returns false if pool is busy.
func TryGo(fn func()) bool {
	initDefaultPool()
	return defaultPool.TrySubmit(fn)
}

// GoWait executes a function and waits for completion.
func GoWait(fn func()) {
	initDefaultPool()
	_ = defaultPool.SubmitWait(fn)
}

// GoBatch submits multiple functions efficiently. Returns number of submitted tasks.
func GoBatch(fns []func()) int {
	initDefaultPool()
	n, _ := defaultPool.SubmitBatch(fns)
	return n
}

// Parallel executes multiple functions in parallel and waits for all to complete.
func Parallel(fns ...func()) {
	if len(fns) == 0 {
		return
	}
	initDefaultPool()
	done := make(chan struct{}, len(fns))
	for _, fn := range fns {
		f := fn
		_ = defaultPool.Submit(func() {
			f()
			done <- struct{}{}
		})
	}
	for range fns {
		<-done
	}
}

// SetCap sets the default pool capacity
func SetCap(cap int32) {
	initDefaultPool()
	defaultPool.Tune(cap)
}

// SetPanicHandler sets the panic handler for the default pool
func SetPanicHandler(handler func(any)) {
	initDefaultPool()
	defaultPool.config.PanicHandler = handler
}

// Running returns the number of running workers in the default pool
func Running() int32 {
	initDefaultPool()
	return defaultPool.Running()
}

// Free returns available worker slots in the default pool
func Free() int32 {
	initDefaultPool()
	return defaultPool.Free()
}

// Cap returns the capacity of the default pool
func Cap() int32 {
	initDefaultPool()
	return defaultPool.Cap()
}

// DefaultPool returns the default pool instance
func DefaultPool() *Pool {
	initDefaultPool()
	return defaultPool
}

// SetDefaultPool replaces the default pool
func SetDefaultPool(p *Pool) {
	defaultPool = p
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
	return v.(*Pool), true
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
		return fn(key.(string), value.(*Pool))
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

// NewMultiPool creates a new multi-pool
func NewMultiPool(size int, poolSize int32, strategy LoadBalancingStrategy, opts ...Option) *MultiPool {
	pools := make([]*Pool, size)
	for i := range size {
		pools[i] = New(fmt.Sprintf("multipool-%d", i), append(opts, WithMaxWorkers(poolSize))...)
	}
	return &MultiPool{
		pools:    pools,
		strategy: strategy,
	}
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
	min := mp.pools[0]
	minTasks := min.Running() + min.Waiting()

	for _, p := range mp.pools[1:] {
		tasks := p.Running() + p.Waiting()
		if tasks < minTasks {
			min = p
			minTasks = tasks
		}
	}
	return min
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

// Release releases all pools
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

// NewObjectPool creates an object pool
func NewObjectPool[T any](factory func() T, reset func(*T)) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any {
				return factory()
			},
		},
		factory: factory,
		reset:   reset,
	}
}

// Get gets an object
func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
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
				return make([]byte, size)
			},
		},
		size: size,
	}
}

// Get gets a byte slice
func (p *ByteSlicePool) Get() []byte {
	return p.pool.Get().([]byte)[:p.size]
}

// Put returns a byte slice
func (p *ByteSlicePool) Put(b []byte) {
	if cap(b) >= p.size {
		p.pool.Put(b[:p.size])
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
				return make([]byte, 0, initialSize)
			},
		},
	}
}

// Get gets a buffer
func (p *BufferPool) Get() []byte {
	return p.pool.Get().([]byte)[:0]
}

// Put returns a buffer
func (p *BufferPool) Put(b []byte) {
	p.pool.Put(b[:0])
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
