package event

import (
	"context"
	"errors"
	"log"
	"runtime"
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

type typedNilError struct{}

func (*typedNilError) Error() string { panic("typed nil error used") }

type panickingLogWriter struct{}

func (panickingLogWriter) Write([]byte) (int, error) {
	panic("log writer failed")
}

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

func TestConcurrentPublishAndCloseDrainsEveryAcceptedSnapshot(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(4))
	var handled atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	releaseHandlers := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandlers)
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		startedOnce.Do(func() { close(started) })
		<-release
		handled.Add(1)
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial handler did not start")
	}

	const publishers = 200
	start := make(chan struct{})
	results := make(chan error, publishers)
	var publishersWG sync.WaitGroup
	publishersWG.Add(publishers)
	for range publishers {
		go func() {
			defer publishersWG.Done()
			<-start
			results <- bus.Publish(Event{Type: EventAgentStart})
		}()
	}
	closeDone := make(chan struct{})
	go func() {
		<-start
		bus.Close()
		close(closeDone)
	}()
	close(start)
	publishersWG.Wait()
	<-closeDone
	close(results)

	accepted := int64(1)
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrClosed):
		default:
			t.Fatalf("Publish() error = %v, want nil or %v", err, ErrClosed)
		}
	}
	releaseHandlers()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := handled.Load(); got != accepted {
		t.Fatalf("handler call count = %d, want accepted publication count %d", got, accepted)
	}
}

func TestConcurrentSubscribeUnsubscribeAndPublishPreservesStableSubscriber(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(4))
	var stableCalls atomic.Int64
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		stableCalls.Add(1)
	}); err != nil {
		t.Fatalf("stable Subscribe() error = %v", err)
	}

	const operations = 200
	start := make(chan struct{})
	publishErrors := make(chan error, operations)
	subscribeErrors := make(chan error, operations)
	var wg sync.WaitGroup
	wg.Add(operations * 2)
	for range operations {
		go func() {
			defer wg.Done()
			<-start
			publishErrors <- bus.Publish(Event{Type: EventAgentStart})
		}()
		go func() {
			defer wg.Done()
			<-start
			unsubscribe, err := bus.Subscribe(EventAgentStart, func(Event) {})
			if err == nil {
				unsubscribe()
				unsubscribe()
			}
			subscribeErrors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(publishErrors)
	close(subscribeErrors)
	for err := range publishErrors {
		if err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}
	for err := range subscribeErrors {
		if err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := stableCalls.Load(); got != operations {
		t.Fatalf("stable handler call count = %d, want %d", got, operations)
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
		{name: "zero max pending deliveries", opt: WithMaxPendingDeliveries(0), want: ErrInvalidMaxPendingDeliveries},
		{name: "negative max pending deliveries", opt: WithMaxPendingDeliveries(-1), want: ErrInvalidMaxPendingDeliveries},
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

func TestZeroValueBusSupportsLifecycle(t *testing.T) {
	var bus Bus
	handled := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(handled)
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("zero-value Bus did not deliver the event")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-bus.Done():
	default:
		t.Fatal("Done() remained open after Shutdown()")
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

func TestHandlerPanicRemainsIsolatedWhenLoggerPanics(t *testing.T) {
	originalWriter := log.Writer()
	log.SetOutput(panickingLogWriter{})
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
	})

	bus := newTestBus(t)
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		panic("handler failed")
	}); err != nil {
		t.Fatalf("first Subscribe() error = %v", err)
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
	if !remainingHandlerCalled.Load() {
		t.Fatal("logger panic prevented a remaining handler")
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

func TestPublishRejectsSnapshotExceedingDeliveryCapacityAtomically(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1), WithMaxPendingDeliveries(1))
	var calls atomic.Int32
	for range 3 {
		if _, err := bus.Subscribe(EventAgentStart, func(Event) {
			calls.Add(1)
		}); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}

	err := bus.Publish(Event{Type: EventAgentStart})
	if !errors.Is(err, ErrPendingDeliveryCapacityExceeded) {
		t.Fatalf("Publish() error = %v, want %v", err, ErrPendingDeliveryCapacityExceeded)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("handler call count = %d, want 0 after atomic rejection", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() after rejected Publish error = %v", err)
	}

	bus.dispatchMu.Lock()
	outstanding := bus.outstanding
	pending := len(bus.pending) - bus.pendingHead
	workers := bus.workers
	bus.dispatchMu.Unlock()
	if outstanding != 0 || pending != 0 || workers != 0 {
		t.Fatalf(
			"dispatcher state after rejected Publish: outstanding=%d pending=%d workers=%d",
			outstanding,
			pending,
			workers,
		)
	}
}

func TestDefaultPendingDeliveryCapacityIsBounded(t *testing.T) {
	bus := newTestBus(t)
	if bus.maxGoroutines != 1024 || bus.maxPendingDeliveries != 4096 {
		t.Fatalf(
			"default delivery limits: workers=%d pending=%d, want workers=1024 pending=4096",
			bus.maxGoroutines,
			bus.maxPendingDeliveries,
		)
	}
	for range bus.deliveryCapacity() + 1 {
		if _, err := bus.Subscribe(EventAgentStart, func(Event) {}); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); !errors.Is(err, ErrPendingDeliveryCapacityExceeded) {
		t.Fatalf("Publish() error = %v, want %v", err, ErrPendingDeliveryCapacityExceeded)
	}
}

func TestConcurrentPublishersCannotExceedDeliveryCapacity(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1), WithMaxPendingDeliveries(2))
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	releaseHandlers := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandlers)
	var calls atomic.Int32
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		startedOnce.Do(func() { close(started) })
		<-release
		calls.Add(1)
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	const publishers = 64
	start := make(chan struct{})
	results := make(chan error, publishers)
	var wg sync.WaitGroup
	wg.Add(publishers)
	for range publishers {
		go func() {
			defer wg.Done()
			<-start
			results <- bus.Publish(Event{Type: EventAgentStart})
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	accepted := 0
	rejected := 0
	for err := range results {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrPendingDeliveryCapacityExceeded):
			rejected++
		default:
			t.Fatalf("Publish() error = %v, want nil or %v", err, ErrPendingDeliveryCapacityExceeded)
		}
	}
	if accepted != 2 || rejected != publishers-2 {
		t.Fatalf("publication results: accepted=%d rejected=%d, want accepted=2 rejected=%d", accepted, rejected, publishers-2)
	}

	bus.dispatchMu.Lock()
	outstanding := bus.outstanding
	bus.dispatchMu.Unlock()
	if outstanding != 3 {
		t.Fatalf("outstanding deliveries = %d, want 3", outstanding)
	}
	releaseHandlers()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("handler call count = %d, want 3", got)
	}
}

