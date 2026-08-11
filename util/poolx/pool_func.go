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
	workerID    atomic.Int32
	workerCache sync.Pool

	// Argument channel
	argCh chan any

	// Blocking control
	waiters *waitNotifier

	// State
	state        atomic.Int32
	generationID atomic.Uint64
	generation   atomic.Pointer[poolFuncGeneration]

	// Hooks
	hooks *Hooks

	maxWorkers atomic.Int32

	panicHandler atomic.Pointer[panicHandlerSnapshot]

	lifecycleMu sync.Mutex
	lock        sync.Mutex
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
		argCh:       make(chan any, config.QueueSize),
		waiters:     newWaitNotifier(),
	}
	p.maxWorkers.Store(config.MaxWorkers)
	p.panicHandler.Store(newPanicHandlerSnapshot(config.PanicHandler))
	generation := newPoolFuncGeneration(p.generationID.Add(1), config.MaxWorkers)
	p.generation.Store(generation)

	p.workerCache.New = func() any {
		return &workerFunc{}
	}

	// Preheat workers
	p.lock.Lock()
	p.preheatLocked(generation)
	p.lock.Unlock()

	// Start cleaner
	go p.purgeStaleWorkers(generation)

	// 注意：PoolWithFunc 目前不支持 AutoScaler。
	// 如需自动扩缩容，请使用 Pool + SubmitFunc。

	return p
}

// preheatLocked 在调用方持有生命周期锁时创建本代际的最小 Worker 集合。
func (p *PoolWithFunc) preheatLocked(generation *poolFuncGeneration) {
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
func (p *PoolWithFunc) createWorkerLocked(generation *poolFuncGeneration) *workerFunc {
	if generation == nil || p.generation.Load() != generation || generation.state.Load() == stateClosed {
		return nil
	}
	if generation.workerCount.Load() >= p.maxWorkers.Load() {
		return nil
	}
	generation.workerCount.Add(1)
	generation.wg.Add(1)

	id := p.workerID.Add(1)
	w := mustPoolValue[*workerFunc](p.workerCache.Get())
	w.pool = p
	w.generation = generation
	w.id = id
	w.lastActive.Store(time.Now().UnixNano())
	w.argCh = make(chan any, 1)

	generation.metrics.RunningWorkers.Add(1)

	// Update peak
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
func (p *PoolWithFunc) retrieveWorkerLocked(generation *poolFuncGeneration) *workerFunc {
	if generation == nil || p.generation.Load() != generation || generation.state.Load() == stateClosed {
		return nil
	}
	if w := generation.workers.pop(); w != nil {
		generation.metrics.IdleWorkers.Add(-1)
		return w
	}

	// Try to create new worker
	if w := p.createWorkerLocked(generation); w != nil {
		w.run()
		return w
	}

	return nil
}

// revertWorker returns a worker to the idle stack
func (p *PoolWithFunc) revertWorker(w *workerFunc) bool {
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

// purgeStaleWorkers removes expired workers
func (p *PoolWithFunc) purgeStaleWorkers(generation *poolFuncGeneration) {
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

func (p *PoolWithFunc) cleanupExpiredWorkers(generation *poolFuncGeneration) {
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

// Invoke submits an argument to be processed by the pool function.
// Blocks if no workers are available.
func (p *PoolWithFunc) Invoke(arg any) error {
	generation := p.loadRunningGeneration()
	if generation == nil {
		return ErrPoolClosed
	}

	generation.metrics.SubmittedTasks.Add(1)

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		return ErrPoolClosed
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		w.argCh <- arg
		return nil
	}
	p.lock.Unlock()

	// Non-blocking mode
	if p.config.NonBlocking {
		generation.metrics.RejectedTasks.Add(1)
		return ErrPoolOverload
	}

	// Check blocking limit
	if p.config.MaxBlockingTasks > 0 {
		if generation.blockingCount.Load() >= p.config.MaxBlockingTasks {
			generation.metrics.RejectedTasks.Add(1)
			return ErrPoolOverload
		}
	}

	// Wait for worker
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
			return ErrPoolClosed
		}

		if w := p.retrieveWorkerLocked(generation); w != nil {
			p.lock.Unlock()
			w.argCh <- arg
			return nil
		}

		p.waiters.waitLocked(&p.lock, nil)
	}
}

func (p *PoolWithFunc) loadRunningGeneration() *poolFuncGeneration {
	generation := p.generation.Load()
	if generation == nil || generation.state.Load() == stateClosed {
		return nil
	}
	return generation
}

func (p *PoolWithFunc) generationRunningLocked(generation *poolFuncGeneration) bool {
	return generation != nil && p.generation.Load() == generation && generation.state.Load() == stateRunning
}

// TryInvoke attempts to submit an argument without blocking.
// Returns false if no worker is immediately available.
func (p *PoolWithFunc) TryInvoke(arg any) bool {
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
		generation.metrics.SubmittedTasks.Add(1)
		w.argCh <- arg
		return true
	}
	p.lock.Unlock()

	generation.metrics.RejectedTasks.Add(1)
	return false
}

// InvokeWithTimeout submits an argument with a timeout for getting a worker.
func (p *PoolWithFunc) InvokeWithTimeout(arg any, timeout time.Duration) error {
	generation := p.loadRunningGeneration()
	if generation == nil {
		return ErrPoolClosed
	}

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		return ErrPoolClosed
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		generation.metrics.SubmittedTasks.Add(1)
		w.argCh <- arg
		return nil
	}
	p.lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return p.invokeWithContext(ctx, generation, arg)
}

