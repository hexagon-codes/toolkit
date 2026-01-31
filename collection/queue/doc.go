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
