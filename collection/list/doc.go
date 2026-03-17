// Package list 提供泛型双向链表实现
//
// 链表使用 Go 1.18+ 泛型保证类型安全，
// 提供 O(1) 时间复杂度的插入和删除操作。
//
// 基本用法:
//
//	l := list.New[int]()
//	l.PushBack(1)
//	l.PushFront(0)
//	for e := l.Front(); e != nil; e = e.Next() {
//	    fmt.Println(e.Value)
//	}
//
// --- English ---
//
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