func TestPendingCapacityRejectionRacingCloseLeavesNoWaitGroupDebt(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1), WithMaxPendingDeliveries(1))
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	releaseHandlers := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandlers)
	var calls atomic.Int32
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		startedOnce.Do(func() { close(started) })
		<-release
		calls.Add(1)
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("queued Publish() error = %v", err)
	}

	start := make(chan struct{})
	publishResult := make(chan error, 1)
	closeDone := make(chan struct{})
	go func() {
		<-start
		publishResult <- bus.Publish(Event{Type: EventAgentStart})
	}()
	go func() {
		<-start
		bus.Close()
		close(closeDone)
	}()
	close(start)
	err := <-publishResult
	<-closeDone
	if !errors.Is(err, ErrPendingDeliveryCapacityExceeded) && !errors.Is(err, ErrClosed) {
		t.Fatalf("racing Publish() error = %v, want %v or %v", err, ErrPendingDeliveryCapacityExceeded, ErrClosed)
	}

	releaseHandlers()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("handler call count = %d, want 2", got)
	}
}

func TestReentrantPublishReturnsCapacityErrorWithoutBlocking(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1), WithMaxPendingDeliveries(1))
	outerStarted := make(chan struct{})
	attemptNested := make(chan struct{})
	var attemptOnce sync.Once
	triggerNested := func() { attemptOnce.Do(func() { close(attemptNested) }) }
	t.Cleanup(triggerNested)
	nestedResult := make(chan error, 1)
	outerReturned := make(chan struct{})
	queuedHandled := make(chan struct{})
	var nestedCalls atomic.Int32
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(outerStarted)
		<-attemptNested
		nestedResult <- bus.Publish(Event{Type: EventAgentError})
		close(outerReturned)
	}); err != nil {
		t.Fatalf("outer Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentEnd, func(Event) {
		close(queuedHandled)
	}); err != nil {
		t.Fatalf("queued Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentError, func(Event) {
		nestedCalls.Add(1)
	}); err != nil {
		t.Fatalf("nested Subscribe() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case <-outerStarted:
	case <-time.After(time.Second):
		t.Fatal("outer handler did not start")
	}
	if err := bus.Publish(Event{Type: EventAgentEnd}); err != nil {
		t.Fatalf("queued Publish() error = %v", err)
	}
	triggerNested()
	select {
	case <-outerReturned:
	case <-time.After(time.Second):
		t.Fatal("reentrant Publish() blocked on a full pending queue")
	}
	if err := <-nestedResult; !errors.Is(err, ErrPendingDeliveryCapacityExceeded) {
		t.Fatalf("reentrant Publish() error = %v, want %v", err, ErrPendingDeliveryCapacityExceeded)
	}
	select {
	case <-queuedHandled:
	case <-time.After(time.Second):
		t.Fatal("previously accepted queued event was not delivered")
	}
	if got := nestedCalls.Load(); got != 0 {
		t.Fatalf("rejected nested handler call count = %d, want 0", got)
	}
}

