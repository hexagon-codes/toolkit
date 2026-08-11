package otel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hexagon-codes/toolkit/infra/observe"
)

type lifecycleProbeExporter struct {
	exportCalls   atomic.Int32
	shutdownCalls atomic.Int32
	shutdownErr   error
}

type startTimeCaptureExporter struct {
	spans chan *SpanData
}

func (e *startTimeCaptureExporter) ExportSpans(_ context.Context, spans []*SpanData) error {
	for _, span := range spans {
		e.spans <- span
	}
	return nil
}

func (e *startTimeCaptureExporter) Shutdown(context.Context) error {
	return nil
}

type blockingLifecycleExporter struct {
	exportStarted        chan struct{}
	releaseExport        chan struct{}
	exportStartedOnce    sync.Once
	inFlightExports      atomic.Int32
	shutdownCalls        atomic.Int32
	shutdownDuringExport atomic.Bool
	shutdownFinished     chan struct{}
	shutdownFinishedOnce sync.Once
}

type blockingShutdownLifecycleExporter struct {
	shutdownStarted chan struct{}
	releaseShutdown chan struct{}
	startedOnce     sync.Once
	shutdownCalls   atomic.Int32
	shutdownErr     error
}

type sequencedLifecycleExporter struct {
	exportCalls       atomic.Int32
	blockedStarted    chan struct{}
	releaseBlocked    chan struct{}
	blockedStartedOne sync.Once
	firstExportErr    error
	blockedExportErr  error
	shutdownErr       error
}

type finalizingLifecycleExporter struct {
	exportStarted chan []*SpanData
	releaseExport chan struct{}
	exportCalls   atomic.Int32
	shutdownCalls atomic.Int32
	exportErr     error
	shutdownErr   error
}

type shutdownCancelableLifecycleExporter struct {
	exportStarted chan struct{}
	exportDone    chan struct{}
	stopExport    chan struct{}
	startOnce     sync.Once
	doneOnce      sync.Once
	stopOnce      sync.Once
	shutdownCalls atomic.Int32
}

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func (e *blockingLifecycleExporter) ExportSpans(context.Context, []*SpanData) error {
	e.inFlightExports.Add(1)
	e.exportStartedOnce.Do(func() { close(e.exportStarted) })
	<-e.releaseExport
	e.inFlightExports.Add(-1)
	return nil
}

func (e *blockingLifecycleExporter) Shutdown(context.Context) error {
	if e.inFlightExports.Load() != 0 {
		e.shutdownDuringExport.Store(true)
	}
	e.shutdownCalls.Add(1)
	if e.shutdownFinished != nil {
		e.shutdownFinishedOnce.Do(func() { close(e.shutdownFinished) })
	}
	return nil
}

func (e *blockingShutdownLifecycleExporter) ExportSpans(context.Context, []*SpanData) error {
	return nil
}

func (e *blockingShutdownLifecycleExporter) Shutdown(context.Context) error {
	e.shutdownCalls.Add(1)
	e.startedOnce.Do(func() { close(e.shutdownStarted) })
	<-e.releaseShutdown
	return e.shutdownErr
}

func (e *sequencedLifecycleExporter) ExportSpans(context.Context, []*SpanData) error {
	if e.exportCalls.Add(1) == 1 {
		return e.firstExportErr
	}
	e.blockedStartedOne.Do(func() { close(e.blockedStarted) })
	<-e.releaseBlocked
	return e.blockedExportErr
}

func (e *sequencedLifecycleExporter) Shutdown(context.Context) error {
	return e.shutdownErr
}

func (e *finalizingLifecycleExporter) ExportSpans(_ context.Context, spans []*SpanData) error {
	e.exportCalls.Add(1)
	e.exportStarted <- spans
	<-e.releaseExport
	return e.exportErr
}

func (e *finalizingLifecycleExporter) Shutdown(context.Context) error {
	e.shutdownCalls.Add(1)
	return e.shutdownErr
}

func (e *shutdownCancelableLifecycleExporter) ExportSpans(context.Context, []*SpanData) error {
	e.startOnce.Do(func() {
		close(e.exportStarted)
		go func() {
			<-e.stopExport
			e.doneOnce.Do(func() { close(e.exportDone) })
		}()
	})
	return nil
}

