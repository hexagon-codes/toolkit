// Package event 提供轻量级事件总线
//
// 支持发布-订阅模式的事件分发，用于系统组件间的松耦合通信。
// 线程安全，支持按类型订阅和全局订阅。
//
// 使用示例:
//
//	bus, err := event.New()
//	if err != nil {
//	    return err
//	}
//	defer bus.Close()
//
//	unsub, err := bus.Subscribe("agent.start", func(e event.Event) {
//	    fmt.Println("Agent 启动:", e.Payload)
//	})
//	if err != nil {
//	    return err
//	}
//	defer unsub()
//
//	return bus.Publish(event.Event{Type: "agent.start", Payload: "my-agent"})
package event

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrNilOption 表示配置选项为 nil。
	ErrNilOption = errors.New("event: option must not be nil")
	// ErrInvalidMaxGoroutines 表示最大并发数不是正数。
	ErrInvalidMaxGoroutines = errors.New("event: max goroutines must be greater than zero")
	// ErrInvalidMaxPendingDeliveries 表示最大待处理数不是正数。
	ErrInvalidMaxPendingDeliveries = errors.New("event: max pending deliveries must be greater than zero")
	// ErrPendingDeliveryCapacityExceeded 表示异步待处理容量不足。
	ErrPendingDeliveryCapacityExceeded = errors.New("event: pending delivery capacity exceeded")
	// ErrNilPanicHandler 表示 panic 处理器为 nil。
	ErrNilPanicHandler = errors.New("event: panic handler must not be nil")
	// ErrNilHandler 表示事件处理器为 nil。
	ErrNilHandler = errors.New("event: handler must not be nil")
	// ErrNilBus 表示事件总线为 nil。
	ErrNilBus = errors.New("event: bus must not be nil")
	// ErrClosed 表示事件总线已经关闭。
	ErrClosed = errors.New("event: bus is closed")
	// ErrInvalidContext 表示调用方传入了 nil context。
	ErrInvalidContext = errors.New("event: context must not be nil")
)

// 预定义事件类型常量
const (
	// Agent 生命周期事件
	EventAgentStart = "agent.start"
	EventAgentEnd   = "agent.end"
	EventAgentError = "agent.error"

	// 工具调用事件
	EventToolCall   = "tool.call"
	EventToolResult = "tool.result"

	// LLM 调用事件
	EventLLMRequest  = "llm.request"
	EventLLMResponse = "llm.response"
	EventLLMStream   = "llm.stream"

	// Skill 生命周期事件
	EventSkillLoad   = "skill.load"
	EventSkillUnload = "skill.unload"

	// 成本事件
	EventCostUpdate = "cost.update"

	// 安全事件
	EventSecurityAlert = "security.alert"

	// 默认最大并发 goroutine 数
	defaultMaxGoroutines = 1024
	// 默认最大待处理 delivery 数，不包含正在执行的 worker
	defaultMaxPendingDeliveries = 4096
)

// Event 事件结构
type Event struct {
	// Type 事件类型（如 "agent.start"）
	Type string
	// Payload 事件数据（任意类型）
	Payload any
	// Timestamp 事件发生时间
	Timestamp time.Time
	// Source 事件来源（如 Agent ID）
	Source string
	// ID 事件唯一标识
	ID string
}

// Handler 事件处理函数
type Handler func(Event)

// PanicHandler panic 处理回调
type PanicHandler func(event Event, panicVal any)

// subscription 订阅记录
type subscription struct {
	id      uint64
	handler Handler
}

// delivery 表示一次已经被 Publish 接受的处理器调用。
type delivery struct {
	handler Handler
	event   Event
}

// BusOption 事件总线配置选项
type BusOption func(*Bus) error

// WithPanicHandler 设置 panic 处理回调
func WithPanicHandler(h PanicHandler) BusOption {
	return func(b *Bus) error {
		if h == nil {
			return ErrNilPanicHandler
		}
		b.panicHandler = h
		return nil
	}
}

// WithMaxGoroutines 设置异步 handler 的最大并发 worker 数，默认值为 1024。
func WithMaxGoroutines(n int) BusOption {
	return func(b *Bus) error {
		if n <= 0 {
			return ErrInvalidMaxGoroutines
		}
		b.maxGoroutines = n
		return nil
	}
}

// WithMaxPendingDeliveries 设置异步 worker 之外允许等待的最大 delivery 数，默认值为 4096。
// 因此异步总在途上限为最大 worker 数与该值之和。
func WithMaxPendingDeliveries(n int) BusOption {
	return func(b *Bus) error {
		if n <= 0 {
			return ErrInvalidMaxPendingDeliveries
		}
		b.maxPendingDeliveries = n
		return nil
	}
}

