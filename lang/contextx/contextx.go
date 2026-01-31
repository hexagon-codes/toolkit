package contextx

import (
	"context"
	"sync"
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

// 类型安全的 context key
type contextKey[T any] struct {
	name string
}

// NewKey 创建类型安全的 context key
func NewKey[T any](name string) contextKey[T] {
	return contextKey[T]{name: name}
}

// WithValue 使用类型安全的 key 设置值
func WithValue[T any](ctx context.Context, key contextKey[T], value T) context.Context {
	return context.WithValue(ctx, key, value)
}

// Value 使用类型安全的 key 获取值
func Value[T any](ctx context.Context, key contextKey[T]) (T, bool) {
	v, ok := ctx.Value(key).(T)
	return v, ok
}

// MustValue 使用类型安全的 key 获取值，不存在则 panic
//
// 警告：仅建议在程序初始化阶段使用。在请求处理路径中，建议使用 Value 或 ValueOr。
// 在生产环境中，panic 可能导致服务中断。
func MustValue[T any](ctx context.Context, key contextKey[T]) T {
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
func TryValue[T any](ctx context.Context, key contextKey[T]) (T, error) {
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
func ValueOr[T any](ctx context.Context, key contextKey[T], defaultValue T) T {
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

// Go 在 goroutine 中运行函数
// 注意：函数内部应该自行检查 ctx.Done() 来响应取消
// 此函数只在启动时检查 context 是否已取消，不会中断正在执行的函数
func Go(ctx context.Context, fn func(ctx context.Context)) {
	go func() {
		// 在启动 goroutine 后检查 context 状态
		if ctx.Err() != nil {
			return
		}
		fn(ctx)
	}()
}

// Run 运行函数直到 context 取消或函数完成
func Run(ctx context.Context, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// RunTimeout 带超时运行函数
func RunTimeout(timeout time.Duration, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	ctx, cancel := context.WithCancel(context.Background())

	if len(contexts) == 0 {
		return ctx, cancel
	}

	// 使用 context.AfterFunc 监听所有 context
	// 当任意一个 context 取消时，取消合并后的 context
	// AfterFunc 在 context 取消时会自动清理，不会泄漏
	stopFuncs := make([]func() bool, 0, len(contexts))
	for _, c := range contexts {
		stop := context.AfterFunc(c, cancel)
		stopFuncs = append(stopFuncs, stop)
	}

	// 包装 cancel 函数，确保清理所有 AfterFunc
	wrappedCancel := func() {
		cancel()
		for _, stop := range stopFuncs {
			stop()
		}
	}

	// 返回合并的 context，值查找使用第一个 context
	return &mergedContext{
		Context:  ctx,
		contexts: contexts,
	}, wrappedCancel
}

type mergedContext struct {
	context.Context
	contexts []context.Context
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
	wg  sync.WaitGroup
	ctx context.Context
}

// NewWaitGroupContext 创建带 context 的 WaitGroup
func NewWaitGroupContext(ctx context.Context) *WaitGroupContext {
	return &WaitGroupContext{ctx: ctx}
}

// Go 启动一个 goroutine
func (w *WaitGroupContext) Go(fn func(ctx context.Context) error) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		fn(w.ctx)
	}()
}

// Wait 等待所有 goroutine 完成或 context 取消
func (w *WaitGroupContext) Wait() error {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

// --- Pool ---

// Pool 带 context 的协程池
type Pool struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	sem    chan struct{}
}

// NewPool 创建协程池
func NewPool(ctx context.Context, size int) *Pool {
	ctx, cancel := context.WithCancel(ctx)
	return &Pool{
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, size),
	}
}

// Go 在池中启动任务
func (p *Pool) Go(fn func(ctx context.Context) error) {
	select {
	case <-p.ctx.Done():
		return
	case p.sem <- struct{}{}:
	}

	p.wg.Add(1)
	go func() {
		defer func() {
			<-p.sem
			p.wg.Done()
		}()
		fn(p.ctx)
	}()
}

// Wait 等待所有任务完成
func (p *Pool) Wait() error {
	p.wg.Wait()
	return p.ctx.Err()
}

// Close 关闭池
func (p *Pool) Close() {
	p.cancel()
	p.wg.Wait()
}
