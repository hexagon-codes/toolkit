package observe

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type timerContextProbe struct {
	observations atomic.Int64
}

func (p *timerContextProbe) ObserveDuration(time.Duration) {
	p.observations.Add(1)
}

func (p *timerContextProbe) Time(fn func()) {
	fn()
}

func (p *timerContextProbe) NewTimer() *TimerContext {
	return NewTimerContext(p)
}

type typedNilSpan struct{}

func (*typedNilSpan) SpanID() string               { return "" }
func (*typedNilSpan) TraceID() string              { return "" }
func (*typedNilSpan) SetName(string)               {}
func (*typedNilSpan) SetInput(any)                 {}
func (*typedNilSpan) SetOutput(any)                {}
func (*typedNilSpan) SetTokenUsage(TokenUsage)     {}
func (*typedNilSpan) SetAttribute(string, any)     {}
func (*typedNilSpan) SetAttributes(map[string]any) {}
func (*typedNilSpan) AddEvent(string, ...any)      {}
func (*typedNilSpan) RecordError(error)            {}
func (*typedNilSpan) SetStatus(StatusCode, string) {}
func (*typedNilSpan) End()                         {}
func (*typedNilSpan) EndWithError(error)           {}
func (*typedNilSpan) IsRecording() bool            { return false }

func TestTimerContextStopRecordsOnceAcrossConcurrentCallers(t *testing.T) {
	probe := &timerContextProbe{}
	timerContext := NewTimerContext(probe)

	const callers = 32
	start := make(chan struct{})
	durations := make(chan time.Duration, callers)
	var waitGroup sync.WaitGroup
	for range callers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			durations <- timerContext.Stop()
		}()
	}
	close(start)
	waitGroup.Wait()
	close(durations)

	var first time.Duration
	for duration := range durations {
		if first == 0 {
			first = duration
			continue
		}
		if duration != first {
			t.Fatalf("Stop() duration = %v, want stable duration %v", duration, first)
		}
	}
	if got := probe.observations.Load(); got != 1 {
		t.Fatalf("ObserveDuration() calls = %d, want 1", got)
	}
}

func TestTimerContextIgnoresTypedNilTimer(t *testing.T) {
	var timer *timerContextProbe
	timerContext := NewTimerContext(timer)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Stop() panicked for typed-nil timer: %v", recovered)
		}
	}()
	if duration := timerContext.Stop(); duration < 0 {
		t.Fatalf("Stop() duration = %v, want non-negative duration", duration)
	}
}

func TestSpanContextTreatsTypedNilAsAbsent(t *testing.T) {
	var span *typedNilSpan
	ctx := ContextWithSpan(context.Background(), span)
	if got := SpanFromContext(ctx); got != nil {
		t.Fatalf("SpanFromContext() = %T, want nil", got)
	}
}