func (e *shutdownCancelableLifecycleExporter) Shutdown(ctx context.Context) error {
	e.shutdownCalls.Add(1)
	select {
	case <-e.exportDone:
		return nil
	case <-ctx.Done():
		e.stopOnce.Do(func() { close(e.stopExport) })
		<-e.exportDone
		return ctx.Err()
	}
}

func (e *lifecycleProbeExporter) ExportSpans(context.Context, []*SpanData) error {
	e.exportCalls.Add(1)
	return nil
}

func (e *lifecycleProbeExporter) Shutdown(context.Context) error {
	e.shutdownCalls.Add(1)
	return e.shutdownErr
}

func mustSetExporter(t *testing.T, tracer *Tracer, exporter Exporter) {
	t.Helper()
	if err := tracer.SetExporter(context.Background(), exporter); err != nil {
		t.Fatalf("SetExporter() error = %v", err)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}

// TestTracerStartSpanUsesConfiguredStartTime 锁定调用方提供的非零开始时间。
func TestTracerStartSpanUsesConfiguredStartTime(t *testing.T) {
	fixed := time.Date(2026, time.August, 11, 9, 8, 7, 654321000, time.UTC)
	exporter := &startTimeCaptureExporter{spans: make(chan *SpanData, 1)}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)
	t.Cleanup(func() {
		_ = tracer.Shutdown(context.Background())
	})

	_, span := tracer.StartSpan(context.Background(), "fixed-start", observe.WithStartTime(fixed))
	span.End()

	select {
	case exported := <-exporter.spans:
		if !exported.StartTime.Equal(fixed) {
			t.Fatalf("exported StartTime = %s, want %s", exported.StartTime.Format(time.RFC3339Nano), fixed.Format(time.RFC3339Nano))
		}
	case <-time.After(time.Second):
		t.Fatal("span was not exported")
	}
}

// TestTracerStartSpanUsesCurrentTimeWhenUnset 锁定零值开始时间的当前时间回退。
func TestTracerStartSpanUsesCurrentTimeWhenUnset(t *testing.T) {
	exporter := &startTimeCaptureExporter{spans: make(chan *SpanData, 1)}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)
	t.Cleanup(func() {
		_ = tracer.Shutdown(context.Background())
	})

	before := time.Now()
	_, span := tracer.StartSpan(context.Background(), "current-start")
	span.End()
	after := time.Now()

	select {
	case exported := <-exporter.spans:
		if exported.StartTime.Before(before) || exported.StartTime.After(after) {
			t.Fatalf("exported StartTime = %s, want within [%s, %s]", exported.StartTime.Format(time.RFC3339Nano), before.Format(time.RFC3339Nano), after.Format(time.RFC3339Nano))
		}
	case <-time.After(time.Second):
		t.Fatal("span was not exported")
	}
}

func TestTracerSetExporterShutsDownPreviousOTLPExporter(t *testing.T) {
	tracer := NewTracer()
	previous := mustNewOTLPExporter(t, "http://127.0.0.1")
	next := &lifecycleProbeExporter{}
	t.Cleanup(func() {
		_ = tracer.Shutdown(context.Background())
	})

	mustSetExporter(t, tracer, previous)
	mustSetExporter(t, tracer, next)

	select {
	case <-previous.loopDone:
	case <-time.After(time.Second):
		t.Fatal("previous OTLP exporter background loop is still running after replacement")
	}
}

func TestTracerShutdownIsIdempotent(t *testing.T) {
	tracer := NewTracer()
	exporter := &lifecycleProbeExporter{}
	mustSetExporter(t, tracer, exporter)

	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if got := exporter.shutdownCalls.Load(); got != 1 {
		t.Fatalf("exporter Shutdown() calls = %d, want 1", got)
	}
}

