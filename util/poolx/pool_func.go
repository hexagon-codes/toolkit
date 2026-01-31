package poolx

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// PoolWithFunc - Single-Function Pool (ants style)
// ============================================================================

// PoolWithFunc is a goroutine pool that executes a single function with different arguments.
// This is more memory-efficient than Pool when all tasks run the same function.
type PoolWithFunc struct {
	// Configuration
	config      Config
	name        string
	poolFunc    func(any) // The function to execute
	argChanSize int32

	// Worker management
	workers     *workerFuncStack
	workerCount atomic.Int32
	workerID    atomic.Int32
	workerCache sync.Pool

	// Argument channel
	argCh chan any

	// Blocking control
	blockingCount atomic.Int32
	cond          *sync.Cond

	// State
	state     atomic.Int32
	heartbeat chan struct{}
	wg        sync.WaitGroup

	// Metrics
	metrics *Metrics

	// Hooks
	hooks *Hooks

	// Auto-scaler
	scaler *AutoScaler

	// Creation time
	createdAt time.Time

	lock sync.Mutex
}

// NewPoolWithFunc creates a new pool with a single function
func NewPoolWithFunc(name string, poolFunc func(any), opts ...Option) *PoolWithFunc {
	if poolFunc == nil {
		panic("pool function cannot be nil")
	}

	config := DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	// Validate config
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 1
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

	p := &PoolWithFunc{
		config:      config,
		name:        name,
		poolFunc:    poolFunc,
		argChanSize: config.QueueSize,
		workers:     newWorkerFuncStack(int(config.MaxWorkers)),
		argCh:       make(chan any, config.QueueSize),
		heartbeat:   make(chan struct{}),
		metrics:     &Metrics{},
		createdAt:   time.Now(),
	}

	p.cond = sync.NewCond(&p.lock)

	p.workerCache.New = func() any {
		return &workerFunc{
			pool: p,
		}
	}

	// Preheat workers
	p.preheat()

	// Start cleaner
	go p.purgeStaleWorkers()

	// 注意：PoolWithFunc 目前不支持 AutoScaler。
	// 如需自动扩缩容，请使用 Pool + SubmitFunc。

	return p
}