// Bus 事件总线
//
// 线程安全的发布-订阅事件分发器。
// 支持按类型订阅和全局订阅（订阅所有事件）。
// 零值可直接使用；需要自定义配置时使用 New。
type Bus struct {
	// initOnce 保证零值总线只初始化一次
	initOnce sync.Once
	// mu 保护 subscribers 和 globalSubs
	mu sync.RWMutex
	// subscribers 按事件类型索引的订阅者
	subscribers map[string][]subscription
	// globalSubs 全局订阅者（接收所有事件）
	globalSubs []subscription
	// nextID 递增的订阅 ID
	nextID atomic.Uint64
	// closed 总线是否已关闭
	closed atomic.Bool
	// maxGoroutines 限制异步 handler 的并发执行数
	maxGoroutines int
	// maxPendingDeliveries 限制异步 worker 之外等待的 delivery 数
	maxPendingDeliveries int
	// dispatchMu 保护异步调度队列、worker 和 outstanding 计数
	dispatchMu sync.Mutex
	// pending 保存已接受但尚未开始执行的 handler
	pending []delivery
	// pendingHead 指向 pending 中下一项待处理任务
	pendingHead int
	// workers 记录当前存活的异步 worker 数
	workers int
	// outstanding 记录正在执行和等待执行的异步 delivery 总数
	outstanding int
	// wg 等待全部已接受的 handler 完成
	wg sync.WaitGroup
	// closeOnce 保证关闭流程只启动一次
	closeOnce sync.Once
	// done 在全部已接受的 handler 返回后关闭
	done chan struct{}
	// panicHandler 可选的 panic 处理回调
	panicHandler PanicHandler
}

// New 创建事件总线
func New(opts ...BusOption) (*Bus, error) {
	b := &Bus{}
	b.initialize()
	for _, opt := range opts {
		if opt == nil {
			return nil, ErrNilOption
		}
		if err := opt(b); err != nil {
			return nil, err
		}
	}
	b.initialize()
	return b, nil
}

func (b *Bus) initialize() {
	b.initOnce.Do(func() {
		b.subscribers = make(map[string][]subscription)
		b.maxGoroutines = defaultMaxGoroutines
		b.maxPendingDeliveries = defaultMaxPendingDeliveries
		b.done = make(chan struct{})
	})
}

// Subscribe 订阅指定类型的事件
//
// 返回取消订阅函数。取消仅影响此后接受的发布；已经捕获该订阅快照的发布仍会执行。
func (b *Bus) Subscribe(eventType string, handler Handler) (unsubscribe func(), err error) {
	if b == nil {
		return nil, ErrNilBus
	}
	if handler == nil {
		return nil, ErrNilHandler
	}
	b.initialize()

	b.mu.Lock()
	// 在锁内检查 closed，避免 TOCTOU 竞态
	if b.closed.Load() {
		b.mu.Unlock()
		return nil, ErrClosed
	}

	id := b.nextID.Add(1)
	sub := subscription{id: id, handler: handler}

	b.subscribers[eventType] = append(b.subscribers[eventType], sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs, removed := removeSubscription(b.subscribers[eventType], id)
		if !removed {
			return
		}
		if len(subs) == 0 {
			delete(b.subscribers, eventType)
			return
		}
		b.subscribers[eventType] = subs
	}, nil
}

// SubscribeAll 订阅所有事件
//
// 返回取消订阅函数。取消仅影响此后接受的发布；已经捕获该订阅快照的发布仍会执行。
func (b *Bus) SubscribeAll(handler Handler) (unsubscribe func(), err error) {
	if b == nil {
		return nil, ErrNilBus
	}
	if handler == nil {
		return nil, ErrNilHandler
	}
	b.initialize()

	b.mu.Lock()
	// 在锁内检查 closed，避免 TOCTOU 竞态
	if b.closed.Load() {
		b.mu.Unlock()
		return nil, ErrClosed
	}

	id := b.nextID.Add(1)
	sub := subscription{id: id, handler: handler}

	b.globalSubs = append(b.globalSubs, sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs, removed := removeSubscription(b.globalSubs, id)
		if !removed {
			return
		}
		if len(subs) == 0 {
			b.globalSubs = nil
			return
		}
		b.globalSubs = subs
	}, nil
}

