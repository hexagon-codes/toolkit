package contextx

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- 超时相关 ---

// WithTimeout 创建带超时的 context
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// WithDeadline 创建带截止时间的 context
func WithDeadline(parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, deadline)
}

// WithTimeoutCause 创建带超时和原因的 context（Go 1.21+）
func WithTimeoutCause(parent context.Context, timeout time.Duration, cause error) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(parent, timeout, cause)
}

// WithDeadlineCause 创建带截止时间和原因的 context（Go 1.21+）
func WithDeadlineCause(parent context.Context, deadline time.Time, cause error) (context.Context, context.CancelFunc) {
	return context.WithDeadlineCause(parent, deadline, cause)
}

// --- 取消相关 ---

// WithCancel 创建可取消的 context
func WithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// WithCancelCause 创建可取消且带原因的 context（Go 1.20+）
func WithCancelCause(parent context.Context) (context.Context, context.CancelCauseFunc) {
	return context.WithCancelCause(parent)
}

// Cause 获取 context 取消的原因（Go 1.20+）
func Cause(ctx context.Context) error {
	return context.Cause(ctx)
}

// --- 值传递相关 ---

var contextKeySequence atomic.Uint64

// Key 是类型安全的 context key。
type Key[T any] struct {
	name     string
	identity uint64
}

// NewKey 创建类型安全的 context key
func NewKey[T any](name string) Key[T] {
	return Key[T]{name: name, identity: contextKeySequence.Add(1)}
}

// WithValue 使用类型安全的 key 设置值
func WithValue[T any](ctx context.Context, key Key[T], value T) context.Context {
	return context.WithValue(ctx, key, value)
}

// Value 使用类型安全的 key 获取值
func Value[T any](ctx context.Context, key Key[T]) (T, bool) {
	v, ok := ctx.Value(key).(T)
	return v, ok
}

// MustValue 使用类型安全的 key 获取值，不存在则 panic
//
// 警告：仅建议在程序初始化阶段使用。在请求处理路径中，建议使用 Value 或 ValueOr。
// 在生产环境中，panic 可能导致服务中断。
func MustValue[T any](ctx context.Context, key Key[T]) T {
	v, ok := Value(ctx, key)
	if !ok {
		panic("contextx: value not found for key: " + key.name)
	}
	return v
}

// TryValue 使用类型安全的 key 获取值（非 panic 版本）
//
// 与 MustValue 不同，当值不存在时返回 KeyNotFoundError 错误而非 panic。
// 适用于需要显式错误处理的场景，推荐在请求处理路径中使用。
func TryValue[T any](ctx context.Context, key Key[T]) (T, error) {
	v, ok := Value(ctx, key)
	if !ok {
		var zero T
		return zero, &KeyNotFoundError{Key: key.name}
	}
	return v, nil
}

// KeyNotFoundError 表示 context 中找不到指定的 key
type KeyNotFoundError struct {
	Key string // key 的名称
}

// Error 实现 error 接口
func (e *KeyNotFoundError) Error() string {
	return "contextx: value not found for key: " + e.Key
}

// ValueOr 使用类型安全的 key 获取值，不存在则返回默认值
func ValueOr[T any](ctx context.Context, key Key[T], defaultValue T) T {
	v, ok := Value(ctx, key)
	if !ok {
		return defaultValue
	}
	return v
}

// --- 常用 context key ---

var (
	// TraceIDKey trace id key
	TraceIDKey = NewKey[string]("trace_id")
	// RequestIDKey request id key
	RequestIDKey = NewKey[string]("request_id")
	// UserIDKey user id key
	UserIDKey = NewKey[int64]("user_id")
	// TenantIDKey tenant id key
	TenantIDKey = NewKey[string]("tenant_id")
)

// WithTraceID 设置 trace id
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return WithValue(ctx, TraceIDKey, traceID)
}

// TraceID 获取 trace id
func TraceID(ctx context.Context) string {
	return ValueOr(ctx, TraceIDKey, "")
}

// WithRequestID 设置 request id
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return WithValue(ctx, RequestIDKey, requestID)
}

// RequestID 获取 request id
func RequestID(ctx context.Context) string {
	return ValueOr(ctx, RequestIDKey, "")
}

// WithUserID 设置 user id
func WithUserID(ctx context.Context, userID int64) context.Context {
	return WithValue(ctx, UserIDKey, userID)
}

// UserID 获取 user id
func UserID(ctx context.Context) int64 {
	return ValueOr(ctx, UserIDKey, 0)
}

// WithTenantID 设置 tenant id
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return WithValue(ctx, TenantIDKey, tenantID)
}

// TenantID 获取 tenant id
func TenantID(ctx context.Context) string {
	return ValueOr(ctx, TenantIDKey, "")
}

// --- 工具函数 ---

// IsTimeout 判断 context 是否因超时而取消
func IsTimeout(ctx context.Context) bool {
	return ctx.Err() == context.DeadlineExceeded
}

