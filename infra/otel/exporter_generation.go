package otel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const maxRetirementErrors = 16

// retirementErrorState 有界保留历史退役错误，同时记录完整失败次数。
type retirementErrorState struct {
	failures uint64
	retained []error
}

func (s *retirementErrorState) add(err error) {
	if err == nil {
		return
	}
	s.failures++
	if len(s.retained) < maxRetirementErrors {
		s.retained = append(s.retained, err)
	}
}

func (s *retirementErrorState) snapshot() error {
	if s.failures == 0 {
		return nil
	}
	retainedErr := errors.Join(s.retained...)
	if s.failures > uint64(len(s.retained)) {
		return fmt.Errorf(
			"exporter retirement failed %d times; first %d errors retained: %w",
			s.failures,
			len(s.retained),
			retainedErr,
		)
	}
	return fmt.Errorf("exporter retirement failed %d times: %w", s.failures, retainedErr)
}

// exporterGeneration 将一次导出器所有权与其在途调用绑定，避免生命周期锁跨越外部调用。
type exporterGeneration struct {
	exporter Exporter

	mu            sync.Mutex
	accepting     bool
	inFlight      uint64
	drained       chan struct{}
	drainedClosed bool

	retireOnce sync.Once
	retireDone chan struct{}
	retireErr  error
}

// exporterLease 表示某一代导出器上的一次在途导出。
type exporterLease struct {
	generation  *exporterGeneration
	releaseOnce sync.Once
}

func newExporterGeneration(exporter Exporter) *exporterGeneration {
	return &exporterGeneration{
		exporter:   exporter,
		accepting:  true,
		drained:    make(chan struct{}),
		retireDone: make(chan struct{}),
	}
}

func (g *exporterGeneration) acquire() *exporterLease {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.accepting {
		return nil
	}
	g.inFlight++
	return &exporterLease{generation: g}
}

func (g *exporterGeneration) beginDrain() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.accepting = false
	g.closeDrainedLocked()
}

func (g *exporterGeneration) closeDrainedLocked() {
	if g.accepting || g.inFlight != 0 || g.drainedClosed {
		return
	}
	g.drainedClosed = true
	close(g.drained)
}

func (l *exporterLease) release() {
	if l == nil || l.generation == nil {
		return
	}
	l.releaseOnce.Do(func() {
		generation := l.generation
		generation.mu.Lock()
		generation.inFlight--
		generation.closeDrainedLocked()
		generation.mu.Unlock()
	})
}

// startRetirement 只负责启动退役；全部外部调用均在代际锁之外执行。
func (g *exporterGeneration) startRetirement(
	finalSpans []*SpanData,
	onComplete func(*exporterGeneration, error),
) {
	if g == nil {
		return
	}
	g.retireOnce.Do(func() {
		g.beginDrain()
		go func() {
			<-g.drained

			var flushErr error
			if len(finalSpans) > 0 {
				if err := g.exporter.ExportSpans(context.Background(), finalSpans); err != nil {
					flushErr = fmt.Errorf("flush active spans: %w", err)
				}
			}
			var shutdownErr error
			if err := g.exporter.Shutdown(context.Background()); err != nil {
				shutdownErr = fmt.Errorf("shutdown exporter: %w", err)
			}
			g.retireErr = errors.Join(flushErr, shutdownErr)
			if onComplete != nil {
				onComplete(g, g.retireErr)
			}
			close(g.retireDone)
		}()
	})
}

func (g *exporterGeneration) waitRetirement(ctx context.Context) (bool, error) {
	if g == nil {
		return true, nil
	}
	select {
	case <-g.retireDone:
		return true, g.retireErr
	default:
	}
	select {
	case <-g.retireDone:
		return true, g.retireErr
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