// TestTracerConcurrentShutdownWaitsAndRemembersError 锁定单次关闭、并发等待与错误记忆。
func TestTracerConcurrentShutdownWaitsAndRemembersError(t *testing.T) {
	sentinel := errors.New("exporter shutdown failed")
	exporter := &blockingShutdownLifecycleExporter{
		shutdownStarted: make(chan struct{}),
		releaseShutdown: make(chan struct{}),
		shutdownErr:     sentinel,
	}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- tracer.Shutdown(context.Background())
	}()
	select {
	case <-exporter.shutdownStarted:
	case <-time.After(time.Second):
		t.Fatal("first Shutdown() did not reach exporter")
	}

	secondDone := make(chan error, 1)
	secondWait := &observedDoneContext{
		Context:  context.Background(),
		observed: make(chan struct{}),
	}
	go func() {
		secondDone <- tracer.Shutdown(secondWait)
	}()
	waitForSignal(t, secondWait.observed, "concurrent Shutdown() did not enter the shared wait")
	select {
	case <-secondDone:
		t.Fatal("concurrent Shutdown() returned before the shared shutdown completed")
	default:
	}
	close(exporter.releaseShutdown)

	firstErr := <-firstDone
	secondErr := <-secondDone
	if !errors.Is(firstErr, sentinel) || !errors.Is(secondErr, sentinel) {
		t.Fatalf("concurrent Shutdown() errors = %v / %v, want sentinel", firstErr, secondErr)
	}
	if err := tracer.Shutdown(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("repeated Shutdown() error = %v, want sentinel", err)
	}
	if got := exporter.shutdownCalls.Load(); got != 1 {
		t.Fatalf("exporter Shutdown() calls = %d, want 1", got)
	}
}

func TestTracerSetExporterInstallsReplacementWhenPreviousShutdownFails(t *testing.T) {
	sentinel := errors.New("previous shutdown failed")
	tracer := NewTracer()
	previous := &lifecycleProbeExporter{shutdownErr: sentinel}
	next := &lifecycleProbeExporter{}
	mustSetExporter(t, tracer, previous)

	err := tracer.SetExporter(context.Background(), next)
	if !errors.Is(err, sentinel) {
		t.Fatalf("SetExporter() error = %v, want previous shutdown error", err)
	}
	_, span := tracer.StartSpan(context.Background(), "replacement")
	span.End()
	if got := next.exportCalls.Load(); got != 1 {
		t.Fatalf("replacement ExportSpans() calls = %d, want 1", got)
	}
	if err := tracer.Shutdown(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("Shutdown() error = %v, want previous shutdown error in history", err)
	}
}

func TestTracerSetExporterAfterShutdownReleasesRejectedExporter(t *testing.T) {
	tracer := NewTracer()
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	rejected := &lifecycleProbeExporter{}
	err := tracer.SetExporter(context.Background(), rejected)
	if !errors.Is(err, ErrTracerShutdown) {
		t.Fatalf("SetExporter() error = %v, want ErrTracerShutdown", err)
	}
	if got := rejected.shutdownCalls.Load(); got != 1 {
		t.Fatalf("rejected exporter Shutdown() calls = %d, want 1", got)
	}
}

func TestTracerSetExporterAfterShutdownStopsRejectedOTLPExporter(t *testing.T) {
	tracer := NewTracer()
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	rejected := mustNewOTLPExporter(t, "http://127.0.0.1")
	if err := tracer.SetExporter(context.Background(), rejected); !errors.Is(err, ErrTracerShutdown) {
		t.Fatalf("SetExporter() error = %v, want ErrTracerShutdown", err)
	}
	select {
	case <-rejected.loopDone:
	case <-time.After(time.Second):
		t.Fatal("rejected OTLP exporter background loop is still running")
	}
}

