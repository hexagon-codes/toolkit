package errorx

import (
	"errors"
	"fmt"
	"runtime/debug"
)

var (
	// ErrNilOperation 表示调用方传入了 nil 操作。
	ErrNilOperation = errors.New("errorx: operation must not be nil")
	// ErrNilCallback 表示调用方传入了 nil 回调。
	ErrNilCallback = errors.New("errorx: callback must not be nil")
	// ErrInvalidLimit 表示并发上限不是正数。
	ErrInvalidLimit = errors.New("errorx: limit must be greater than zero")
	// ErrCyclicReference 表示错误聚合拒绝了循环引用。
	ErrCyclicReference = errors.New("errorx: cyclic error reference")
)

// PanicError 表示从 panic 恢复得到的错误，并保留恢复点堆栈。
type PanicError struct {
	Value      any
	StackTrace []byte
}

// Error 返回 panic 的可诊断文本。
func (e *PanicError) Error() string {
	if err, ok := e.Value.(error); ok && !isNilError(err) {
		return err.Error()
	}
	return fmt.Sprintf("panic: %v", e.Value)
}

// Unwrap 在 panic 值本身是 error 时保留 errors.Is/As 语义。
func (e *PanicError) Unwrap() error {
	err, ok := e.Value.(error)
	if !ok || isNilError(err) {
		return nil
	}
	return err
}

// Stack 返回 panic 恢复点的堆栈。
func (e *PanicError) Stack() string {
	return string(e.StackTrace)
}

// OperationError 为批量操作错误补充稳定的输入位置。
type OperationError struct {
	// Index 是操作在输入切片中的零基索引。
	Index int
	Err   error
}

// Error 返回带操作位置的错误文本。
func (e *OperationError) Error() string {
	return fmt.Sprintf("operation %d: %v", e.Index, e.Err)
}

// Unwrap 保留底层错误的 errors.Is/As 语义。
func (e *OperationError) Unwrap() error {
	return e.Err
}

func newPanicError(value any) *PanicError {
	return &PanicError{Value: value, StackTrace: debug.Stack()}
}

func recoveredError(value any) error {
	return newPanicError(value)
}
