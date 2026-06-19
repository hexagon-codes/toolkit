package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// errPrimary 模拟下游真正关心的原始业务错误。
// 下游断言 errors.Is(返回值, errPrimary)，验证最终错误可解包到它。
var errPrimary = errors.New("primary upstream error")

// TestWithUnwrapFinalError_ErrorsIsBothChains 验证开启 WithUnwrapFinalError 后，
// 重试耗尽返回的最终错误同时可被 errors.Is 命中 ErrMaxAttemptsReached 与原始 lastErr。
func TestWithUnwrapFinalError_ErrorsIsBothChains(t *testing.T) {
	tests := []struct {
		name           string                                // 子用例名称
		opt            Option                                // 被测的解包选项（含别名）
		wantSentinelIs bool                                  // 期望 errors.Is(err, ErrMaxAttemptsReached)
		wantPrimaryIs  bool                                  // 期望 errors.Is(err, errPrimary)
		run            func(fn func() error, o Option) error // 执行入口（Do / DoWithContext）
	}{
		{
			name:           "Do 开启解包：sentinel 与原始错误均可命中",
			opt:            WithUnwrapFinalError(),
			wantSentinelIs: true,
			wantPrimaryIs:  true,
			run: func(fn func() error, o Option) error {
				return Do(fn, Attempts(3), Delay(time.Millisecond), o)
			},
		},
		{
			name:           "Do 默认不开启：仅 sentinel 命中，原始错误不可解包",
			opt:            func(*Config) {}, // 空选项，保持默认行为
			wantSentinelIs: true,
			wantPrimaryIs:  false,
			run: func(fn func() error, o Option) error {
				return Do(fn, Attempts(3), Delay(time.Millisecond), o)
			},
		},
		{
			name:           "WithReturnLastError 别名：等价于 WithUnwrapFinalError",
			opt:            WithReturnLastError(),
			wantSentinelIs: true,
			wantPrimaryIs:  true,
			run: func(fn func() error, o Option) error {
				return Do(fn, Attempts(3), Delay(time.Millisecond), o)
			},
		},
		{
			name:           "DoWithContext 开启解包：sentinel 与原始错误均可命中",
			opt:            WithUnwrapFinalError(),
			wantSentinelIs: true,
			wantPrimaryIs:  true,
			run: func(fn func() error, o Option) error {
				return DoWithContext(context.Background(), fn, Attempts(3), Delay(time.Millisecond), o)
			},
		},
		{
			name:           "DoWithContext 默认不开启：原始错误不可解包",
			opt:            func(*Config) {},
			wantSentinelIs: true,
			wantPrimaryIs:  false,
			run: func(fn func() error, o Option) error {
				return DoWithContext(context.Background(), fn, Attempts(3), Delay(time.Millisecond), o)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 每次执行都返回 errPrimary，确保重试必然耗尽
			err := tt.run(func() error { return errPrimary }, tt.opt)
			if err == nil {
				t.Fatal("期望返回非 nil 错误（重试耗尽），实际为 nil")
			}
			if got := errors.Is(err, ErrMaxAttemptsReached); got != tt.wantSentinelIs {
				t.Errorf("errors.Is(err, ErrMaxAttemptsReached) = %v, 期望 %v", got, tt.wantSentinelIs)
			}
			if got := errors.Is(err, errPrimary); got != tt.wantPrimaryIs {
				t.Errorf("errors.Is(err, errPrimary) = %v, 期望 %v", got, tt.wantPrimaryIs)
			}
		})
	}
}

// TestWithUnwrapFinalError_ErrorsAs 验证开启解包后可用 errors.As 取出具体错误类型。
// 这是下游区分上游 5xx / 业务错误码等场景的核心能力。
func TestWithUnwrapFinalError_ErrorsAs(t *testing.T) {
	// 构造一个携带状态码的 HTTPError 作为最后一次错误
	httpErr := &HTTPError{StatusCode: 503, Status: "Service Unavailable"}

	err := Do(func() error { return httpErr },
		Attempts(2),
		Delay(time.Millisecond),
		WithUnwrapFinalError(),
	)
	if err == nil {
		t.Fatal("期望返回非 nil 错误，实际为 nil")
	}

	// sentinel 仍在
	if !errors.Is(err, ErrMaxAttemptsReached) {
		t.Error("期望 errors.Is(err, ErrMaxAttemptsReached) == true")
	}

	// 可解包出具体的 *HTTPError 并读取状态码
	var got *HTTPError
	if !errors.As(err, &got) {
		t.Fatal("期望 errors.As(err, *HTTPError) == true，实际无法解包")
	}
	if got.StatusCode != 503 {
		t.Errorf("解包出的状态码 = %d, 期望 503", got.StatusCode)
	}
}

// TestWithUnwrapFinalError_DefaultUnchanged 守护默认行为零改变：
// 不开启选项时，原始错误必须无法被 errors.Is 命中（与历史 %v 语义一致）。
func TestWithUnwrapFinalError_DefaultUnchanged(t *testing.T) {
	err := Do(func() error { return errPrimary },
		Attempts(3),
		Delay(time.Millisecond),
	)
	if err == nil {
		t.Fatal("期望返回非 nil 错误，实际为 nil")
	}
	// 历史语义：sentinel 命中
	if !errors.Is(err, ErrMaxAttemptsReached) {
		t.Error("期望 errors.Is(err, ErrMaxAttemptsReached) == true（默认行为）")
	}
	// 历史语义：原始错误不可解包
	if errors.Is(err, errPrimary) {
		t.Error("默认行为下不应能 errors.Is 到原始错误（历史字符串嵌入语义）")
	}
	// 文案应仍包含原始错误的字符串（%v 嵌入），保证日志可读性不退化
	if want := errPrimary.Error(); !strings.Contains(err.Error(), want) {
		t.Errorf("错误文案 %q 应包含原始错误文本 %q", err.Error(), want)
	}
}