func TestTracerSetExporterTreatsTypedNilAsNoExporter(t *testing.T) {
	tracer := NewTracer()
	previous := &lifecycleProbeExporter{}
	mustSetExporter(t, tracer, previous)

	var none *lifecycleProbeExporter
	if err := tracer.SetExporter(context.Background(), none); err != nil {
		t.Fatalf("SetExporter() error = %v", err)
	}
	if got := previous.shutdownCalls.Load(); got != 1 {
		t.Fatalf("previous exporter Shutdown() calls = %d, want 1", got)
	}
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestTracerSetExporterWaitsForInFlightExportBeforeShutdown(t *testing.T) {
	tracer := NewTracer()
	previous := &blockingLifecycleExporter{
		exportStarted: make(chan struct{}),
		releaseExport: make(chan struct{}),
	}
	next := &lifecycleProbeExporter{}
	mustSetExporter(t, tracer, previous)

	_, span := tracer.StartSpan(context.Background(), "in-flight")
	endDone := make(chan struct{})
	go func() {
		span.End()
		close(endDone)
	}()
	select {
	case <-previous.exportStarted:
	case <-time.After(time.Second):
		t.Fatal("span export did not start")
	}

	replaceDone := make(chan error, 1)
	replaceWait := &observedDoneContext{
		Context:  context.Background(),
		observed: make(chan struct{}),
	}
	go func() {
		replaceDone <- tracer.SetExporter(replaceWait, next)
	}()
	waitForSignal(t, replaceWait.observed, "SetExporter() did not enter the generation drain wait")
	select {
	case <-replaceDone:
		t.Fatal("SetExporter() returned before the in-flight export completed")
	default:
	}
	close(previous.releaseExport)
	<-endDone
	if err := <-replaceDone; err != nil {
		t.Fatalf("SetExporter() error = %v", err)
	}
	if previous.shutdownDuringExport.Load() {
		t.Fatal("previous exporter was shut down while ExportSpans() was in flight")
	}
	if got := previous.shutdownCalls.Load(); got != 1 {
		t.Fatalf("previous exporter Shutdown() calls = %d, want 1", got)
	}
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

// TestTracerBlockedExportAllowsCancelableReplacement 锁定替换等待可取消且旧代际继续完成释放。
func TestTracerBlockedExportAllowsCancelableReplacement(t *testing.T) {
	previous := &blockingLifecycleExporter{
		exportStarted:    make(chan struct{}),
		releaseExport:    make(chan struct{}),
		shutdownFinished: make(chan struct{}),
	}
	next := &lifecycleProbeExporter{}
	tracer := NewTracer()
	mustSetExporter(t, tracer, previous)

	_, blockedSpan := tracer.StartSpan(context.Background(), "blocked-replacement")
	endDone := make(chan struct{})
	go func() {
		blockedSpan.End()
		close(endDone)
	}()
	waitForSignal(t, previous.exportStarted, "span export did not start")

	baseContext, cancel := context.WithCancel(context.Background())
	replaceWait := &observedDoneContext{
		Context:  baseContext,
		observed: make(chan struct{}),
	}
	replaceDone := make(chan error, 1)
	go func() {
		replaceDone <- tracer.SetExporter(replaceWait, next)
	}()
	waitForSignal(t, replaceWait.observed, "SetExporter() did not enter the cancelable generation drain wait")
	cancel()
	select {
	case err := <-replaceDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SetExporter() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		close(previous.releaseExport)
		t.Fatal("SetExporter() did not return after cancellation")
	}

	_, replacementSpan := tracer.StartSpan(context.Background(), "replacement-active")
	replacementSpan.End()
	if got := next.exportCalls.Load(); got != 1 {
		close(previous.releaseExport)
		t.Fatalf("replacement ExportSpans() calls = %d, want 1", got)
	}

	close(previous.releaseExport)
	waitForSignal(t, endDone, "blocked Span.End() did not return after release")
	waitForSignal(t, previous.shutdownFinished, "retired exporter was not shut down after its export drained")
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

// TestTracerCanceledReplacementsAreReapedAndErrorsRetained 锁定取消换代后的确定性摘除与有界错误留存。
func TestTracerCanceledReplacementsAreReapedAndErrorsRetained(t *testing.T) {
	retirementErrors := []error{
		errors.New("retirement one failed"),
		errors.New("retirement two failed"),
		errors.New("retirement three failed"),
		errors.New("retirement four failed"),
		errors.New("current retirement failed"),
	}
	exporters := make([]*blockingShutdownLifecycleExporter, len(retirementErrors))
	releaseOnce := make([]sync.Once, len(retirementErrors))
	release := make([]func(), len(retirementErrors))
	for index, retirementErr := range retirementErrors {
		exporters[index] = &blockingShutdownLifecycleExporter{
			shutdownStarted: make(chan struct{}),
			releaseShutdown: make(chan struct{}),
			shutdownErr:     retirementErr,
		}
		index := index
		release[index] = func() {
			releaseOnce[index].Do(func() { close(exporters[index].releaseShutdown) })
		}
	}

	tracer := NewTracer()
	defer func() {
		for _, releaseExporter := range release {
			releaseExporter()
		}
		_ = tracer.Shutdown(context.Background())
	}()
	mustSetExporter(t, tracer, exporters[0])

	for index := 0; index < len(exporters)-1; index++ {
		retiringGeneration := tracer.generation
		replaceContext, cancelReplace := context.WithCancel(context.Background())
		replaceDone := make(chan error, 1)
		go func(next Exporter) {
			replaceDone <- tracer.SetExporter(replaceContext, next)
		}(exporters[index+1])

		waitForSignal(t, exporters[index].shutdownStarted, "retired exporter shutdown did not start")
		cancelReplace()
		select {
		case err := <-replaceDone:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("SetExporter() error = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("SetExporter() did not return after cancellation")
		}

		release[index]()
		waitForSignal(t, retiringGeneration.retireDone, "retired generation did not finish")
	}

	tracer.mu.RLock()
	registrySize := len(tracer.retirements)
	tracer.mu.RUnlock()
	if registrySize != 0 {
		t.Errorf("retirement registry size = %d, want 0", registrySize)
	}

	release[len(release)-1]()
	shutdownErr := tracer.Shutdown(context.Background())
	for _, expected := range retirementErrors {
		if !errors.Is(shutdownErr, expected) {
			t.Errorf("Shutdown() error = %v, want %v in retirement history", shutdownErr, expected)
		}
	}
	tracer.mu.RLock()
	registrySize = len(tracer.retirements)
	tracer.mu.RUnlock()
	if registrySize != 0 {
		t.Errorf("retirement registry size after Shutdown = %d, want 0", registrySize)
	}
}

// TestRetirementErrorStateBoundsRetainedErrors 锁定历史错误状态只保留固定数量的错误对象。
func TestRetirementErrorStateBoundsRetainedErrors(t *testing.T) {
	var state retirementErrorState
	sentinels := make([]error, maxRetirementErrors+3)
	for index := range sentinels {
		sentinels[index] = errors.New("retirement failed")
		state.add(sentinels[index])
	}

	if got := len(state.retained); got != maxRetirementErrors {
		t.Fatalf("retained retirement errors = %d, want %d", got, maxRetirementErrors)
	}
	if got := state.failures; got != uint64(len(sentinels)) {
		t.Fatalf("retirement failure count = %d, want %d", got, len(sentinels))
	}
	snapshot := state.snapshot()
	if !errors.Is(snapshot, sentinels[0]) || !errors.Is(snapshot, sentinels[maxRetirementErrors-1]) {
		t.Fatal("retirement error snapshot lost a retained error")
	}
	if errors.Is(snapshot, sentinels[maxRetirementErrors]) {
		t.Fatal("retirement error snapshot retained an error beyond its bound")
	}
}

// TestTracerShutdownDeadlineWhileExportBlockedPreservesKnownErrors 锁定关闭超时与既有导出错误链。
func TestTracerShutdownDeadlineWhileExportBlockedPreservesKnownErrors(t *testing.T) {
	firstExportErr := errors.New("first export failed")
	exporter := &sequencedLifecycleExporter{
		blockedStarted:   make(chan struct{}),
		releaseBlocked:   make(chan struct{}),
		firstExportErr:   firstExportErr,
		blockedExportErr: errors.New("blocked export failed"),
	}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)

	_, failedSpan := tracer.StartSpan(context.Background(), "known-failure")
	failedSpan.End()
	_, blockedSpan := tracer.StartSpan(context.Background(), "blocked-shutdown")
	endDone := make(chan struct{})
	go func() {
		blockedSpan.End()
		close(endDone)
	}()
	waitForSignal(t, exporter.blockedStarted, "blocked export did not start")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- tracer.Shutdown(shutdownContext)
	}()

	var shutdownErr error
	select {
	case shutdownErr = <-shutdownDone:
	case <-time.After(time.Second):
		close(exporter.releaseBlocked)
		<-endDone
		<-shutdownDone
		t.Fatal("Shutdown() did not honor its deadline while an export was blocked")
	}
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		close(exporter.releaseBlocked)
		<-endDone
		t.Fatalf("Shutdown() error = %v, want context.DeadlineExceeded", shutdownErr)
	}
	if !errors.Is(shutdownErr, firstExportErr) {
		close(exporter.releaseBlocked)
		<-endDone
		t.Fatalf("Shutdown() error = %v, want first export error in chain", shutdownErr)
	}

	close(exporter.releaseBlocked)
	waitForSignal(t, endDone, "blocked Span.End() did not return after release")
	_ = tracer.Shutdown(context.Background())
}

// TestTracerShutdownDeadlineCancelsExporterOwnedWorkAfterGenerationDrain 锁定排空代际后由截止时间取消导出器自有任务。
func TestTracerShutdownDeadlineCancelsExporterOwnedWorkAfterGenerationDrain(t *testing.T) {
	exporter := &shutdownCancelableLifecycleExporter{
		exportStarted: make(chan struct{}),
		exportDone:    make(chan struct{}),
		stopExport:    make(chan struct{}),
	}
	t.Cleanup(func() {
		exporter.stopOnce.Do(func() { close(exporter.stopExport) })
	})

	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)
	_, span := tracer.StartSpan(context.Background(), "cancelable-in-flight")
	endDone := make(chan struct{})
	go func() {
		span.End()
		close(endDone)
	}()
	waitForSignal(t, exporter.exportStarted, "span export did not start")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := tracer.Shutdown(shutdownContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want context.DeadlineExceeded", err)
	}
	waitForSignal(t, exporter.exportDone, "shutdown deadline did not cancel exporter-owned work")
	waitForSignal(t, endDone, "Span.End() did not return after exporter cancellation")

	finalDone := make(chan error, 1)
	go func() {
		finalDone <- tracer.Shutdown(context.Background())
	}()
	select {
	case err := <-finalDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("final Shutdown() error = %v, want remembered deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("final Shutdown() did not converge after exporter cancellation")
	}
	if got := exporter.shutdownCalls.Load(); got != 1 {
		t.Fatalf("exporter Shutdown() calls = %d, want 1", got)
	}
}

// TestTracerShutdownDeadlineDoesNotBypassGenerationDrain 锁定关闭截止时间不能绕过同步导出的代际排空。
func TestTracerShutdownDeadlineDoesNotBypassGenerationDrain(t *testing.T) {
	exporter := &blockingLifecycleExporter{
		exportStarted:    make(chan struct{}),
		releaseExport:    make(chan struct{}),
		shutdownFinished: make(chan struct{}),
	}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)

	_, span := tracer.StartSpan(context.Background(), "drain-before-shutdown")
	endDone := make(chan struct{})
	go func() {
		span.End()
		close(endDone)
	}()
	waitForSignal(t, exporter.exportStarted, "span export did not start")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := tracer.Shutdown(shutdownContext); !errors.Is(err, context.DeadlineExceeded) {
		close(exporter.releaseExport)
		t.Fatalf("Shutdown() error = %v, want context.DeadlineExceeded", err)
	}
	if got := exporter.shutdownCalls.Load(); got != 0 {
		close(exporter.releaseExport)
		t.Fatalf("exporter Shutdown() calls before drain = %d, want 0", got)
	}
	if exporter.shutdownDuringExport.Load() {
		close(exporter.releaseExport)
		t.Fatal("exporter was shut down while ExportSpans() was in flight")
	}

	close(exporter.releaseExport)
	waitForSignal(t, endDone, "Span.End() did not finish after export release")
	waitForSignal(t, exporter.shutdownFinished, "exporter was not shut down after generation drain")
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown() error = %v", err)
	}
	if got := exporter.shutdownCalls.Load(); got != 1 {
		t.Fatalf("exporter Shutdown() calls = %d, want 1", got)
	}
}

