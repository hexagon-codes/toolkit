// Package event 提供轻量级事件总线
//
// 支持发布-订阅模式的事件分发，用于系统组件间的松耦合通信。
// 线程安全，支持按类型订阅和全局订阅。
//
// 使用示例:
//
//	bus := event.New()
//	defer bus.Close()
//
//	unsub := bus.Subscribe("agent.start", func(e event.Event) {
//	    fmt.Println("Agent 启动:", e.Payload)
//	})
//	defer unsub()
//
//	bus.Publish(event.Event{Type: "agent.start", Payload: "my-agent"})
package event

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
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
type BusOption func(*Bus)

// WithPanicHandler 设置 panic 处理回调
func WithPanicHandler(h PanicHandler) BusOption {
	return func(b *Bus) {
		b.panicHandler = h
	}
}

// WithMaxGoroutines 设置最大并发 goroutine 数
func WithMaxGoroutines(n int) BusOption {
	return func(b *Bus) {
		if n > 0 {
			b.sem = make(chan struct{}, n)
		}
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
	// panicHandler 可选的 panic 处理回调
	panicHandler PanicHandler
}

// New 创建事件总线
func New(opts ...BusOption) *Bus {
	b := &Bus{
		subscribers: make(map[string][]subscription),
		sem:         make(chan struct{}, defaultMaxGoroutines),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Subscribe 订阅指定类型的事件
//
// 返回取消订阅函数。调用取消函数后，该处理器不再接收事件。
func (b *Bus) Subscribe(eventType string, handler Handler) (unsubscribe func()) {
	b.mu.Lock()
	// 在锁内检查 closed，避免 TOCTOU 竞态
	if b.closed.Load() {
		b.mu.Unlock()
		return func() {}
	}

	id := b.nextID.Add(1)
	sub := subscription{id: id, handler: handler}

	b.subscribers[eventType] = append(b.subscribers[eventType], sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[eventType]
		for i, s := range subs {
			if s.id == id {
				b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
}

// SubscribeAll 订阅所有事件
//
// 返回取消订阅函数。
func (b *Bus) SubscribeAll(handler Handler) (unsubscribe func()) {
	b.mu.Lock()
	// 在锁内检查 closed，避免 TOCTOU 竞态
	if b.closed.Load() {
		b.mu.Unlock()
		return func() {}
	}

	id := b.nextID.Add(1)
	sub := subscription{id: id, handler: handler}

	b.globalSubs = append(b.globalSubs, sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, s := range b.globalSubs {
			if s.id == id {
				b.globalSubs = append(b.globalSubs[:i], b.globalSubs[i+1:]...)
				break
			}
		}
	}
}

// Publish 异步发布事件
//
// 每个订阅者在独立的 goroutine 中接收事件，
// 不会阻塞发布者。事件处理器中的 panic 会被捕获。
// 使用信号量限制并发 goroutine 数量。
func (b *Bus) Publish(event Event) {
	if b.closed.Load() {
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.RLock()
	// 复制订阅者列表，避免持锁执行 handler
	typeSubs := make([]subscription, len(b.subscribers[event.Type]))
	copy(typeSubs, b.subscribers[event.Type])
	globalSubs := make([]subscription, len(b.globalSubs))
	copy(globalSubs, b.globalSubs)
	b.mu.RUnlock()

	for _, sub := range typeSubs {
		b.wg.Add(1)
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
		b.wg.Add(1)
		b.sem <- struct{}{} // 获取信号量，限制并发
		go func(s subscription) {
			defer func() {
				<-b.sem // 释放信号量
				b.wg.Done()
			}()
			b.safeCall(s.handler, event)
		}(sub)
	}
}

// PublishSync 同步发布事件
//
// 在当前 goroutine 中依次调用所有订阅者，
// 阻塞直到所有处理器执行完毕。
func (b *Bus) PublishSync(event Event) {
	if b.closed.Load() {
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.RLock()
	typeSubs := make([]subscription, len(b.subscribers[event.Type]))
	copy(typeSubs, b.subscribers[event.Type])
	globalSubs := make([]subscription, len(b.globalSubs))
	copy(globalSubs, b.globalSubs)
	b.mu.RUnlock()

	for _, sub := range typeSubs {
		b.safeCall(sub.handler, event)
	}
	for _, sub := range globalSubs {
		b.safeCall(sub.handler, event)
	}
}

// Close 关闭事件总线
//
// 关闭后不再接受新的订阅和发布。
// 等待所有活跃的 handler 执行完毕后返回。
func (b *Bus) Close() {
	b.closed.Store(true)
	// 等待所有活跃的 handler 完成
	b.wg.Wait()
	b.mu.Lock()
	b.subscribers = make(map[string][]subscription)
	b.globalSubs = nil
	b.mu.Unlock()
}

// Len 返回当前订阅总数
func (b *Bus) Len() int {
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
			if b.panicHandler != nil {
				b.panicHandler(event, r)
			} else {
				// 默认输出 panic 信息到标准错误
				fmt.Printf("[event] handler panic: event=%s, panic=%v\n", event.Type, r)
			}
		}
	}()
	handler(event)
}