// preheat creates the minimum number of workers
func (p *PoolWithFunc) preheat() {
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
func (p *PoolWithFunc) createWorker() *workerFunc {
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
	w := p.workerCache.Get().(*workerFunc)
	w.pool = p
	w.id = id
	w.lastActive.Store(time.Now().UnixNano())
	w.argCh = make(chan any, 1)

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

// retrieveWorker gets an available worker
func (p *PoolWithFunc) retrieveWorker() *workerFunc {
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

// revertWorker returns a worker to the idle stack
func (p *PoolWithFunc) revertWorker(w *workerFunc) bool {
	w.lastActive.Store(time.Now().UnixNano())

	p.lock.Lock()
	defer p.lock.Unlock()

	// Check state under lock to avoid race with Release
	if p.state.Load() == stateClosed {
		return false
	}

	if p.workers.push(w) {
		p.metrics.IdleWorkers.Add(1)
		p.cond.Signal()
		return true
	}

	return false
}

// purgeStaleWorkers removes expired workers
func (p *PoolWithFunc) purgeStaleWorkers() {
	ticker := time.NewTicker(p.config.WorkerExpiry)
	defer ticker.Stop()

	// Capture heartbeat channel under lock
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

func (p *PoolWithFunc) cleanupExpiredWorkers() {
	expired := p.workers.retrieveExpiry(p.config.WorkerExpiry)
	p.metrics.IdleWorkers.Add(-int32(len(expired)))

	minToKeep := p.config.MinWorkers
	currentRunning := p.workerCount.Load()

	for _, w := range expired {
		if currentRunning <= minToKeep {
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

// Invoke submits an argument to be processed by the pool function.
// Blocks if no workers are available.
func (p *PoolWithFunc) Invoke(arg any) error {
	if p.state.Load() == stateClosed {
		return ErrPoolClosed
	}

	p.metrics.SubmittedTasks.Add(1)

	// Try to get a worker
	if w := p.retrieveWorker(); w != nil {
		w.argCh <- arg
		return nil
	}

	// Non-blocking mode
	if p.config.NonBlocking {
		p.metrics.RejectedTasks.Add(1)
		return ErrPoolOverload
	}

	// Check blocking limit
	if p.config.MaxBlockingTasks > 0 {
		if p.blockingCount.Load() >= p.config.MaxBlockingTasks {
			p.metrics.RejectedTasks.Add(1)
			return ErrPoolOverload
		}
	}

	// Wait for worker
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
			return ErrPoolClosed
		}

		if w := p.workers.pop(); w != nil {
			p.metrics.IdleWorkers.Add(-1)
			p.lock.Unlock()
			w.argCh <- arg
			return nil
		}

		if w := p.createWorker(); w != nil {
			p.lock.Unlock()
			w.run()
			w.argCh <- arg
			return nil
		}

		p.cond.Wait()
	}
}

// TryInvoke attempts to submit an argument without blocking.
// Returns false if no worker is immediately available.
func (p *PoolWithFunc) TryInvoke(arg any) bool {
	if p.state.Load() == stateClosed {
		return false
	}

	if w := p.retrieveWorker(); w != nil {
		p.metrics.SubmittedTasks.Add(1)
		w.argCh <- arg
		return true
	}

	p.metrics.RejectedTasks.Add(1)
	return false
}

// InvokeWithTimeout submits an argument with a timeout for getting a worker.
func (p *PoolWithFunc) InvokeWithTimeout(arg any, timeout time.Duration) error {
	if p.state.Load() == stateClosed {
		return ErrPoolClosed
	}

	// Try non-blocking first
	if w := p.retrieveWorker(); w != nil {
		p.metrics.SubmittedTasks.Add(1)
		w.argCh <- arg
		return nil
	}

	// Use timeout
	done := make(chan error, 1)
	go func() {
		done <- p.Invoke(arg)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return ErrTimeout
	}
}

// InvokeWithContext submits an argument with context cancellation support.
func (p *PoolWithFunc) InvokeWithContext(ctx context.Context, arg any) error {
	if p.state.Load() == stateClosed {
		return ErrPoolClosed
	}

	// Try non-blocking first
	if p.TryInvoke(arg) {
		return nil
	}

	// Wait with context
	done := make(chan error, 1)
	go func() {
		done <- p.Invoke(arg)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Running returns the number of running workers.
func (p *PoolWithFunc) Running() int32 {
	return p.workerCount.Load()
}

// Free returns the number of available worker slots.
func (p *PoolWithFunc) Free() int32 {
	return p.config.MaxWorkers - p.workerCount.Load()
}

// Waiting returns the number of waiting callers.
func (p *PoolWithFunc) Waiting() int32 {
	return p.metrics.BlockingTasks.Load()
}

// Idle returns the number of idle workers.
func (p *PoolWithFunc) Idle() int32 {
	return int32(p.workers.size())
}

// Cap returns the pool capacity.
func (p *PoolWithFunc) Cap() int32 {
	return p.config.MaxWorkers
}

// Name returns the pool name.
func (p *PoolWithFunc) Name() string {
	return p.name
}

// IsClosed returns true if the pool is closed.
func (p *PoolWithFunc) IsClosed() bool {
	return p.state.Load() == stateClosed
}

// Metrics returns performance metrics.
func (p *PoolWithFunc) Metrics() MetricsSnapshot {
	return p.metrics.Snapshot()
}

// ResetMetrics resets all metrics.
func (p *PoolWithFunc) ResetMetrics() {
	p.metrics.Reset()
}

// Uptime returns how long the pool has been running.
func (p *PoolWithFunc) Uptime() time.Duration {
	return time.Since(p.createdAt)
}

// Tune dynamically adjusts the pool capacity.
func (p *PoolWithFunc) Tune(newCap int32) {
	if newCap <= 0 || p.state.Load() == stateClosed {
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	p.config.MaxWorkers = newCap

	// Shrink if necessary
	for p.workerCount.Load() > newCap {
		if w := p.workers.pop(); w != nil {
			p.metrics.IdleWorkers.Add(-1)
			w.finish()
		} else {
			break
		}
	}
}

// OnHook registers a hook callback.
func (p *PoolWithFunc) OnHook(hookType HookType, fn HookFunc) {
	if p.hooks == nil {
		p.hooks = NewHooks()
	}
	p.hooks.Register(hookType, fn)
}

// Release shuts down the pool and waits for all workers to finish.
func (p *PoolWithFunc) Release() {
	if !p.state.CompareAndSwap(stateRunning, stateClosed) {
		return
	}

	p.lock.Lock()
	p.cond.Broadcast()
	p.lock.Unlock()

	close(p.heartbeat)

	// Close all idle workers
	// Need to keep trying because workers that were busy might
	// finish and push themselves back to the stack after we pop.
	for p.workerCount.Load() > 0 {
		for {
			if w := p.workers.pop(); w != nil {
				w.finish()
			} else {
				break
			}
		}
		time.Sleep(time.Millisecond)
	}

	p.wg.Wait()
}

// ReleaseTimeout shuts down with a timeout.
func (p *PoolWithFunc) ReleaseTimeout(timeout time.Duration) error {
	if !p.state.CompareAndSwap(stateRunning, stateClosed) {
		return nil
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
		return nil
	case <-time.After(remaining):
		return ErrTimeout
	}
}

// Reboot restarts the pool after it has been closed.
func (p *PoolWithFunc) Reboot() {
	if !p.state.CompareAndSwap(stateClosed, stateRunning) {
		return
	}

	p.lock.Lock()
	p.heartbeat = make(chan struct{})
	p.workers = newWorkerFuncStack(int(p.config.MaxWorkers))
	p.metrics.Reset()
	p.createdAt = time.Now()
	p.lock.Unlock()

	p.preheat()
	go p.purgeStaleWorkers()
}

// ============================================================================
// Worker for PoolWithFunc
// ============================================================================

type workerFunc struct {
	pool       *PoolWithFunc
	argCh      chan any
	lastActive atomic.Int64
	id         int32
}

func (w *workerFunc) run() {
	w.pool.wg.Add(1)
	go w.loop()
}

func (w *workerFunc) loop() {
	defer func() {
		w.pool.workerCount.Add(-1)
		w.pool.metrics.RunningWorkers.Add(-1)
		// Put back to cache before wg.Done to avoid race with Reboot
		w.pool.workerCache.Put(w)
		w.pool.wg.Done()
	}()

	for arg := range w.argCh {
		if arg == nil {
			return
		}

		w.execute(arg)
		w.lastActive.Store(time.Now().UnixNano())

		if !w.pool.revertWorker(w) {
			return
		}
	}
}

func (w *workerFunc) execute(arg any) {
	defer func() {
		if r := recover(); r != nil {
			w.pool.metrics.FailedTasks.Add(1)
			if w.pool.config.PanicHandler != nil {
				// 包装 panic handler 调用，防止它本身 panic
				func() {
					defer func() { recover() }()
					w.pool.config.PanicHandler(r)
				}()
			}
		}
	}()

	startTime := time.Now()
	w.pool.poolFunc(arg)
	execTime := time.Since(startTime)

	w.pool.metrics.TotalExecTime.Add(int64(execTime))
	w.pool.metrics.CompletedTasks.Add(1)
}

func (w *workerFunc) finish() {
	w.argCh <- nil
}

// ============================================================================
// Worker Stack for PoolWithFunc
// ============================================================================

type workerFuncStack struct {
	items  []*workerFunc
	expiry []*workerFunc
	head   int
	len    int
	cap    int
	lock   Spinlock
}

func newWorkerFuncStack(cap int) *workerFuncStack {
	return &workerFuncStack{
		items: make([]*workerFunc, cap),
		cap:   cap,
	}
}

func (s *workerFuncStack) push(w *workerFunc) bool {
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

func (s *workerFuncStack) pop() *workerFunc {
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

func (s *workerFuncStack) size() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.len
}

func (s *workerFuncStack) retrieveExpiry(duration time.Duration) []*workerFunc {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.len == 0 {
		return nil
	}

	s.expiry = s.expiry[:0]
	now := time.Now()

	for i := 0; i < s.len; i++ {
		idx := (s.head - s.len + i + s.cap) % s.cap
		w := s.items[idx]
		if w != nil && now.Sub(time.Unix(0, w.lastActive.Load())) > duration {
			s.expiry = append(s.expiry, w)
			s.items[idx] = nil
		}
	}

	// Compact
	newItems := make([]*workerFunc, s.cap)
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