// TestTracerShutdownDeadlineFlushesActiveSpansBeforeExporterShutdown 锁定最终 Span 刷新先于导出器关闭。
func TestTracerShutdownDeadlineFlushesActiveSpansBeforeExporterShutdown(t *testing.T) {
	shutdownErr := errors.New("final exporter shutdown failed")
	exporter := &finalizingLifecycleExporter{
		exportStarted: make(chan []*SpanData, 1),
		releaseExport: make(chan struct{}),
		shutdownErr:   shutdownErr,
	}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)
	_, _ = tracer.StartSpan(context.Background(), "active-at-shutdown")

	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- tracer.Shutdown(shutdownContext)
	}()
	select {
	case spans := <-exporter.exportStarted:
		if len(spans) != 1 {
			close(exporter.releaseExport)
			t.Fatalf("final span count = %d, want 1", len(spans))
		}
	case <-time.After(time.Second):
		t.Fatal("final span flush did not start")
	}
	if err := <-shutdownDone; !errors.Is(err, context.DeadlineExceeded) {
		close(exporter.releaseExport)
		t.Fatalf("Shutdown() error = %v, want context.DeadlineExceeded", err)
	}
	if got := exporter.shutdownCalls.Load(); got != 0 {
		close(exporter.releaseExport)
		t.Fatalf("exporter Shutdown() calls before final flush = %d, want 0", got)
	}

	close(exporter.releaseExport)
	finalDone := make(chan error, 1)
	go func() {
		finalDone <- tracer.Shutdown(context.Background())
	}()
	select {
	case err := <-finalDone:
		if !errors.Is(err, shutdownErr) {
			t.Fatalf("final Shutdown() error = %v, want exporter shutdown error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("final Shutdown() did not converge after final span flush")
	}
	if got := exporter.shutdownCalls.Load(); got != 1 {
		t.Fatalf("exporter Shutdown() calls = %d, want 1", got)
	}
}

// TestTracerShutdownDeadlineIncludesCompletedRetirementErrors 锁定超时时全部已知代际错误仍保留在错误链中。
func TestTracerShutdownDeadlineIncludesCompletedRetirementErrors(t *testing.T) {
	retiredErr := errors.New("retired exporter shutdown failed")
	currentErr := errors.New("current exporter shutdown failed")
	retired := &blockingShutdownLifecycleExporter{
		shutdownStarted: make(chan struct{}),
		releaseShutdown: make(chan struct{}),
		shutdownErr:     retiredErr,
	}
	current := &blockingShutdownLifecycleExporter{
		shutdownStarted: make(chan struct{}),
		releaseShutdown: make(chan struct{}),
		shutdownErr:     currentErr,
	}
	var retiredReleaseOnce sync.Once
	var currentReleaseOnce sync.Once
	releaseRetired := func() { retiredReleaseOnce.Do(func() { close(retired.releaseShutdown) }) }
	releaseCurrent := func() { currentReleaseOnce.Do(func() { close(current.releaseShutdown) }) }
	tracer := NewTracer()
	defer func() {
		releaseRetired()
		releaseCurrent()
		_ = tracer.Shutdown(context.Background())
	}()
	mustSetExporter(t, tracer, retired)
	retiredGeneration := tracer.generation

	replaceContext, cancelReplace := context.WithCancel(context.Background())
	replaceDone := make(chan error, 1)
	go func() {
		replaceDone <- tracer.SetExporter(replaceContext, current)
	}()
	waitForSignal(t, retired.shutdownStarted, "retired exporter shutdown did not start")
	cancelReplace()
	if err := <-replaceDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("SetExporter() error = %v, want context.Canceled", err)
	}
	releaseRetired()
	waitForSignal(t, retiredGeneration.retireDone, "retired exporter shutdown did not finish")

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelShutdown()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- tracer.Shutdown(shutdownContext)
	}()
	waitForSignal(t, current.shutdownStarted, "current exporter shutdown did not start")

	var shutdownErr error
	select {
	case shutdownErr = <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("Shutdown() did not honor its deadline while exporter shutdown was blocked")
	}
	if !errors.Is(shutdownErr, context.DeadlineExceeded) || !errors.Is(shutdownErr, retiredErr) {
		t.Fatalf("Shutdown() error = %v, want deadline and completed retirement error", shutdownErr)
	}

	releaseCurrent()
	finalErr := tracer.Shutdown(context.Background())
	if !errors.Is(finalErr, retiredErr) || !errors.Is(finalErr, currentErr) {
		t.Fatalf("final Shutdown() error = %v, want both exporter errors", finalErr)
	}
}

