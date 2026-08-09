package event

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type typedNilContext struct{}

func (*typedNilContext) Deadline() (time.Time, bool) { panic("typed nil context used") }
func (*typedNilContext) Done() <-chan struct{}       { panic("typed nil context used") }
func (*typedNilContext) Err() error                  { panic("typed nil context used") }
func (*typedNilContext) Value(any) any               { panic("typed nil context used") }

func newTestBus(t *testing.T, opts ...BusOption) *Bus {
	t.Helper()

	bus, err := New(opts...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := bus.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})
	return bus
}

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := newTestBus(t)

	received := make(chan Event, 1)
	if _, err := bus.Subscribe(EventAgentStart, func(event Event) {
		received <- event
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	want := Event{Type: EventAgentStart, Payload: "test"}
	if err := bus.Publish(want); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	select {
	case got := <-received:
		if got.Type != want.Type || got.Payload != want.Payload {
			t.Fatalf("received event = %#v, want type %q and payload %q", got, want.Type, want.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not receive event")
	}
}

func TestBus_PublishSync(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()

	var received bool
	bus.Subscribe(EventToolCall, func(e Event) {
		received = true
	})

	bus.PublishSync(Event{Type: EventToolCall})

	if !received {
		t.Error("同步发布后应该立即收到")
	}
}

func TestBus_SubscribeAll(t *testing.T) {
	bus := newTestBus(t)

	received := make(chan string, 2)
	if _, err := bus.SubscribeAll(func(event Event) {
		received <- event.Type
	}); err != nil {
		t.Fatalf("SubscribeAll() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventToolCall}); err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}

	seen := make(map[string]bool, 2)
	for range 2 {
		select {
		case eventType := <-received:
			seen[eventType] = true
		case <-time.After(time.Second):
			t.Fatal("global handler did not receive every event")
		}
	}
	if !seen[EventAgentStart] || !seen[EventToolCall] {
		t.Fatalf("received event types = %v, want both published types", seen)
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()

	var count atomic.Int32
	unsub, err := bus.Subscribe(EventAgentStart, func(e Event) {
		count.Add(1)
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	bus.PublishSync(Event{Type: EventAgentStart})
	unsub()
	bus.PublishSync(Event{Type: EventAgentStart})

	if count.Load() != 1 {
		t.Errorf("取消订阅后不应收到事件, count=%d", count.Load())
	}
}

func TestUnsubscribeReleasesHandlerReferences(t *testing.T) {
	t.Run("typed subscriptions", func(t *testing.T) {
		bus := newTestBus(t)
		first, err := bus.Subscribe(EventAgentStart, func(Event) {})
		if err != nil {
			t.Fatalf("first Subscribe() error = %v", err)
		}
		second, err := bus.Subscribe(EventAgentStart, func(Event) {})
		if err != nil {
			t.Fatalf("second Subscribe() error = %v", err)
		}

		first()
		bus.mu.RLock()
		subs := bus.subscribers[EventAgentStart]
		if len(subs) != 1 {
			bus.mu.RUnlock()
			t.Fatalf("subscription count = %d, want 1", len(subs))
		}
		retainedTail := subs[:cap(subs)][len(subs)].handler != nil
		bus.mu.RUnlock()
		if retainedTail {
			t.Fatal("unsubscribed handler remains referenced in the slice tail")
		}

		second()
		bus.mu.RLock()
		_, exists := bus.subscribers[EventAgentStart]
		bus.mu.RUnlock()
		if exists {
			t.Fatal("empty typed subscription entry remains in the map")
		}
	})

	t.Run("global subscriptions", func(t *testing.T) {
		bus := newTestBus(t)
		unsubscribe, err := bus.SubscribeAll(func(Event) {})
		if err != nil {
			t.Fatalf("SubscribeAll() error = %v", err)
		}

		unsubscribe()
		bus.mu.RLock()
		globalSubs := bus.globalSubs
		bus.mu.RUnlock()
		if globalSubs != nil {
			t.Fatal("empty global subscription slice retains its backing array")
		}
	})
}

func TestBus_Close(t *testing.T) {
	bus := newTestBus(t)
	bus.Subscribe(EventAgentStart, func(e Event) {})
	bus.Close()

	if bus.Len() != 0 {
		t.Error("关闭后订阅数应为 0")
	}

}

func TestBus_Len(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()

	if bus.Len() != 0 {
		t.Error("初始订阅数应为 0")
	}

	unsub1, err := bus.Subscribe(EventAgentStart, func(e Event) {})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	bus.Subscribe(EventToolCall, func(e Event) {})
	bus.SubscribeAll(func(e Event) {})

	if bus.Len() != 3 {
		t.Errorf("期望 3 个订阅, 实际 %d", bus.Len())
	}

	unsub1()
	if bus.Len() != 2 {
		t.Errorf("取消后期望 2 个订阅, 实际 %d", bus.Len())
	}
}

func TestBus_Concurrent(t *testing.T) {
	bus := newTestBus(t)

	var count atomic.Int64
	for i := 0; i < 10; i++ {
		if _, err := bus.Subscribe(EventAgentStart, func(Event) {
			count.Add(1)
		}); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}

	var wg sync.WaitGroup
	publishErrors := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			publishErrors <- bus.Publish(Event{Type: EventAgentStart})
		}()
	}
	wg.Wait()
	close(publishErrors)
	for err := range publishErrors {
		if err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if count.Load() != 1000 {
		t.Fatalf("handler call count = %d, want 1000", count.Load())
	}
}

func TestBus_PanicRecovery(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()

	bus.Subscribe(EventAgentStart, func(e Event) {
		panic("handler panic")
	})

	// 不应 panic
	bus.PublishSync(Event{Type: EventAgentStart})
}

func TestBus_AutoTimestamp(t *testing.T) {
	bus := newTestBus(t)
	defer bus.Close()

	var received Event
	bus.Subscribe(EventAgentStart, func(e Event) {
		received = e
	})

	bus.PublishSync(Event{Type: EventAgentStart})

	if received.Timestamp.IsZero() {
		t.Error("未设置 Timestamp 时应自动填充")
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opt  BusOption
		want error
	}{
		{name: "nil option", opt: nil, want: ErrNilOption},
		{name: "zero max goroutines", opt: WithMaxGoroutines(0), want: ErrInvalidMaxGoroutines},
		{name: "negative max goroutines", opt: WithMaxGoroutines(-1), want: ErrInvalidMaxGoroutines},
		{name: "nil panic handler", opt: WithPanicHandler(nil), want: ErrNilPanicHandler},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bus, err := New(tt.opt)
			if bus != nil {
				t.Fatalf("New() bus = %v, want nil", bus)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("New() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSubscribeRejectsNilHandlerAndClosedBus(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	if _, err := bus.Subscribe(EventAgentStart, nil); !errors.Is(err, ErrNilHandler) {
		t.Fatalf("Subscribe() error = %v, want %v", err, ErrNilHandler)
	}
	if _, err := bus.SubscribeAll(nil); !errors.Is(err, ErrNilHandler) {
		t.Fatalf("SubscribeAll() error = %v, want %v", err, ErrNilHandler)
	}

	bus.Close()
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Subscribe() after Close error = %v, want %v", err, ErrClosed)
	}
	if _, err := bus.SubscribeAll(func(Event) {}); !errors.Is(err, ErrClosed) {
		t.Fatalf("SubscribeAll() after Close error = %v, want %v", err, ErrClosed)
	}
}

func TestPanicHandlerPanicDoesNotEscape(t *testing.T) {
	t.Parallel()

	var panicCalls atomic.Int32
	bus := newTestBus(t, WithPanicHandler(func(Event, any) {
		panicCalls.Add(1)
		panic("panic handler failed")
	}))

	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		panic("handler failed")
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var remainingHandlerCalled atomic.Bool
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		remainingHandlerCalled.Store(true)
	}); err != nil {
		t.Fatalf("second Subscribe() error = %v", err)
	}

	if err := bus.PublishSync(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("PublishSync() error = %v", err)
	}
	if got := panicCalls.Load(); got != 1 {
		t.Fatalf("panic handler call count = %d, want 1", got)
	}
	if !remainingHandlerCalled.Load() {
		t.Fatal("panic handler panic prevented a remaining handler")
	}
}

func TestPublishReportsNilAndClosedBus(t *testing.T) {
	t.Parallel()

	var nilBus *Bus
	if err := nilBus.Publish(Event{}); !errors.Is(err, ErrNilBus) {
		t.Fatalf("nil Bus.Publish() error = %v, want %v", err, ErrNilBus)
	}
	if err := nilBus.PublishSync(Event{}); !errors.Is(err, ErrNilBus) {
		t.Fatalf("nil Bus.PublishSync() error = %v, want %v", err, ErrNilBus)
	}
	if nilBus.Len() != 0 {
		t.Fatalf("nil Bus.Len() = %d, want 0", nilBus.Len())
	}
	nilBus.Close()

	bus := newTestBus(t)
	bus.Close()
	if err := bus.Publish(Event{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Bus.Publish() error = %v, want %v", err, ErrClosed)
	}
	if err := bus.PublishSync(Event{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Bus.PublishSync() error = %v, want %v", err, ErrClosed)
	}
}

func TestWithMaxGoroutinesBoundsActiveHandlers(t *testing.T) {
	t.Parallel()

	const maxGoroutines = 2
	bus := newTestBus(t, WithMaxGoroutines(maxGoroutines))
	defer bus.Close()

	var active atomic.Int32
	var peak atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	for range 8 {
		if _, err := bus.Subscribe(EventAgentStart, func(Event) {
			current := active.Add(1)
			for {
				previous := peak.Load()
				if current <= previous || peak.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
		}); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}

	publishDone := make(chan error, 1)
	go func() {
		publishDone <- bus.Publish(Event{Type: EventAgentStart})
	}()
	for range maxGoroutines {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("handlers did not start")
		}
	}
	if got := peak.Load(); got > maxGoroutines {
		t.Fatalf("peak handlers = %d, want at most %d", got, maxGoroutines)
	}

	close(release)
	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Publish() did not return")
	}
}

func TestHandlerCanCloseBusWithoutWaitingForItself(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	handlerDone := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		bus.Close()
		close(handlerDone)
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case <-handlerDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Close() blocked while waiting for the current handler")
	}
}

func TestHandlerCanCloseBusWhenPublishIsSaturated(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t, WithMaxGoroutines(1))
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		bus.Close()
		close(firstDone)
	}); err != nil {
		t.Fatalf("first Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(secondDone)
	}); err != nil {
		t.Fatalf("second Subscribe() error = %v", err)
	}

	publishDone := make(chan error, 1)
	go func() {
		publishDone <- bus.Publish(Event{Type: EventAgentStart})
	}()

	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("Close() blocked in the first handler while publication was saturated")
	}
	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Publish() did not finish scheduling the accepted snapshot")
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("accepted second handler did not run after Close()")
	}
	select {
	case <-bus.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close after accepted handlers completed")
	}
}

func TestHandlerCanUnsubscribeItself(t *testing.T) {
	t.Run("synchronous publication", func(t *testing.T) {
		bus := newTestBus(t)
		var calls atomic.Int32
		var unsubscribe func()
		var err error
		unsubscribe, err = bus.Subscribe(EventAgentStart, func(Event) {
			calls.Add(1)
			unsubscribe()
		})
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}

		if err := bus.PublishSync(Event{Type: EventAgentStart}); err != nil {
			t.Fatalf("first PublishSync() error = %v", err)
		}
		if err := bus.PublishSync(Event{Type: EventAgentStart}); err != nil {
			t.Fatalf("second PublishSync() error = %v", err)
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("handler call count = %d, want 1", got)
		}
	})

	t.Run("asynchronous publication", func(t *testing.T) {
		bus := newTestBus(t)
		var calls atomic.Int32
		handlerDone := make(chan struct{})
		var unsubscribe func()
		var err error
		unsubscribe, err = bus.Subscribe(EventAgentStart, func(Event) {
			calls.Add(1)
			unsubscribe()
			close(handlerDone)
		})
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}

		if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
		select {
		case <-handlerDone:
		case <-time.After(time.Second):
			t.Fatal("self-unsubscribing handler did not finish")
		}
		if err := bus.PublishSync(Event{Type: EventAgentStart}); err != nil {
			t.Fatalf("PublishSync() after unsubscribe error = %v", err)
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("handler call count = %d, want 1", got)
		}
	})
}

