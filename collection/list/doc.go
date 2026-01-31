// Package list provides a generic doubly linked list implementation.
//
// The list is type-safe using Go 1.18+ generics and provides
// O(1) insertion and deletion operations.
//
// Basic usage:
//
//	l := list.New[int]()
//	l.PushBack(1)
//	l.PushFront(0)
//	for e := l.Front(); e != nil; e = e.Next() {
//	    fmt.Println(e.Value)
//	}
package list