// TestTracerShutdownFinalizesAndClearsActiveSpans 锁定关闭时的原子终止、清空与后续 End 幂等。
func TestTracerShutdownFinalizesAndClearsActiveSpans(t *testing.T) {
	exporter := &finalizingLifecycleExporter{
		exportStarted: make(chan []*SpanData, 1),
		releaseExport: make(chan struct{}),
	}
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)

	_, first := tracer.StartSpan(context.Background(), "first-active")
	_, second := tracer.StartSpan(context.Background(), "second-active")
	firstSpan := first.(*Span)
	secondSpan := second.(*Span)

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- tracer.Shutdown(context.Background())
	}()

	var exported []*SpanData
	select {
	case exported = <-exporter.exportStarted:
	case <-time.After(time.Second):
		close(exporter.releaseExport)
		t.Fatal("Shutdown() did not flush active spans")
	}

	activeCount := 0
	tracer.spans.Range(func(_, _ any) bool {
		activeCount++
		return true
	})
	if activeCount != 0 {
		close(exporter.releaseExport)
		<-shutdownDone
		t.Fatalf("active span count = %d, want 0", activeCount)
	}
	if len(exported) != 2 {
		close(exporter.releaseExport)
		<-shutdownDone
		t.Fatalf("exported span count = %d, want 2", len(exported))
	}

	byID := make(map[string]*SpanData, len(exported))
	for _, span := range exported {
		byID[span.SpanID] = span
		if span.EndTime.IsZero() || span.EndTime.Before(span.StartTime) {
			close(exporter.releaseExport)
			<-shutdownDone
			t.Fatalf("exported span %q has invalid EndTime %s", span.Name, span.EndTime.Format(time.RFC3339Nano))
		}
	}
	firstData, firstFound := byID[first.SpanID()]
	secondData, secondFound := byID[second.SpanID()]
	if !firstFound || !secondFound {
		close(exporter.releaseExport)
		<-shutdownDone
		t.Fatalf("exported span IDs do not match active spans: first=%v second=%v", firstFound, secondFound)
	}
	firstEnd := firstData.EndTime
	secondEnd := secondData.EndTime
	if !firstEnd.Equal(secondEnd) {
		close(exporter.releaseExport)
		<-shutdownDone
		t.Fatalf("shutdown EndTime values differ: %s / %s", firstEnd.Format(time.RFC3339Nano), secondEnd.Format(time.RFC3339Nano))
	}

	first.End()
	second.End()
	if got := exporter.exportCalls.Load(); got != 1 {
		close(exporter.releaseExport)
		<-shutdownDone
		t.Fatalf("ExportSpans() calls after repeated End = %d, want 1", got)
	}
	firstSpan.mu.RLock()
	firstEndAfter := firstSpan.endTime
	firstEnded := firstSpan.ended
	firstSpan.mu.RUnlock()
	secondSpan.mu.RLock()
	secondEndAfter := secondSpan.endTime
	secondEnded := secondSpan.ended
	secondSpan.mu.RUnlock()
	if !firstEnded || !secondEnded || !firstEndAfter.Equal(firstEnd) || !secondEndAfter.Equal(secondEnd) {
		close(exporter.releaseExport)
		<-shutdownDone
		t.Fatal("End() changed spans already finalized by Shutdown()")
	}

	close(exporter.releaseExport)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if got := exporter.shutdownCalls.Load(); got != 1 {
		t.Fatalf("exporter Shutdown() calls = %d, want 1", got)
	}
}