func TestConcurrentCloseAndShutdownAreIdempotent(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(started)
		<-release
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	const callers = 32
	var closeWG sync.WaitGroup
	closeWG.Add(callers)
	for range callers {
		go func() {
			defer closeWG.Done()
			bus.Close()
		}()
	}
	closeCallsDone := make(chan struct{})
	go func() {
		closeWG.Wait()
		close(closeCallsDone)
	}()
	select {
	case <-closeCallsDone:
	case <-time.After(time.Second):
		t.Fatal("concurrent Close() calls blocked on the active handler")
	}
	select {
	case <-bus.Done():
		t.Fatal("Done() closed before the active handler completed")
	default:
	}

	shutdownErrors := make(chan error, callers)
	var shutdownWG sync.WaitGroup
	shutdownWG.Add(callers)
	for range callers {
		go func() {
			defer shutdownWG.Done()
			shutdownErrors <- bus.Shutdown(context.Background())
		}()
	}
	close(release)
	shutdownWG.Wait()
	close(shutdownErrors)
	for err := range shutdownErrors {
		if err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	}
	select {
	case <-bus.Done():
	default:
		t.Fatal("Done() remained open after every active handler completed")
	}
}

func TestShutdownWaitsForActiveHandlersAndHonorsContext(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(started)
		<-release
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := bus.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		close(release)
		t.Fatalf("Shutdown() error = %v, want context deadline exceeded", err)
	}
	close(release)
	if err := bus.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestShutdownWaitsForPublishSyncHandler(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(started)
		<-release
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	publishDone := make(chan error, 1)
	go func() {
		publishDone <- bus.PublishSync(Event{Type: EventAgentStart})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("synchronous handler did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	shutdownErr := bus.Shutdown(ctx)
	close(release)
	if err := <-publishDone; err != nil {
		t.Fatalf("PublishSync() error = %v", err)
	}
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want context deadline exceeded", shutdownErr)
	}
	if err := bus.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestShutdownRejectsNilInputs(t *testing.T) {
	t.Parallel()

	var nilBus *Bus
	if err := nilBus.Shutdown(context.Background()); !errors.Is(err, ErrNilBus) {
		t.Fatalf("nil Bus.Shutdown() error = %v, want %v", err, ErrNilBus)
	}
	bus := newTestBus(t)
	var nilContext context.Context
	if err := bus.Shutdown(nilContext); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Shutdown(nil) error = %v, want %v", err, ErrInvalidContext)
	}
}

func TestShutdownRejectsTypedNilContextWithoutClosingBus(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	var ctx *typedNilContext
	if err := bus.Shutdown(ctx); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Shutdown() error = %v, want %v", err, ErrInvalidContext)
	}
	if err := bus.PublishSync(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("PublishSync() after rejected context error = %v", err)
	}
}

func TestShutdownCompletedBusPrefersDoneOverCanceledContext(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t)
	bus.Close()
	select {
	case <-bus.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 100 {
		if err := bus.Shutdown(ctx); err != nil {
			t.Fatalf("Shutdown() after completed close error = %v, want nil", err)
		}
	}
}