// IsCanceled 判断 context 是否被取消
func IsCanceled(ctx context.Context) bool {
	return ctx.Err() == context.Canceled
}

// IsDone 判断 context 是否已完成（取消或超时）
func IsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// Remaining 返回 context 剩余时间
func Remaining(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return -1 // 没有设置截止时间
	}
	return time.Until(deadline)
}

// HasDeadline 判断 context 是否设置了截止时间
func HasDeadline(ctx context.Context) bool {
	_, ok := ctx.Deadline()
	return ok
}

// --- 运行控制 ---

var (
	// ErrNilContext 表示运行控制函数收到 nil context。
	ErrNilContext = errors.New("contextx: context must not be nil")
	// ErrNilTask 表示运行控制函数收到 nil task。
	ErrNilTask = errors.New("contextx: task must not be nil")
)

// Go 在 goroutine 中运行函数
// 注意：函数内部应该自行检查 ctx.Done() 来响应取消
// 此函数只在启动时检查 context 是否已取消，不会中断正在执行的函数
func Go(ctx context.Context, fn func(ctx context.Context)) error {
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilTask
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	go func() {
		// 在启动 goroutine 后检查 context 状态
		if ctx.Err() != nil {
			return
		}
		fn(ctx)
	}()
	return nil
}

// Run 使用调用方 context 同步运行函数。
// 取消由任务通过 ctx 协作处理，Run 不会提前返回并遗留后台任务。
func Run(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilTask
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	taskErr := fn(ctx)
	contextErr := ctx.Err()
	if taskErr == nil {
		return contextErr
	}
	if contextErr == nil || errors.Is(taskErr, contextErr) {
		return taskErr
	}
	return errors.Join(taskErr, contextErr)
}

// RunTimeout 从父 context 派生超时并同步运行函数。
func RunTimeout(parent context.Context, timeout time.Duration, fn func(context.Context) error) error {
	if parent == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilTask
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return Run(ctx, fn)
}

// --- Detach ---

// Detach 创建一个脱离父 context 取消控制的新 context
// 新 context 会继承父 context 的值，但不会被父 context 取消
func Detach(ctx context.Context) context.Context {
	return &detachedContext{ctx: ctx}
}

type detachedContext struct {
	ctx context.Context
}

func (d *detachedContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (d *detachedContext) Done() <-chan struct{} {
	return nil
}

func (d *detachedContext) Err() error {
	return nil
}

func (d *detachedContext) Value(key any) any {
	return d.ctx.Value(key)
}

// --- Merge ---

// Merge 合并多个 context，任意一个取消则合并后的 context 也取消
// 使用 context.AfterFunc 避免 goroutine 泄漏
func Merge(contexts ...context.Context) (context.Context, context.CancelFunc) {
	sources := make([]context.Context, 0, len(contexts))
	for _, source := range contexts {
		if source != nil {
			sources = append(sources, source)
		}
	}

	ctx, cancelCause := context.WithCancelCause(context.Background())
	var cancelOnce sync.Once
	var stopMu sync.Mutex
	stopFuncs := make([]func() bool, 0, len(sources))
	stopped := false
	cancelMerged := func(cause error) {
		cancelOnce.Do(func() {
			if cause == nil {
				cause = context.Canceled
			}
			cancelCause(cause)
			stopMu.Lock()
			stopped = true
			stops := append([]func() bool(nil), stopFuncs...)
			stopMu.Unlock()
			for _, stop := range stops {
				stop()
			}
		})
	}

	for _, source := range sources {
		source := source
		stop := context.AfterFunc(source, func() {
			cause := context.Cause(source)
			if cause == nil {
				cause = source.Err()
			}
			cancelMerged(cause)
		})
		stopMu.Lock()
		if stopped {
			stopMu.Unlock()
			stop()
			continue
		}
		stopFuncs = append(stopFuncs, stop)
		stopMu.Unlock()
	}

	return &mergedContext{Context: ctx, contexts: sources}, func() {
		cancelMerged(context.Canceled)
	}
}

type mergedContext struct {
	context.Context
	contexts []context.Context
}

func (m *mergedContext) Deadline() (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, source := range m.contexts {
		deadline, ok := source.Deadline()
		if ok && (!found || deadline.Before(earliest)) {
			earliest = deadline
			found = true
		}
	}
	return earliest, found
}

func (m *mergedContext) Value(key any) any {
	for _, ctx := range m.contexts {
		if v := ctx.Value(key); v != nil {
			return v
		}
	}
	return nil
}

// --- AfterFunc ---

// AfterFunc 在 context 取消后执行函数
func AfterFunc(ctx context.Context, fn func()) func() bool {
	return context.AfterFunc(ctx, fn)
}

// --- WaitGroup with Context ---

// WaitGroupContext 带 context 支持的 WaitGroup
type WaitGroupContext struct {
	ctx     context.Context
	mu      sync.Mutex
	pending int
	done    chan struct{}
	errs    []error
}

// NewWaitGroupContext 创建带 context 的 WaitGroup
func NewWaitGroupContext(ctx context.Context) *WaitGroupContext {
	done := make(chan struct{})
	close(done)
	return &WaitGroupContext{ctx: ctx, done: done}
}

// Go 启动一个 goroutine
func (w *WaitGroupContext) Go(fn func(ctx context.Context) error) error {
	if w.ctx == nil {
		return ErrNilContext
	}
	if fn == nil {
		return ErrNilTask
	}
	w.mu.Lock()
	if w.pending == 0 {
		w.done = make(chan struct{})
		w.errs = nil
	}
	w.pending++
	w.mu.Unlock()

	go func() {
		var taskErr error
		defer func() { w.finishTask(taskErr) }()
		taskErr = fn(w.ctx)
	}()
	return nil
}

// Wait 等待所有 goroutine 完成或 context 取消。
// 注意：当 context 取消导致 Wait 提前返回时，后台已启动的任务仍会继续运行直到完成。
// 这是预期行为，调用方应在任务函数中通过检查 ctx.Done() 来及时退出。
// 同一批次的重复 Wait 会复用完成通知，不会创建额外 goroutine。
func (w *WaitGroupContext) Wait() error {
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()

	select {
	case <-done:
		return w.waitError()
	default:
	}

	select {
	case <-done:
		return w.waitError()
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *WaitGroupContext) finishTask(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.errs = append(w.errs, err)
	}
	w.pending--
	if w.pending == 0 {
		close(w.done)
	}
}

func (w *WaitGroupContext) waitError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	errs := append([]error(nil), w.errs...)
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return multiWaitError(errs)
	}
}