// TestFinalError_NilLastErr 边界：lastErr 为 nil 时两种格式化均不 panic。
// 该路径在正常 Do 流程中不会触发（耗尽必有 lastErr），此处直接测内部方法以覆盖防御性分支。
func TestFinalError_NilLastErr(t *testing.T) {
	tests := []struct {
		name   string
		unwrap bool
	}{
		{name: "默认 %w+%v 格式 nil lastErr", unwrap: false},
		{name: "解包 %w+%w 格式 nil lastErr", unwrap: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{unwrapFinalError: tt.unwrap}
			// 不应 panic
			err := c.finalError(nil)
			if err == nil {
				t.Fatal("finalError(nil) 不应返回 nil")
			}
			// sentinel 始终可命中
			if !errors.Is(err, ErrMaxAttemptsReached) {
				t.Error("nil lastErr 时仍应保留 ErrMaxAttemptsReached")
			}
		})
	}
}

// TestWithOnRetryZeroBased 验证 OnRetry 计数基准选项的正常路径与默认路径。
func TestWithOnRetryZeroBased(t *testing.T) {
	tests := []struct {
		name        string // 子用例名称
		zeroBased   bool   // 是否开启零基
		attempts    int    // 最大尝试次数
		wantNumbers []int  // 期望 OnRetry 依次收到的计数序列
	}{
		{
			name:        "默认一基：3 次尝试触发 2 次回调，计数 1,2",
			zeroBased:   false,
			attempts:    3,
			wantNumbers: []int{1, 2},
		},
		{
			name:        "零基：3 次尝试触发 2 次回调，计数 0,1",
			zeroBased:   true,
			attempts:    3,
			wantNumbers: []int{0, 1},
		},
		{
			name:        "零基边界：2 次尝试仅触发 1 次回调，计数 0",
			zeroBased:   true,
			attempts:    2,
			wantNumbers: []int{0},
		},
		{
			name:        "零基边界：1 次尝试不触发回调，计数为空",
			zeroBased:   true,
			attempts:    1,
			wantNumbers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			opts := []Option{
				Attempts(tt.attempts),
				Delay(time.Millisecond),
				OnRetry(func(n int, err error) {
					got = append(got, n)
				}),
			}
			if tt.zeroBased {
				opts = append(opts, WithOnRetryZeroBased())
			}

			_ = Do(func() error { return errTest }, opts...)

			if !equalInts(got, tt.wantNumbers) {
				t.Errorf("OnRetry 计数序列 = %v, 期望 %v", got, tt.wantNumbers)
			}
		})
	}
}

// TestWithOnRetryZeroBased_Context 验证零基计数在 DoWithContext 下同样生效。
func TestWithOnRetryZeroBased_Context(t *testing.T) {
	var got []int
	_ = DoWithContext(context.Background(), func() error { return errTest },
		Attempts(3),
		Delay(time.Millisecond),
		OnRetry(func(n int, err error) { got = append(got, n) }),
		WithOnRetryZeroBased(),
	)
	want := []int{0, 1}
	if !equalInts(got, want) {
		t.Errorf("DoWithContext 零基计数序列 = %v, 期望 %v", got, want)
	}
}

// TestWithOnRetryZeroBased_DelayUnaffected 验证切换计数基准不影响延迟计算。
// 通过指数退避下的尝试次数恒定来间接确认退避循环逻辑未被破坏。
func TestWithOnRetryZeroBased_DelayUnaffected(t *testing.T) {
	attempts := 0
	_ = Do(func() error {
		attempts++
		return errTest
	},
		Attempts(4),
		Delay(time.Millisecond),
		DelayType(ExponentialBackoff),
		Multiplier(2.0),
		OnRetry(func(n int, err error) {}),
		WithOnRetryZeroBased(),
	)
	if attempts != 4 {
		t.Errorf("尝试次数 = %d, 期望 4（计数基准不应影响重试循环）", attempts)
	}
}

// TestCombinedOptions 组合场景：解包 + 零基同时开启，二者互不干扰。
func TestCombinedOptions(t *testing.T) {
	var lastN int
	called := 0
	err := Do(func() error { return errPrimary },
		Attempts(3),
		Delay(time.Millisecond),
		OnRetry(func(n int, e error) {
			called++
			lastN = n
		}),
		WithUnwrapFinalError(),
		WithOnRetryZeroBased(),
	)

	// 解包语义生效
	if !errors.Is(err, errPrimary) {
		t.Error("组合选项下应能 errors.Is 到原始错误")
	}
	if !errors.Is(err, ErrMaxAttemptsReached) {
		t.Error("组合选项下 sentinel 仍应命中")
	}
	// 零基计数生效：3 次尝试 -> 2 次回调，最后一次计数为 1
	if called != 2 {
		t.Errorf("回调次数 = %d, 期望 2", called)
	}
	if lastN != 1 {
		t.Errorf("最后一次零基计数 = %d, 期望 1", lastN)
	}
}

// --- 测试辅助函数 ---

// equalInts 比较两个 int 切片是否相等（含 nil 与空切片等价处理）。
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 编译期断言：finalError 返回的错误确实实现 error 接口（防御性，保证签名稳定）。
var _ error = (&Config{}).finalError(fmt.Errorf("x"))
