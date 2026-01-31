package errorx

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// Try 执行函数并捕获 panic，返回 error
func Try(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("panic: %v", r)
			}
		}
	}()
	fn()
	return nil
}

// TryWithValue 执行函数并捕获 panic，返回值和 error
func TryWithValue[T any](fn func() T) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("panic: %v", r)
			}
		}
	}()
	result = fn()
	return result, nil
}

// TryWithError 执行可能返回 error 的函数，同时捕获 panic
func TryWithError[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("panic: %v", r)
			}
		}
	}()
	return fn()
}

// Must 如果 error 不为 nil 则 panic
func Must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

// MustOK 如果 ok 为 false 则 panic
func MustOK[T any](val T, ok bool) T {
	if !ok {
		panic("assertion failed")
	}
	return val
}

// Must0 如果 error 不为 nil 则 panic（无返回值版本）
func Must0(err error) {
	if err != nil {
		panic(err)
	}
}

// Must1 与 Must 相同，提供语义化名称
func Must1[T any](val T, err error) T {
	return Must(val, err)
}

// Must2 处理两个返回值加 error 的情况
func Must2[T1, T2 any](v1 T1, v2 T2, err error) (T1, T2) {
	if err != nil {
		panic(err)
	}
	return v1, v2
}

// Must3 处理三个返回值加 error 的情况
func Must3[T1, T2, T3 any](v1 T1, v2 T2, v3 T3, err error) (T1, T2, T3) {
	if err != nil {
		panic(err)
	}
	return v1, v2, v3
}

// Wrap 包装 error，添加上下文信息
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Wrapf 包装 error，添加格式化的上下文信息
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}

// Unwrap 解包 error
func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// Is 判断 error 是否为指定类型
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As 尝试将 error 转换为指定类型
func As[T error](err error) (T, bool) {
	var target T
	ok := errors.As(err, &target)
	return target, ok
}

// New 创建新的 error
func New(message string) error {
	return errors.New(message)
}

// Newf 创建格式化的 error
func Newf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// Join 合并多个 error
func Join(errs ...error) error {
	return errors.Join(errs...)
}

// Ignore 忽略 error，只返回值
func Ignore[T any](val T, _ error) T {
	return val
}

// Ignore0 忽略 error
func Ignore0(_ error) {}

// IgnoreClose 忽略 Close 方法的 error（用于 defer）
func IgnoreClose(c interface{ Close() error }) {
	_ = c.Close()
}

// Coalesce 返回第一个非 nil 的 error
func Coalesce(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// StackError 带堆栈信息的 error
type StackError struct {
	err   error
	stack []uintptr
}

// WithStack 添加堆栈信息到 error
func WithStack(err error) error {
	if err == nil {
		return nil
	}
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(2, pcs[:])
	return &StackError{
		err:   err,
		stack: pcs[:n],
	}
}

// Error 实现 error 接口
func (e *StackError) Error() string {
	return e.err.Error()
}

// Unwrap 实现 errors.Unwrap 接口
func (e *StackError) Unwrap() error {
	return e.err
}

// Stack 返回堆栈信息
func (e *StackError) Stack() string {
	var sb strings.Builder
	frames := runtime.CallersFrames(e.stack)
	for {
		frame, more := frames.Next()
		sb.WriteString(fmt.Sprintf("%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line))
		if !more {
			break
		}
	}
	return sb.String()
}

// StackTrace 获取 error 的堆栈信息（如果有）
func StackTrace(err error) string {
	var se *StackError
	if errors.As(err, &se) {
		return se.Stack()
	}
	return ""
}

// Recover 从 panic 中恢复，返回 error
func Recover() error {
	if r := recover(); r != nil {
		if err, ok := r.(error); ok {
			return err
		}
		return fmt.Errorf("panic: %v", r)
	}
	return nil
}

// RecoverWithHandler 从 panic 中恢复，使用自定义处理函数
func RecoverWithHandler(handler func(error)) {
	if r := recover(); r != nil {
		var err error
		if e, ok := r.(error); ok {
			err = e
		} else {
			err = fmt.Errorf("panic: %v", r)
		}
		handler(err)
	}
}

// Safe 安全执行函数，返回 error 而不是 panic
func Safe(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("panic: %v", r)
			}
		}
	}()
	return fn()
}

// SafeGo 安全启动 goroutine，捕获 panic
func SafeGo(fn func(), onPanic func(error)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err error
				if e, ok := r.(error); ok {
					err = e
				} else {
					err = fmt.Errorf("panic: %v", r)
				}
				if onPanic != nil {
					onPanic(err)
				}
			}
		}()
		fn()
	}()
}

// Result 表示可能失败的操作结果
type Result[T any] struct {
	value T
	err   error
}

// Ok 创建成功的 Result
func Ok[T any](value T) Result[T] {
	return Result[T]{value: value}
}

// Err 创建失败的 Result
func Err[T any](err error) Result[T] {
	return Result[T]{err: err}
}

// FromError 从 (T, error) 创建 Result
func FromError[T any](value T, err error) Result[T] {
	return Result[T]{value: value, err: err}
}

// IsOk 判断是否成功
func (r Result[T]) IsOk() bool {
	return r.err == nil
}

// IsErr 判断是否失败
func (r Result[T]) IsErr() bool {
	return r.err != nil
}

// Value 获取值（失败时返回零值）
func (r Result[T]) Value() T {
	return r.value
}

// Error 获取 error
func (r Result[T]) Error() error {
	return r.err
}

// Unwrap 获取值和 error
func (r Result[T]) Unwrap() (T, error) {
	return r.value, r.err
}

// UnwrapOr 获取值，失败时返回默认值
func (r Result[T]) UnwrapOr(defaultVal T) T {
	if r.err != nil {
		return defaultVal
	}
	return r.value
}

// UnwrapOrElse 获取值，失败时调用函数获取默认值
func (r Result[T]) UnwrapOrElse(fn func(error) T) T {
	if r.err != nil {
		return fn(r.err)
	}
	return r.value
}

// Must 获取值，失败时 panic
func (r Result[T]) Must() T {
	if r.err != nil {
		panic(r.err)
	}
	return r.value
}

// Map 转换成功的值
func Map[T, U any](r Result[T], fn func(T) U) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return Ok(fn(r.value))
}

// FlatMap 转换成功的值（返回 Result）
func FlatMap[T, U any](r Result[T], fn func(T) Result[U]) Result[U] {
	if r.err != nil {
		return Err[U](r.err)
	}
	return fn(r.value)
}