func TestDispatcherClearsConsumedDeliveryReferences(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1), WithMaxPendingDeliveries(2))
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var firstStartedOnce sync.Once
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	t.Cleanup(releaseHandler)
	if _, err := bus.Subscribe(EventAgentStart, func(event Event) {
		if event.ID == "first" {
			firstStartedOnce.Do(func() { close(firstStarted) })
			<-releaseFirst
		}
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	firstPayload := &struct{ value string }{value: "first"}
	secondPayload := &struct{ value string }{value: "second"}
	if err := bus.Publish(Event{Type: EventAgentStart, ID: "first", Payload: firstPayload}); err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}
	if err := bus.Publish(Event{Type: EventAgentStart, ID: "second", Payload: secondPayload}); err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}

	bus.dispatchMu.Lock()
	consumed := bus.pending[0]
	queued := bus.pending[bus.pendingHead]
	bus.dispatchMu.Unlock()
	if consumed.handler != nil || consumed.event.Payload != nil {
		t.Fatal("consumed queue slot retained handler or payload references")
	}
	if queued.event.Payload != secondPayload {
		t.Fatal("pending queue did not retain the accepted second payload")
	}

	releaseHandler()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	bus.dispatchMu.Lock()
	outstanding := bus.outstanding
	retainsQueue := bus.pending != nil || bus.pendingHead != 0
	bus.dispatchMu.Unlock()
	if outstanding != 0 || retainsQueue {
		t.Fatalf("dispatcher retained state after Shutdown(): outstanding=%d retains_queue=%t", outstanding, retainsQueue)
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

func TestAsyncHandlerCanPublishWhenConcurrencyLimitIsSaturated(t *testing.T) {
	t.Parallel()

	bus := newTestBus(t, WithMaxGoroutines(1))
	innerHandled := make(chan struct{})
	outerReturned := make(chan struct{})
	publishErrors := make(chan error, 1)
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		publishErrors <- bus.Publish(Event{Type: EventAgentEnd})
		close(outerReturned)
	}); err != nil {
		t.Fatalf("outer Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentEnd, func(Event) {
		close(innerHandled)
	}); err != nil {
		t.Fatalf("inner Subscribe() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case <-outerReturned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reentrant Publish() blocked while the concurrency limit was saturated")
	}
	if err := <-publishErrors; err != nil {
		t.Fatalf("reentrant Publish() error = %v", err)
	}
	select {
	case <-innerHandled:
	case <-time.After(time.Second):
		t.Fatal("event published by the handler was not delivered")
	}
}

func TestShutdownDrainsReentrantPublishAcceptedBeforeClose(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1))
	outerRelease := make(chan struct{})
	innerRelease := make(chan struct{})
	var outerReleaseOnce sync.Once
	var innerReleaseOnce sync.Once
	releaseOuter := func() { outerReleaseOnce.Do(func() { close(outerRelease) }) }
	releaseInner := func() { innerReleaseOnce.Do(func() { close(innerRelease) }) }
	t.Cleanup(releaseOuter)
	t.Cleanup(releaseInner)

	nestedAccepted := make(chan error, 1)
	innerStarted := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		nestedAccepted <- bus.Publish(Event{Type: EventAgentEnd})
		<-outerRelease
	}); err != nil {
		t.Fatalf("outer Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentEnd, func(Event) {
		close(innerStarted)
		<-innerRelease
	}); err != nil {
		t.Fatalf("inner Subscribe() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case err := <-nestedAccepted:
		if err != nil {
			t.Fatalf("reentrant Publish() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reentrant Publish() did not enqueue the nested event")
	}

	bus.Close()
	select {
	case <-bus.Done():
		t.Fatal("Done() closed before the accepted handlers completed")
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err := bus.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		cancel()
		t.Fatalf("Shutdown() error = %v, want context deadline exceeded", err)
	}
	cancel()

	releaseOuter()
	select {
	case <-innerStarted:
	case <-time.After(time.Second):
		t.Fatal("nested event accepted before Close() was not delivered")
	}
	select {
	case <-bus.Done():
		t.Fatal("Done() closed while the nested handler was active")
	default:
	}

	releaseInner()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("final Shutdown() error = %v", err)
	}
}

