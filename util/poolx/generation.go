package poolx

import (
	"sync"
	"sync/atomic"
	"time"
)

// poolGeneration 保存普通池单个运行代际的全部可变资源。
type poolGeneration struct {
	id                uint64
	state             atomic.Int32
	workers           *workerStack
	workerCount       atomic.Int32
	blockingCount     atomic.Int32
	heartbeat         chan struct{}
	heartbeatOnce     sync.Once
	wg                sync.WaitGroup
	retireOnce        sync.Once
	done              chan struct{}
	metrics           *Metrics
	priorityQueue     *PriorityQueue
	stealingScheduler *StealingScheduler
	createdAt         time.Time
}

func newPoolGeneration(id uint64, config Config, maxWorkers int32) *poolGeneration {
	generation := &poolGeneration{
		id:        id,
		workers:   newWorkerStack(int(maxWorkers)),
		heartbeat: make(chan struct{}),
		done:      make(chan struct{}),
		metrics:   &Metrics{},
		createdAt: time.Now(),
	}
	if config.EnablePriorityQueue {
		generation.priorityQueue = NewPriorityQueue(int(config.QueueSize))
	}
	if config.EnableWorkStealing {
		generation.stealingScheduler = NewStealingScheduler()
	}
	return generation
}

func (g *poolGeneration) stopCleaner() {
	g.heartbeatOnce.Do(func() { close(g.heartbeat) })
}

// retire 在关闭封口后等待本代际全部 Worker 退出。
func (g *poolGeneration) retire() <-chan struct{} {
	g.retireOnce.Do(func() {
		go func() {
			g.wg.Wait()
			close(g.done)
		}()
	})
	return g.done
}

// poolFuncGeneration 保存单函数池单个运行代际的全部可变资源。
type poolFuncGeneration struct {
	id            uint64
	state         atomic.Int32
	workers       *workerFuncStack
	workerCount   atomic.Int32
	blockingCount atomic.Int32
	heartbeat     chan struct{}
	heartbeatOnce sync.Once
	wg            sync.WaitGroup
	retireOnce    sync.Once
	done          chan struct{}
	metrics       *Metrics
	createdAt     time.Time
}

func newPoolFuncGeneration(id uint64, maxWorkers int32) *poolFuncGeneration {
	return &poolFuncGeneration{
		id:        id,
		workers:   newWorkerFuncStack(int(maxWorkers)),
		heartbeat: make(chan struct{}),
		done:      make(chan struct{}),
		metrics:   &Metrics{},
		createdAt: time.Now(),
	}
}

func (g *poolFuncGeneration) stopCleaner() {
	g.heartbeatOnce.Do(func() { close(g.heartbeat) })
}

// retire 在关闭封口后等待本代际全部 Worker 退出。
func (g *poolFuncGeneration) retire() <-chan struct{} {
	g.retireOnce.Do(func() {
		go func() {
			g.wg.Wait()
			close(g.done)
		}()
	})
	return g.done
}