// multiWaitError 合并多个错误
type waitErrors struct {
	errs []error
}

func (e *waitErrors) Error() string {
	msgs := make([]string, len(e.errs))
	for i, err := range e.errs {
		msgs[i] = err.Error()
	}
	return "multiple errors: " + joinStrings(msgs, "; ")
}

func (e *waitErrors) Unwrap() []error {
	return e.errs
}

func multiWaitError(errs []error) error {
	return &waitErrors{errs: errs}
}

// joinStrings 拼接字符串切片
func joinStrings(ss []string, sep string) string {
	return strings.Join(ss, sep)
}

// --- Pool ---

var (
	// ErrNilPoolContext 表示协程池收到 nil context。
	ErrNilPoolContext = errors.New("contextx: pool context must not be nil")
	// ErrInvalidPoolSize 表示协程池大小不是正数。
	ErrInvalidPoolSize = errors.New("contextx: pool size must be greater than zero")
	// ErrPoolClosed 表示协程池已停止接收任务。
	ErrPoolClosed = errors.New("contextx: pool is closed")
)

// Pool 带 context 的协程池
type Pool struct {
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	sem       chan struct{}
	mu        sync.Mutex
	accepting bool
	errs      []error
}

// NewPool 创建协程池
func NewPool(ctx context.Context, size int) (*Pool, error) {
	if ctx == nil {
		return nil, ErrNilPoolContext
	}
	if size <= 0 {
		return nil, ErrInvalidPoolSize
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Pool{
		ctx:       ctx,
		cancel:    cancel,
		sem:       make(chan struct{}, size),
		accepting: true,
	}, nil
}

// Go 在池中启动任务
func (p *Pool) Go(fn func(ctx context.Context) error) error {
	if fn == nil {
		return ErrNilTask
	}
	p.mu.Lock()
	if !p.accepting {
		p.mu.Unlock()
		return ErrPoolClosed
	}
	p.wg.Add(1)
	p.mu.Unlock()

	select {
	case <-p.ctx.Done():
		p.wg.Done()
		return p.ctx.Err()
	case p.sem <- struct{}{}:
	}
	if err := p.ctx.Err(); err != nil {
		<-p.sem
		p.wg.Done()
		return err
	}

	go func() {
		defer func() {
			<-p.sem
			p.wg.Done()
		}()
		if err := fn(p.ctx); err != nil {
			p.mu.Lock()
			p.errs = append(p.errs, err)
			p.mu.Unlock()
		}
	}()
	return nil
}

// Wait 等待所有任务完成。
// 返回首个任务错误（多个错误时合并）；若无任务错误但 context 已取消，返回 ctx.Err()。
func (p *Pool) Wait() error {
	p.mu.Lock()
	p.accepting = false
	p.mu.Unlock()
	p.wg.Wait()

	p.mu.Lock()
	errs := append([]error(nil), p.errs...)
	p.mu.Unlock()
	contextErr := p.ctx.Err()
	if contextErr != nil {
		found := false
		for _, err := range errs {
			if errors.Is(err, contextErr) {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, contextErr)
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return multiWaitError(errs)
	}
}

// Close 关闭池
func (p *Pool) Close() {
	p.mu.Lock()
	p.accepting = false
	p.mu.Unlock()
	p.cancel()
	p.wg.Wait()
}