func removeSubscription(subs []subscription, id uint64) ([]subscription, bool) {
	for i, sub := range subs {
		if sub.id == id {
			return slices.Delete(subs, i, i+1), true
		}
	}
	return subs, false
}

// Publish 异步发布事件
//
// 订阅者由有界 worker 并发接收事件。
// 事件处理器中的 panic 会被捕获。
// 达到并发上限时，已接受的处理器进入内部队列，Publish 不等待 handler 完成。
// 当整个订阅快照无法原子进入剩余容量时，返回 ErrPendingDeliveryCapacityExceeded，
// 且不会执行其中任何 handler。
// Publish 返回 nil 后，即使随后调用 Close，对应订阅快照也会被完整执行。
func (b *Bus) Publish(event Event) error {
	if b == nil {
		return ErrNilBus
	}
	b.initialize()
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 在读锁保护下完成 closed 检查、订阅者复制和 wg.Add，
	// 确保与 Close() 中的 wg.Wait() 不产生竞态
	b.mu.RLock()
	if b.closed.Load() {
		b.mu.RUnlock()
		return ErrClosed
	}
	typeSubs := b.subscribers[event.Type]
	globalSubs := b.globalSubs
	total := len(typeSubs) + len(globalSubs)
	if total == 0 {
		b.mu.RUnlock()
		return nil
	}

	b.dispatchMu.Lock()
	capacity := b.deliveryCapacity()
	available := capacity - b.outstanding
	if total > available {
		outstanding := b.outstanding
		b.dispatchMu.Unlock()
		b.mu.RUnlock()
		return fmt.Errorf(
			"%w: outstanding=%d requested=%d limit=%d",
			ErrPendingDeliveryCapacityExceeded,
			outstanding,
			total,
			capacity,
		)
	}

	// 容量确认后再复制快照，避免被拒绝的大订阅集合产生无界临时分配。
	deliveries := make([]delivery, 0, total)
	for _, sub := range typeSubs {
		deliveries = append(deliveries, delivery{handler: sub.handler, event: event})
	}
	for _, sub := range globalSubs {
		deliveries = append(deliveries, delivery{handler: sub.handler, event: event})
	}
	b.wg.Add(total)
	b.outstanding += total
	b.pending = append(b.pending, deliveries...)
	workersToStart := b.reserveWorkersLocked()
	b.dispatchMu.Unlock()
	b.mu.RUnlock()

	b.startWorkers(workersToStart)
	return nil
}

func (b *Bus) deliveryCapacity() int {
	maxInt := int(^uint(0) >> 1)
	if b.maxPendingDeliveries > maxInt-b.maxGoroutines {
		return maxInt
	}
	return b.maxGoroutines + b.maxPendingDeliveries
}

func (b *Bus) reserveWorkersLocked() int {
	workersToStart := min(b.maxGoroutines-b.workers, len(b.pending)-b.pendingHead)
	b.workers += workersToStart
	return workersToStart
}

func (b *Bus) startWorkers(count int) {
	for range count {
		go b.runWorker()
	}
}

func (b *Bus) runWorker() {
	defer b.workerStopped()
	for {
		next, ok := b.nextDelivery()
		if !ok {
			return
		}
		func() {
			defer b.deliveryCompleted()
			b.safeCall(next.handler, next.event)
		}()
	}
}

func (b *Bus) deliveryCompleted() {
	b.dispatchMu.Lock()
	b.outstanding--
	b.dispatchMu.Unlock()
	b.wg.Done()
}

func (b *Bus) nextDelivery() (delivery, bool) {
	b.dispatchMu.Lock()
	defer b.dispatchMu.Unlock()

	if b.pendingHead == len(b.pending) {
		b.pending = nil
		b.pendingHead = 0
		return delivery{}, false
	}

	next := b.pending[b.pendingHead]
	b.pending[b.pendingHead] = delivery{}
	b.pendingHead++
	if b.pendingHead >= 1024 && b.pendingHead*2 >= len(b.pending) {
		b.pending = append([]delivery(nil), b.pending[b.pendingHead:]...)
		b.pendingHead = 0
	}
	return next, true
}

func (b *Bus) workerStopped() {
	b.dispatchMu.Lock()
	b.workers--
	workersToStart := b.reserveWorkersLocked()
	b.dispatchMu.Unlock()

	b.startWorkers(workersToStart)
}