func TestPanicHandlerCanPublishWhenConcurrencyLimitIsSaturated(t *testing.T) {
	var bus *Bus
	panicReturned := make(chan struct{})
	panicPublishErrors := make(chan error, 1)
	bus = newTestBus(t,
		WithMaxGoroutines(1),
		WithPanicHandler(func(Event, any) {
			panicPublishErrors <- bus.Publish(Event{Type: EventAgentEnd})
			close(panicReturned)
		}),
	)
	innerHandled := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		panic("handler failed")
	}); err != nil {
		t.Fatalf("outer Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentEnd, func(Event) {
		close(innerHandled)
	}); err != nil {
		t.Fatalf("inner Subscribe() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("initial Publish() error = %v", err)
	}
	select {
	case <-panicReturned:
	case <-time.After(time.Second):
		t.Fatal("panic handler blocked while publishing a nested event")
	}
	if err := <-panicPublishErrors; err != nil {
		t.Fatalf("panic handler Publish() error = %v", err)
	}
	select {
	case <-innerHandled:
	case <-time.After(time.Second):
		t.Fatal("event published by the panic handler was not delivered")
	}
}

func TestWorkerIsReplacedWhenHandlerCallsGoexit(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1))
	remainingHandlerCalled := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		runtime.Goexit()
	}); err != nil {
		t.Fatalf("first Subscribe() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(remainingHandlerCalled)
	}); err != nil {
		t.Fatalf("second Subscribe() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	select {
	case <-remainingHandlerCalled:
	case <-time.After(time.Second):
		t.Fatal("worker exit stranded a queued handler")
	}
}

func TestPublishSyncContinuesAfterHandlerCallsGoexit(t *testing.T) {
	bus, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		runtime.Goexit()
	}); err != nil {
		t.Fatalf("first Subscribe() error = %v", err)
	}
	remainingHandlerCalled := make(chan struct{})
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		close(remainingHandlerCalled)
	}); err != nil {
		t.Fatalf("second Subscribe() error = %v", err)
	}

	publishResult := make(chan error, 1)
	go func() {
		publishResult <- bus.PublishSync(Event{Type: EventAgentStart})
	}()
	select {
	case err := <-publishResult:
		if err != nil {
			t.Fatalf("PublishSync() error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("PublishSync() did not survive handler Goexit")
	}
	select {
	case <-remainingHandlerCalled:
	default:
		t.Fatal("handler Goexit stranded the remaining synchronous handler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestShutdownReleasesQueuedDeliveriesAndWorkers(t *testing.T) {
	bus := newTestBus(t, WithMaxGoroutines(1))
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	if _, err := bus.Subscribe(EventAgentStart, func(Event) {
		startedOnce.Do(func() { close(started) })
		<-release
	}); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("first Publish() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	for range 2048 {
		if err := bus.Publish(Event{Type: EventAgentStart}); err != nil {
			t.Fatalf("queued Publish() error = %v", err)
		}
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		bus.dispatchMu.Lock()
		workers := bus.workers
		outstanding := bus.outstanding
		pending := len(bus.pending) - bus.pendingHead
		retainsQueue := bus.pending != nil || bus.pendingHead != 0
		bus.dispatchMu.Unlock()
		if workers == 0 && outstanding == 0 && pending == 0 && !retainsQueue {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"dispatcher state after Shutdown(): workers=%d outstanding=%d pending=%d retains_queue=%t",
				workers,
				outstanding,
				pending,
				retainsQueue,
			)
		}
		runtime.Gosched()
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

func TestShutdownPreservesCancellationCause(t *testing.T) {
	bus := newTestBus(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandler)
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

	cause := errors.New("caller canceled shutdown")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := bus.Shutdown(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want context canceled", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Shutdown() error = %v, want cancellation cause %v", err, cause)
	}

	releaseHandler()
	if err := bus.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown() error = %v", err)
	}
}

func TestShutdownIgnoresTypedNilCancellationCause(t *testing.T) {
	bus := newTestBus(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHandler)
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

	ctx, cancel := context.WithCancelCause(context.Background())
	var cause *typedNilError
	cancel(cause)
	err := bus.Shutdown(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want context canceled", err)
	}
	if got := err.Error(); got != context.Canceled.Error() {
		t.Fatalf("Shutdown() error text = %q, want %q", got, context.Canceled.Error())
	}

	releaseHandler()
	if err := bus.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown() error = %v", err)
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
