package otel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleProbeExporter struct {
	exportCalls   atomic.Int32
	shutdownCalls atomic.Int32
	shutdownErr   error
}

type blockingLifecycleExporter struct {
	exportStarted        chan struct{}
	releaseExport        chan struct{}
	exportStartedOnce    sync.Once
	inFlightExports      atomic.Int32
	shutdownCalls        atomic.Int32
	shutdownDuringExport atomic.Bool
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
	return nil
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
	if err := tracer.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
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
	go func() {
		replaceDone <- tracer.SetExporter(context.Background(), next)
	}()
	prematureReturn := false
	select {
	case <-replaceDone:
		prematureReturn = true
	case <-time.After(20 * time.Millisecond):
	}
	close(previous.releaseExport)
	<-endDone
	if !prematureReturn {
		if err := <-replaceDone; err != nil {
			t.Fatalf("SetExporter() error = %v", err)
		}
	}
	if prematureReturn {
		t.Fatal("SetExporter() returned before the in-flight export completed")
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
