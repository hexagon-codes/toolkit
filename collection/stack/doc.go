// Package stack provides a generic stack (LIFO) implementation.
//
// Basic usage:
//
//	s := stack.New[int]()
//	s.Push(1)
//	s.Push(2)
//	v, ok := s.Pop()   // v=2, ok=true
//	v, ok = s.Peek()   // v=1, ok=true (doesn't remove)
//	s.IsEmpty()        // false
//	s.Size()           // 1
package stack
