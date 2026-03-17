// Package set 提供泛型集合实现
//
// 集合使用 map 实现，提供 O(1) 时间复杂度的添加、删除和包含检查操作。
//
// 基本用法:
//
//	s := set.New[int]()
//	s.Add(1)
//	s.Add(2)
//	s.Contains(1)  // true
//	s.Remove(1)
//	s.Size()       // 1
//
// 集合运算:
//
//	union := s1.Union(s2)
//	inter := s1.Intersection(s2)
//	diff := s1.Difference(s2)
//
// --- English ---
//
// Package set provides a generic set implementation.
//
// The set uses a map for O(1) add, remove, and contains operations.
//
// Basic usage:
//
//	s := set.New[int]()
//	s.Add(1)
//	s.Add(2)
//	s.Contains(1)  // true
//	s.Remove(1)
//	s.Size()       // 1
//
// Set operations:
//
//	union := s1.Union(s2)
//	inter := s1.Intersection(s2)
//	diff := s1.Difference(s2)
package set
