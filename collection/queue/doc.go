// Package queue 提供泛型队列实现
//
// 包括 FIFO 队列和优先级队列，使用类型安全的泛型实现。
//
// 基本用法:
//
//	q := queue.New[int]()
//	q.Push(1)
//	q.Push(2)
//	v, ok := q.Pop()  // v=1, ok=true
//
// 优先级队列:
//
//	pq := queue.NewPriority[Task](func(a, b Task) bool {
//	    return a.Priority > b.Priority
//	})
//
// --- English ---
//
// Package queue provides generic queue implementations.
//
// Includes FIFO queue and priority queue with type-safe generics.
//
// Basic usage:
//
//	q := queue.New[int]()
//	q.Push(1)
//	q.Push(2)
//	v, ok := q.Pop()  // v=1, ok=true
//
// Priority queue:
//
//	pq := queue.NewPriority[Task](func(a, b Task) bool {
//	    return a.Priority > b.Priority
//	})
package queue
