package errorx

import (
	"reflect"
	"strings"
	"sync"
)

// MultiError 多错误聚合，用于收集多个错误
//
// 线程安全，可在并发场景中使用
type MultiError struct {
	mu     sync.Mutex
	errors []error
}

// NewMultiError 创建一个新的 MultiError
//
// 返回:
//   - *MultiError: 空的 MultiError
//
// 示例:
//
//	me := errorx.NewMultiError()
//	me.Append(err1)
//	me.Append(err2)
//	return me.ErrorOrNil()
func NewMultiError() *MultiError {
	return &MultiError{}
}

// Append 添加错误到 MultiError
//
// 参数:
//   - errs: 要添加的错误（nil 值会被忽略）
//
// 返回:
//   - *MultiError: 返回自身以支持链式调用
//
// 示例:
//
//	me.Append(err1).Append(err2)
func (m *MultiError) Append(errs ...error) *MultiError {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, err := range errs {
		if err != nil {
			m.errors = append(m.errors, err)
		}
	}
	return m
}

// AppendResult 添加操作结果的错误
//
// 参数:
//   - _: 被忽略的值（用于接收函数返回值）
//   - err: 要添加的错误
//
// 返回:
//   - *MultiError: 返回自身以支持链式调用
//
// 示例:
//
//	me.AppendResult(os.Remove("file1.txt"))
//	me.AppendResult(os.Remove("file2.txt"))
func (m *MultiError) AppendResult(_ any, err error) *MultiError {
	return m.Append(err)
}

// Errors 返回所有收集的错误
//
// 返回:
//   - []error: 错误切片的副本
func (m *MultiError) Errors() []error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errors == nil {
		return nil
	}
	result := make([]error, len(m.errors))
	copy(result, m.errors)
	return result
}

// Len 返回错误数量
//
// 返回:
//   - int: 错误数量
func (m *MultiError) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.errors)
}

// HasErrors 检查是否有错误
//
// 返回:
//   - bool: 如果有错误返回 true
func (m *MultiError) HasErrors() bool {
	return m.Len() > 0
}

// Error 实现 error 接口
//
// 返回:
//   - string: 所有错误的字符串表示
func (m *MultiError) Error() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.errors) == 0 {
		return ""
	}

	if len(m.errors) == 1 {
		return m.errors[0].Error()
	}

	var sb strings.Builder
	sb.WriteString("multiple errors occurred:\n")
	for i, err := range m.errors {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
	}
	return sb.String()
}

// ErrorOrNil 如果没有错误则返回 nil，否则返回自身
//
// 返回:
//   - error: 如果有错误返回 *MultiError，否则返回 nil
//
// 示例:
//
//	me := errorx.NewMultiError()
//	// ... append errors ...
//	if err := me.ErrorOrNil(); err != nil {
//	    return err
//	}
func (m *MultiError) ErrorOrNil() error {
	if m.Len() == 0 {
		return nil
	}
	return m
}

// Unwrap 实现 Go 1.20+ errors.Unwrap 接口
//
// 返回:
//   - []error: 所有包含的错误
func (m *MultiError) Unwrap() []error {
	return m.Errors()
}

// First 返回第一个错误
//
// 返回:
//   - error: 第一个错误，如果没有错误返回 nil
func (m *MultiError) First() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.errors) == 0 {
		return nil
	}
	return m.errors[0]
}

// Last 返回最后一个错误
//
// 返回:
//   - error: 最后一个错误，如果没有错误返回 nil
func (m *MultiError) Last() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.errors) == 0 {
		return nil
	}
	return m.errors[len(m.errors)-1]
}

// Go 并行执行多个函数，收集所有错误
//
// 参数:
//   - fns: 要并行执行的函数列表
//
// 返回:
//   - *MultiError: 包含所有错误的 MultiError
//
// 示例:
//
//	me := errorx.Go(
//	    func() error { return doTask1() },
//	    func() error { return doTask2() },
//	    func() error { return doTask3() },
//	)
//	if err := me.ErrorOrNil(); err != nil {
//	    return err
//	}
func Go(fns ...func() error) *MultiError {
	me := NewMultiError()
	var wg sync.WaitGroup
	wg.Add(len(fns))

	for _, fn := range fns {
		go func(f func() error) {
			defer wg.Done()
			if err := f(); err != nil {
				me.Append(err)
			}
		}(fn)
	}

	wg.Wait()
	return me
}

