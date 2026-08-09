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

// WithMaxGoroutines 设置最大并发 goroutine 数
func WithMaxGoroutines(n int) BusOption {
	return func(b *Bus) error {
		if n <= 0 {
			return ErrInvalidMaxGoroutines
		}
		b.sem = make(chan struct{}, n)
		return nil
	}
}

// Bus 事件总线
//
// 线程安全的发布-订阅事件分发器。
// 支持按类型订阅和全局订阅（订阅所有事件）。
type Bus struct {
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
	// sem 信号量，限制并发 goroutine 数
	sem chan struct{}
	// wg 等待活跃 handler 完成
	wg sync.WaitGroup
	// closeOnce 保证关闭流程只启动一次
	closeOnce sync.Once
	// done 在全部活跃 handler 返回后关闭
	done chan struct{}
	// panicHandler 可选的 panic 处理回调
	panicHandler PanicHandler
}

// New 创建事件总线
func New(opts ...BusOption) (*Bus, error) {
	b := &Bus{
		subscribers: make(map[string][]subscription),
		sem:         make(chan struct{}, defaultMaxGoroutines),
		done:        make(chan struct{}),
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, ErrNilOption
		}
		if err := opt(b); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// Subscribe 订阅指定类型的事件
//
// 返回取消订阅函数。调用取消函数后，该处理器不再接收事件。
func (b *Bus) Subscribe(eventType string, handler Handler) (unsubscribe func(), err error) {
	if b == nil {
		return nil, ErrNilBus
	}
	if handler == nil {
		return nil, ErrNilHandler
	}

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
// 返回取消订阅函数。
func (b *Bus) SubscribeAll(handler Handler) (unsubscribe func(), err error) {
	if b == nil {
		return nil, ErrNilBus
	}
	if handler == nil {
		return nil, ErrNilHandler
	}

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
// 每个订阅者在独立的 goroutine 中接收事件。
// 事件处理器中的 panic 会被捕获。
// 使用信号量限制并发 goroutine 数量，信号量满时会阻塞发布者。
func (b *Bus) Publish(event Event) error {
	if b == nil {
		return ErrNilBus
	}
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
	// 复制订阅者列表，避免持锁执行 handler
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
		b.sem <- struct{}{} // 获取信号量，限制并发
		go func(s subscription) {
			defer func() {
				<-b.sem // 释放信号量
				b.wg.Done()
			}()
			b.safeCall(s.handler, event)
		}(sub)
	}
	for _, sub := range globalSubs {
		b.sem <- struct{}{} // 获取信号量，限制并发
		go func(s subscription) {
			defer func() {
				<-b.sem // 释放信号量
				b.wg.Done()
			}()
			b.safeCall(s.handler, event)
		}(sub)
	}
	return nil
}

// PublishSync 同步发布事件
//
// 在当前 goroutine 中依次调用所有订阅者，
// 阻塞直到所有处理器执行完毕。
func (b *Bus) PublishSync(event Event) error {
	if b == nil {
		return ErrNilBus
	}
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
		func() {
			defer b.wg.Done()
			b.safeCall(sub.handler, event)
		}()
	}
	for _, sub := range globalSubs {
		func() {
			defer b.wg.Done()
			b.safeCall(sub.handler, event)
		}()
	}
	return nil
}

// Close 启动事件总线关闭流程
//
// 关闭后不再接受新的订阅和发布。该方法不等待活跃 handler，因而可以安全地从
// handler 内部调用；需要优雅等待时使用 Shutdown。
func (b *Bus) Close() {
	if b == nil {
		return
	}
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

// Shutdown 关闭事件总线并等待全部活跃 handler 返回。
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
			return ctx.Err()
		}
	}
}

func isNilContext(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	value := reflect.ValueOf(ctx)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
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
	return b.done
}

// Len 返回当前订阅总数
func (b *Bus) Len() int {
	if b == nil {
		return 0
	}
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
		log.Printf("[event] handler panic: event=%s, panic=%v", event.Type, handlerPanic)
		return
	}

	defer func() {
		if panicHandlerPanic := recover(); panicHandlerPanic != nil {
			log.Printf(
				"[event] panic handler failed: event=%s, handler_panic=%v, panic_handler_panic=%v",
				event.Type,
				handlerPanic,
				panicHandlerPanic,
			)
		}
	}()
	b.panicHandler(event, handlerPanic)
}
