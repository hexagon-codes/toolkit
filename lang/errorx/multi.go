package errorx

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// MaxMultiErrorEntries 是单个聚合中最多保留的错误对象数。
// 超出上限后保留最早的错误和最新错误，并记录省略数量。
const MaxMultiErrorEntries = 100

// DefaultGoLimit 是 Go 使用的默认并发上限。
const DefaultGoLimit = 64

var multiErrorAppendMu sync.Mutex

// MultiError 多错误聚合，用于收集多个错误
//
// 线程安全，可在并发场景中使用
type MultiError struct {
	mu     sync.RWMutex
	errors []error
	total  int
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
// 示例:
//
//	me.Append(err1, err2)
func (m *MultiError) Append(errs ...error) {
	if m == nil {
		return
	}

	// 循环检测与写入必须串行，避免两个聚合并发互相引用。
	multiErrorAppendMu.Lock()
	defer multiErrorAppendMu.Unlock()

	for _, err := range errs {
		if isNilError(err) {
			continue
		}
		if referencesMultiError(err, m) {
			m.appendOne(ErrCyclicReference)
			continue
		}
		m.appendOne(err)
	}
}

func (m *MultiError) appendOne(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.total++
	if len(m.errors) < MaxMultiErrorEntries {
		m.errors = append(m.errors, err)
		return
	}
	// 首部保留最早样本，末位持续更新为最新错误。
	m.errors[MaxMultiErrorEntries-1] = err
}

// AppendResult 添加操作结果的错误
//
// 参数:
//   - _: 被忽略的值（用于接收函数返回值）
//   - err: 要添加的错误
//
// 示例:
//
//	me.AppendResult(os.Remove("file1.txt"))
//	me.AppendResult(os.Remove("file2.txt"))
func (m *MultiError) AppendResult(_ any, err error) {
	m.Append(err)
}

// Errors 返回所有收集的错误
//
// 返回:
//   - []error: 错误切片的副本
func (m *MultiError) Errors() []error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

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
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.total
}

// Omitted 返回因聚合上限而未保留的错误数量。
func (m *MultiError) Omitted() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.total - len(m.errors)
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
	errs, total := m.snapshot()

	if total == 0 {
		return ""
	}

	if total == 1 {
		return errs[0].Error()
	}

	var sb strings.Builder
	sb.WriteString("multiple errors occurred:\n")
	for i, err := range errs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
	}
	if omitted := total - len(errs); omitted > 0 {
		fmt.Fprintf(&sb, "\n  - ... %d errors omitted (%d retained, %d total)", omitted, len(errs), total)
	}
	return sb.String()
}

func (m *MultiError) snapshot() (errs []error, total int) {
	if m == nil {
		return nil, 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	errs = make([]error, len(m.errors))
	copy(errs, m.errors)
	total = m.total
	return errs, total
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
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

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
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

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
//   - error: 全部成功时返回 nil，否则返回按输入顺序聚合的错误
//
// 示例:
//
//	err := errorx.Go(
//	    func() error { return doTask1() },
//	    func() error { return doTask2() },
//	    func() error { return doTask3() },
//	)
//	if err != nil {
//	    return err
//	}
func Go(fns ...func() error) error {
	return GoWithLimit(DefaultGoLimit, fns...)
}

// GoWithLimit 并行执行多个函数，限制并发数
//
// 参数:
//   - limit: 最大并发数
//   - fns: 要执行的函数列表
//
// 返回:
//   - error: limit 非法或任一操作失败时返回错误，否则返回 nil
//
// 示例:
//
//	err := errorx.GoWithLimit(3,
//	    func() error { return process(items[0]) },
//	    func() error { return process(items[1]) },
//	    // ... more functions
//	)
func GoWithLimit(limit int, fns ...func() error) error {
	if limit <= 0 {
		return fmt.Errorf("%w: got %d", ErrInvalidLimit, limit)
	}
	if len(fns) == 0 {
		return nil
	}
	if limit > len(fns) {
		limit = len(fns)
	}

	results := make([]error, len(fns))
	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(limit)
	for range limit {
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = runOperation(index, fns[index])
			}
		}()
	}

	for index := range fns {
		jobs <- index
	}
	close(jobs)

	wg.Wait()
	return CombineErrors(results...)
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
	if err == nil || fn == nil {
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
	visited := make(map[errorIdentity]struct{})

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
		identity, identifiable := identifyError(item.err)
		// 值类型 error 没有稳定指针，依靠深度上限终止异常循环。
		if identifiable {
			if _, ok := visited[identity]; ok {
				continue
			}
			visited[identity] = struct{}{}
		}

		// 调用处理函数
		if !fn(item.err) {
			return
		}

		// 收集子错误并压入栈中（逆序压入以保持遍历顺序）
		var children []error

		switch typedErr := item.err.(type) {
		case *MultiError:
			children = typedErr.Errors()
		case interface{ Unwrap() []error }:
			// 处理 errors.Join 返回的类型
			children = typedErr.Unwrap()
		case interface{ Unwrap() error }:
			// 处理单个包装错误
			if unwrapped := typedErr.Unwrap(); unwrapped != nil {
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

type errorIdentity struct {
	typeOf  reflect.Type
	pointer uintptr
}

// identifyError 获取指针错误的类型和地址，避免不同类型共享地址时误判。
func identifyError(err error) (errorIdentity, bool) {
	v := reflect.ValueOf(err)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errorIdentity{}, false
	}
	return errorIdentity{typeOf: v.Type(), pointer: v.Pointer()}, true
}

func isNilError(err error) bool {
	if err == nil {
		return true
	}
	v := reflect.ValueOf(err)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func referencesMultiError(root error, target *MultiError) bool {
	stack := []error{root}
	visited := make(map[errorIdentity]struct{})
	for inspected := 0; len(stack) > 0; inspected++ {
		// 对无法建立稳定身份的异常错误图采取保守拒绝策略。
		if inspected >= maxWalkDepth {
			return true
		}
		last := len(stack) - 1
		err := stack[last]
		stack = stack[:last]
		if isNilError(err) {
			continue
		}
		if aggregate, ok := err.(*MultiError); ok {
			if aggregate == target {
				return true
			}
		}
		if identity, identifiable := identifyError(err); identifiable {
			if _, exists := visited[identity]; exists {
				continue
			}
			visited[identity] = struct{}{}
		}

		switch typedErr := err.(type) {
		case *MultiError:
			stack = append(stack, typedErr.Errors()...)
		case interface{ Unwrap() []error }:
			stack = append(stack, typedErr.Unwrap()...)
		case interface{ Unwrap() error }:
			stack = append(stack, typedErr.Unwrap())
		}
	}
	return false
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
	results := make([]error, len(ops))
	for index, op := range ops {
		results[index] = runOperation(index, op)
	}
	return CombineErrors(results...)
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

func runOperation(index int, operation func() error) (err error) {
	if operation == nil {
		return &OperationError{Index: index, Err: ErrNilOperation}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &OperationError{Index: index, Err: newPanicError(recovered)}
		}
	}()
	if operationErr := operation(); operationErr != nil {
		return &OperationError{Index: index, Err: operationErr}
	}
	return nil
}