// InvokeWithContext submits an argument with context cancellation support.
func (p *PoolWithFunc) InvokeWithContext(ctx context.Context, arg any) error {
	generation := p.loadRunningGeneration()
	if generation == nil {
		return ErrPoolClosed
	}

	p.lock.Lock()
	if !p.generationRunningLocked(generation) {
		p.lock.Unlock()
		return ErrPoolClosed
	}
	if w := p.retrieveWorkerLocked(generation); w != nil {
		p.lock.Unlock()
		generation.metrics.SubmittedTasks.Add(1)
		w.argCh <- arg
		return nil
	}
	p.lock.Unlock()

	return p.invokeWithContext(ctx, generation, arg)
}

// invokeWithContext 直接在当前 goroutine 等待，避免启动额外 goroutine 导致泄漏。
func (p *PoolWithFunc) invokeWithContext(ctx context.Context, generation *poolFuncGeneration, arg any) error {
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
			w.argCh <- arg
			return nil
		}

		p.waiters.waitLocked(&p.lock, ctx.Done())
	}
}

// Running returns the number of running workers.
func (p *PoolWithFunc) Running() int32 {
	generation := p.generation.Load()
	if generation == nil {
		return 0
	}
	return generation.workerCount.Load()
}

// Free returns the number of available worker slots.
func (p *PoolWithFunc) Free() int32 {
	return p.maxWorkers.Load() - p.Running()
}

// Waiting returns the number of waiting callers.
func (p *PoolWithFunc) Waiting() int32 {
	generation := p.generation.Load()
	if generation == nil {
		return 0
	}
	return generation.metrics.BlockingTasks.Load()
}

// Idle returns the number of idle workers.
func (p *PoolWithFunc) Idle() int32 {
	generation := p.generation.Load()
	if generation == nil {
		return 0
	}
	return int32(generation.workers.size())
}

// Cap returns the pool capacity.
func (p *PoolWithFunc) Cap() int32 {
	return p.maxWorkers.Load()
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
	return p.generation.Load().metrics.Snapshot()
}

// ResetMetrics resets all metrics.
func (p *PoolWithFunc) ResetMetrics() {
	p.generation.Load().metrics.Reset()
}

// Uptime returns how long the pool has been running.
func (p *PoolWithFunc) Uptime() time.Duration {
	return time.Since(p.generation.Load().createdAt)
}