// GoWithLimit 并行执行多个函数，限制并发数
//
// 参数:
//   - limit: 最大并发数
//   - fns: 要执行的函数列表
//
// 返回:
//   - *MultiError: 包含所有错误的 MultiError
//
// 示例:
//
//	me := errorx.GoWithLimit(3,
//	    func() error { return process(items[0]) },
//	    func() error { return process(items[1]) },
//	    // ... more functions
//	)
func GoWithLimit(limit int, fns ...func() error) *MultiError {
	if limit <= 0 {
		limit = 1
	}

	me := NewMultiError()
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	wg.Add(len(fns))

	for _, fn := range fns {
		go func(f func() error) {
			defer wg.Done()
			// 在 goroutine 内部获取信号量，防止主 goroutine 阻塞导致死锁
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := f(); err != nil {
				me.Append(err)
			}
		}(fn)
	}

	wg.Wait()
	return me
}

// maxWalkDepth 最大遍历深度，防止无限循环
const maxWalkDepth = 1000

// Walk 遍历错误链，对每个错误调用函数
//
// 参数:
//   - err: 要遍历的错误
//   - fn: 对每个错误调用的函数，返回 false 停止遍历
//
// 注意: 使用迭代方式实现，避免深层错误链导致栈溢出
// 限制最大遍历深度为 1000，超过后自动停止，防止循环引用导致无限循环
//
// 示例:
//
//	errorx.Walk(err, func(e error) bool {
//	    if myErr, ok := e.(*MyError); ok {
//	        handleMyError(myErr)
//	        return false  // 停止遍历
//	    }
//	    return true  // 继续遍历
//	})
func Walk(err error, fn func(error) bool) {
	if err == nil {
		return
	}

	// 使用栈代替递归，防止深层错误链导致栈溢出
	// 使用 map 记录已访问的错误指针地址，防止循环引用导致无限循环
	type stackItem struct {
		err   error
		depth int
	}
	stack := []stackItem{{err: err, depth: 0}}
	// 使用 uintptr 作为 key，基于错误对象的内存地址进行去重
	// 这比使用 error 接口作为 key 更可靠，因为：
	// 1. 不依赖于 error 的值相等性
	// 2. 不会因为不可比较的类型而 panic
	visited := make(map[uintptr]struct{})

	for len(stack) > 0 {
		// 弹出栈顶元素
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if item.err == nil {
			continue
		}

		// 检查深度限制，防止无限循环
		if item.depth >= maxWalkDepth {
			continue
		}

		// 获取错误对象的指针地址用于去重
		// 使用 interface 的数据指针作为唯一标识
		ptr := errorPtr(item.err)
		// ptr == 0 表示值类型 error，不做去重缓存，每次都遍历
		if ptr != 0 {
			if _, ok := visited[ptr]; ok {
				continue
			}
			visited[ptr] = struct{}{}
		}

		// 调用处理函数
		if !fn(item.err) {
			return
		}

		// 收集子错误并压入栈中（逆序压入以保持遍历顺序）
		var children []error

		// 处理 MultiError
		if me, ok := item.err.(*MultiError); ok {
			children = me.Errors()
		} else if unwrapper, ok := item.err.(interface{ Unwrap() []error }); ok {
			// 处理 errors.Join 返回的类型
			children = unwrapper.Unwrap()
		} else if unwrapper, ok := item.err.(interface{ Unwrap() error }); ok {
			// 处理单个包装错误
			if unwrapped := unwrapper.Unwrap(); unwrapped != nil {
				children = []error{unwrapped}
			}
		}

		// 逆序压入栈以保持遍历顺序
		for i := len(children) - 1; i >= 0; i-- {
			if children[i] != nil {
				stack = append(stack, stackItem{err: children[i], depth: item.depth + 1})
			}
		}
	}
}

// errorPtr 获取 error 接口的数据指针，用于唯一标识
// 利用 reflect 获取底层值的指针地址
func errorPtr(err error) uintptr {
	v := reflect.ValueOf(err)
	// 对于指针类型，直接获取指针值
	if v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if !v.IsNil() {
			return v.Pointer()
		}
	}
	// 对于值类型（非指针），返回 0（不缓存），让每个值类型 error 都被遍历
	// 结合深度限制，可以有效防止无限循环
	return 0
}

// CollectErrors 从多个操作收集错误
//
// 参数:
//   - ops: 返回错误的操作函数
//
// 返回:
//   - error: 如果有错误返回 *MultiError，否则返回 nil
//
// 示例:
//
//	err := errorx.CollectErrors(
//	    func() error { return validate(data) },
//	    func() error { return save(data) },
//	    func() error { return notify(data) },
//	)
func CollectErrors(ops ...func() error) error {
	me := NewMultiError()
	for _, op := range ops {
		me.Append(op())
	}
	return me.ErrorOrNil()
}

// CombineErrors 合并多个错误
//
// 参数:
//   - errs: 要合并的错误列表
//
// 返回:
//   - error: 如果有错误返回合并后的错误，否则返回 nil
//
// 示例:
//
//	err := errorx.CombineErrors(err1, err2, err3)
func CombineErrors(errs ...error) error {
	me := NewMultiError()
	me.Append(errs...)
	return me.ErrorOrNil()
}
