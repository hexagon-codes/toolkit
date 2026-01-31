package asynq

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// =========================================
// 任务追踪器
// 用于分布式追踪和可观测性
// =========================================
// TaskTracer 任务追踪器
type TaskTracer struct {
	mu     sync.RWMutex
	events map[string][]TraceEvent // traceID -> events
}

// TraceEvent 追踪事件
type TraceEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	TraceID   string         `json:"trace_id"`
	TaskID    string         `json:"task_id"`
	Event     string         `json:"event"`
	Data      map[string]any `json:"data"`
}

var (
	taskTracer     *TaskTracer
	taskTracerOnce sync.Once
)

// GetTaskTracer 获取全局追踪器
func GetTaskTracer() *TaskTracer {
	taskTracerOnce.Do(func() {
		taskTracer = &TaskTracer{
			events: make(map[string][]TraceEvent),
		}
		// 启动清理协程，定期清理过期事件
		go taskTracer.cleanupLoop()
	})
	return taskTracer
}

// RecordEvent 记录追踪事件
func (t *TaskTracer) RecordEvent(ctx context.Context, traceID, taskID, event string, data map[string]any) {
	if traceID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e := TraceEvent{
		Timestamp: time.Now(),
		TraceID:   traceID,
		TaskID:    taskID,
		Event:     event,
		Data:      data,
	}
	t.events[traceID] = append(t.events[traceID], e)
	// 记录日志
	GetLogger().Log(fmt.Sprintf("[Trace] %s/%s: %s data=%v", traceID, taskID, event, data))
}

// GetEvents 获取追踪事件
func (t *TaskTracer) GetEvents(traceID string) []TraceEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if events, ok := t.events[traceID]; ok {
		// 返回副本
		result := make([]TraceEvent, len(events))
		copy(result, events)
		return result
	}
	return nil
}

// cleanupLoop 清理过期事件（保留 1 小时）
func (t *TaskTracer) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		t.cleanup()
	}
}

// cleanup 清理过期事件
func (t *TaskTracer) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-1 * time.Hour)
	for traceID, events := range t.events {
		// 检查最后一个事件的时间
		if len(events) > 0 && events[len(events)-1].Timestamp.Before(cutoff) {
			delete(t.events, traceID)
		}
	}
}