// TestTracerShutdownConvergesUnexportedActiveSpans 锁定无可用导出器时也会终止并清空活跃 Span。
func TestTracerShutdownConvergesUnexportedActiveSpans(t *testing.T) {
	tracer := NewTracer(WithSamplingRate(0))
	_, span := tracer.StartSpan(context.Background(), "unexported-active")
	concrete := span.(*Span)

	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	activeCount := 0
	tracer.spans.Range(func(_, _ any) bool {
		activeCount++
		return true
	})
	if activeCount != 0 {
		t.Fatalf("active span count = %d, want 0", activeCount)
	}
	concrete.mu.RLock()
	ended := concrete.ended
	endTime := concrete.endTime
	startTime := concrete.startTime
	concrete.mu.RUnlock()
	if !ended || endTime.IsZero() || endTime.Before(startTime) {
		t.Fatalf("unexported span did not converge: ended=%v start=%s end=%s", ended, startTime.Format(time.RFC3339Nano), endTime.Format(time.RFC3339Nano))
	}

	span.End()
	concrete.mu.RLock()
	endTimeAfter := concrete.endTime
	concrete.mu.RUnlock()
	if !endTimeAfter.Equal(endTime) {
		t.Fatal("End() changed an unexported span already finalized by Shutdown()")
	}
}

// TestTracerShutdownPreservesEveryLifecycleError 锁定结束导出、关闭刷新和导出器关闭的完整错误链。
func TestTracerShutdownPreservesEveryLifecycleError(t *testing.T) {
	endErr := errors.New("span end export failed")
	flushErr := errors.New("active span flush failed")
	shutdownErr := errors.New("exporter shutdown failed")
	exporter := &sequencedLifecycleExporter{
		blockedStarted:   make(chan struct{}),
		releaseBlocked:   make(chan struct{}),
		firstExportErr:   endErr,
		blockedExportErr: flushErr,
		shutdownErr:      shutdownErr,
	}
	close(exporter.releaseBlocked)
	tracer := NewTracer()
	mustSetExporter(t, tracer, exporter)

	_, ended := tracer.StartSpan(context.Background(), "ended")
	ended.End()
	tracer.StartSpan(context.Background(), "active")

	err := tracer.Shutdown(context.Background())
	for _, expected := range []error{endErr, flushErr, shutdownErr} {
		if !errors.Is(err, expected) {
			t.Fatalf("Shutdown() error = %v, want %v in chain", err, expected)
		}
	}
}