// Tune dynamically adjusts the pool capacity.
func (p *PoolWithFunc) Tune(newCap int32) {
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
func (p *PoolWithFunc) closeCurrentGeneration() (generation *poolFuncGeneration, done <-chan struct{}) {
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

// OnHook registers a hook callback.
func (p *PoolWithFunc) OnHook(hookType HookType, fn HookFunc) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.hooks == nil {
		p.hooks = NewHooks()
	}
	p.hooks.Register(hookType, fn)
}

// Release shuts down the pool and waits for all workers to finish.
func (p *PoolWithFunc) Release() {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	_, done := p.closeCurrentGeneration()
	if done == nil {
		return
	}
	<-done
}

// ReleaseTimeout shuts down with a timeout.
func (p *PoolWithFunc) ReleaseTimeout(timeout time.Duration) error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	deadline := time.Now().Add(timeout)
	_, done := p.closeCurrentGeneration()
	if done == nil {
		return nil
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		select {
		case <-done:
			return nil
		default:
		}
		return ErrTimeout
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return ErrTimeout
	}
}

// Reboot restarts the pool after it has been closed.
func (p *PoolWithFunc) Reboot() {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	p.lock.Lock()
	if p.state.Load() != stateClosed {
		p.lock.Unlock()
		return
	}
	generation := newPoolFuncGeneration(p.generationID.Add(1), p.maxWorkers.Load())
	p.generation.Store(generation)
	p.state.Store(stateRunning)
	p.preheatLocked(generation)
	p.lock.Unlock()

	go p.purgeStaleWorkers(generation)
}

// ============================================================================
// Worker for PoolWithFunc
// ============================================================================

type workerFunc struct {
	pool       *PoolWithFunc
	generation *poolFuncGeneration
	argCh      chan any
	lastActive atomic.Int64
	id         int32
}

func (w *workerFunc) run() {
	go w.loop()
}

func (w *workerFunc) loop() {
	pool := w.pool
	generation := w.generation
	defer func() {
		generation.workerCount.Add(-1)
		generation.metrics.RunningWorkers.Add(-1)
		pool.lock.Lock()
		if pool.generationRunningLocked(generation) {
			pool.waiters.signalLocked()
		}
		pool.lock.Unlock()
		generation.wg.Done()
		pool.workerCache.Put(w)
	}()

	for arg := range w.argCh {
		if arg == nil {
			return
		}

		w.execute(arg)
		w.lastActive.Store(time.Now().UnixNano())

		if !pool.revertWorker(w) {
			return
		}
	}
}

func (w *workerFunc) execute(arg any) {
	generation := w.generation
	defer func() {
		if r := recover(); r != nil {
			generation.metrics.FailedTasks.Add(1)
			if handler := w.pool.panicHandler.Load(); handler != nil {
				// 包装 panic handler 调用，防止它本身 panic
				func() {
					defer func() {
						if recover() != nil {
							return
						}
					}()
					handler.handle(r)
				}()
			}
		}
	}()

	startTime := time.Now()
	w.pool.poolFunc(arg)
	execTime := time.Since(startTime)

	generation.metrics.TotalExecTime.Add(int64(execTime))
	generation.metrics.CompletedTasks.Add(1)
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

func newWorkerFuncStack(capacity int) *workerFuncStack {
	return &workerFuncStack{
		items: make([]*workerFunc, capacity),
		cap:   capacity,
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

// resize 调整空闲栈容量，并保持当前 Worker 的栈顺序。
func (s *workerFuncStack) resize(capacity int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if capacity <= 0 || capacity == s.cap {
		return
	}
	if capacity < s.len {
		capacity = s.len
	}
	items := make([]*workerFunc, capacity)
	for index := 0; index < s.len; index++ {
		oldIndex := (s.head - s.len + index + s.cap) % s.cap
		items[index] = s.items[oldIndex]
	}
	s.items = items
	s.cap = capacity
	s.head = s.len % capacity
	s.expiry = nil
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

	// 原地紧缩，避免在持有 spinlock 时分配内存
	writeIdx := 0
	for i := 0; i < s.len; i++ {
		idx := (s.head - s.len + i + s.cap) % s.cap
		if s.items[idx] != nil {
			if writeIdx != idx {
				s.items[writeIdx] = s.items[idx]
				s.items[idx] = nil
			}
			writeIdx++
		}
	}
	// 清除尾部残留引用
	for i := writeIdx; i < s.cap; i++ {
		s.items[i] = nil
	}
	s.len = writeIdx
	s.head = writeIdx

	return s.expiry
}