// PublishSync 同步发布事件
//
// 按订阅顺序逐个调用所有订阅者，阻塞直到所有处理器执行完毕。
// 每个处理器使用独立 goroutine 隔离 runtime.Goexit，避免中断剩余订阅快照。
func (b *Bus) PublishSync(event Event) error {
	if b == nil {
		return ErrNilBus
	}
	b.initialize()
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.RLock()
	if b.closed.Load() {
		b.mu.RUnlock()
		return ErrClosed
	}
	typeSubs := make([]subscription, len(b.subscribers[event.Type]))
	copy(typeSubs, b.subscribers[event.Type])
	globalSubs := make([]subscription, len(b.globalSubs))
	copy(globalSubs, b.globalSubs)
	total := len(typeSubs) + len(globalSubs)
	if total > 0 {
		b.wg.Add(total)
	}
	b.mu.RUnlock()

	for _, sub := range typeSubs {
		b.callSync(sub.handler, event)
	}
	for _, sub := range globalSubs {
		b.callSync(sub.handler, event)
	}
	return nil
}

func (b *Bus) callSync(handler Handler, event Event) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer b.wg.Done()
		b.safeCall(handler, event)
	}()
	<-done
}

// Close 启动事件总线关闭流程
//
// 关闭后不再接受新的订阅和发布，但会继续排空此前接受的订阅快照。该方法不等待
// handler，因而可以安全地从 handler 内部调用；需要优雅等待时使用 Shutdown。
func (b *Bus) Close() {
	if b == nil {
		return
	}
	b.initialize()
	b.closeOnce.Do(func() {
		// 写锁确保与 Publish 中的 wg.Add 互斥；解锁后不会再有新的 Add。
		b.mu.Lock()
		b.closed.Store(true)
		b.subscribers = make(map[string][]subscription)
		b.globalSubs = nil
		b.mu.Unlock()

		go func() {
			b.wg.Wait()
			close(b.done)
		}()
	})
}

// Shutdown 关闭事件总线并等待全部已接受的 handler 返回。
// context 取消只终止本次等待，不会放弃队列；后续可再次调用 Shutdown 继续等待。
func (b *Bus) Shutdown(ctx context.Context) error {
	if b == nil {
		return ErrNilBus
	}
	if isNilContext(ctx) {
		return ErrInvalidContext
	}
	b.Close()
	select {
	case <-b.done:
		return nil
	default:
	}
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		select {
		case <-b.done:
			return nil
		default:
			return contextTerminationError(ctx)
		}
	}
}

func contextTerminationError(ctx context.Context) error {
	err := ctx.Err()
	cause := context.Cause(ctx)
	if isNilValue(cause) {
		return err
	}
	if err == nil || errors.Is(cause, err) {
		return cause
	}
	// 先保留标准 context 错误，再附加调用方 cause，确保匹配与输出次序稳定。
	return errors.Join(err, cause)
}

func isNilContext(ctx context.Context) bool {
	return isNilValue(ctx)
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Done 返回全部活跃 handler 完成后关闭的通道。
func (b *Bus) Done() <-chan struct{} {
	if b == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	b.initialize()
	return b.done
}

// Len 返回当前订阅总数
func (b *Bus) Len() int {
	if b == nil {
		return 0
	}
	b.initialize()
	b.mu.RLock()
	defer b.mu.RUnlock()
	count := len(b.globalSubs)
	for _, subs := range b.subscribers {
		count += len(subs)
	}
	return count
}

// safeCall 安全调用 handler，捕获 panic 并通过 PanicHandler 通知
func (b *Bus) safeCall(handler Handler, event Event) {
	defer func() {
		if r := recover(); r != nil {
			b.notifyPanic(event, r)
		}
	}()
	handler(event)
}

// notifyPanic 通知 panic 处理器，并隔离处理器自身的 panic。
func (b *Bus) notifyPanic(event Event, handlerPanic any) {
	if b.panicHandler == nil {
		logPanic("[event] handler panic: event=%s, panic=%v", event.Type, handlerPanic)
		return
	}

	defer func() {
		if panicHandlerPanic := recover(); panicHandlerPanic != nil {
			logPanic(
				"[event] panic handler failed: event=%s, handler_panic=%v, panic_handler_panic=%v",
				event.Type,
				handlerPanic,
				panicHandlerPanic,
			)
		}
	}()
	b.panicHandler(event, handlerPanic)
}

// logPanic 保证宿主替换的日志 writer 不会破坏 handler 的 panic 隔离边界。
func logPanic(format string, args ...any) {
	defer func() {
		if recovered := recover(); recovered == nil {
			return
		}
	}()
	log.Printf(format, args...)
}
