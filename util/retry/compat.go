package retry

import "fmt"

// 本文件提供纯增量的兼容性增强选项，用于满足下游框架（如 hexagon）对
// 错误链可解包语义与回调计数基准的差异化需求。
//
// 设计原则（务必遵守，保证绝对向后兼容）：
//   - 不改变任何既有导出签名（Do / DoWithContext / Option / Config 等）。
//   - 不改变任何默认行为：所有新语义均需调用方显式通过 Option 开启。
//   - 仅在 Config 上追加新字段（追加字段对既有零值构造完全兼容）。
//
// 背景（W5 实证）：
//   早期 Do / DoWithContext 在重试耗尽时返回
//       fmt.Errorf("%w: %v", ErrMaxAttemptsReached, lastErr)
//   其中原始错误 lastErr 使用 %v 嵌入字符串，导致返回值的错误链中
//   只挂载了 ErrMaxAttemptsReached 这一个 sentinel，无法被
//   errors.Is(返回值, lastErr) 命中（恒为 false）。
//   下游若断言 errors.Is(err, errPrimary) 便无法成立，阻碍能力下沉。
//
//   增强方案：新增 WithUnwrapFinalError()（别名 WithReturnLastError()），
//   开启后最终错误改用多 %w 包装：
//       fmt.Errorf("%w: %w", ErrMaxAttemptsReached, lastErr)
//   既保留 sentinel ErrMaxAttemptsReached，又能 errors.Unwrap 到原始
//   lastErr，从而同时满足：
//       errors.Is(返回值, ErrMaxAttemptsReached) == true
//       errors.Is(返回值, lastErr)               == true
//   多 %w 自 Go 1.20 起由标准库 fmt 原生支持。

// WithUnwrapFinalError 让重试耗尽时返回的最终错误同时可被
// errors.Is 命中 ErrMaxAttemptsReached 与原始的最后一次错误（lastErr）。
//
// 默认（不设置本选项）行为保持不变：最终错误为
//
//	fmt.Errorf("%w: %v", ErrMaxAttemptsReached, lastErr)
//
// 其错误链仅包含 ErrMaxAttemptsReached，errors.Is(err, lastErr) 返回 false。
//
// 开启本选项后，最终错误改为多 %w 包装：
//
//	fmt.Errorf("%w: %w", ErrMaxAttemptsReached, lastErr)
//
// 此时同时满足：
//
//	errors.Is(err, ErrMaxAttemptsReached) == true  // sentinel 仍在
//	errors.Is(err, lastErr)               == true  // 原始错误可解包
//
// 适用场景：下游调用方需要在重试耗尽后对具体原始错误做类型判定或
// errors.Is / errors.As 断言（例如区分业务错误码、超时、上游 5xx 等）。
//
// 注意：本选项仅影响"重试耗尽（达到最大尝试次数）"这一路径返回的错误。
// 对于 RetryIf 判定为不可重试而提前返回的错误、以及 DoWithContext 在
// 上下文取消/超时返回的 ctx.Err()，本选项均不介入（这些路径本就直接
// 返回原始错误，无需包装）。
func WithUnwrapFinalError() Option {
	return func(c *Config) {
		c.unwrapFinalError = true
	}
}

// WithReturnLastError 是 WithUnwrapFinalError 的语义别名。
//
// 二者效果完全一致：开启后重试耗尽返回的最终错误可被
// errors.Is 解包到原始的最后一次错误（lastErr），同时保留
// ErrMaxAttemptsReached sentinel。
//
// 提供别名是为了贴合下游不同的命名习惯（"返回最后一次错误" vs
// "可解包最终错误"），调用方按语义偏好任选其一即可。
func WithReturnLastError() Option {
	return WithUnwrapFinalError()
}

// WithOnRetryZeroBased 将 OnRetry 回调的次数计数切换为零基（zero-based）。
//
// 默认（不设置本选项）行为保持不变——OnRetry 采用一基计数：
//   - 首次重试回调 n == 1
//   - 第二次重试回调 n == 2
//   - 以此类推
//
// 开启本选项后，OnRetry 采用零基计数：
//   - 首次重试回调 n == 0
//   - 第二次重试回调 n == 1
//   - 以此类推
//
// 设计背景：部分下游框架（如 hexagon）约定回调以零基语义表达
// "已发生的重试次数"（首次重试时已重试 0 次）。本选项允许在不改变
// toolkit 默认一基语义的前提下，按需对齐下游期望。
//
// 注意：
//   - 本选项不影响 OnRetry 的"调用时机"与"调用次数"，仅平移传入的计数值。
//     OnRetry 仍在每次失败且将要重试前调用，最后一次失败（不再重试）不调用。
//   - 本选项不影响延迟计算（calculateDelay 内部仍使用原始一基的尝试序号），
//     因此退避曲线与抖动行为完全不变。
//   - 若未设置 OnRetry 回调，本选项无任何可观察效果。
func WithOnRetryZeroBased() Option {
	return func(c *Config) {
		c.onRetryZeroBased = true
	}
}

// finalError 根据配置构造重试耗尽时返回的最终错误。
//
// 默认保持历史行为（%w + %v，仅挂载 sentinel）；当开启
// unwrapFinalError 时改用多 %w，使 lastErr 进入错误链可被解包。
//
// lastErr 为最后一次执行失败的原始错误；调用本函数的前提是确实
// 发生过至少一次失败（lastErr != nil）。即便 lastErr 为 nil，
// 两种格式化方式也都能安全工作（不会 panic），仅文案略有差异。
func (c *Config) finalError(lastErr error) error {
	if c.unwrapFinalError {
		// 多 %w：错误链同时包含 ErrMaxAttemptsReached 与 lastErr。
		return fmt.Errorf("%w: %w", ErrMaxAttemptsReached, lastErr)
	}
	// 历史默认：%v 仅做字符串嵌入，错误链只含 ErrMaxAttemptsReached。
	return fmt.Errorf("%w: %v", ErrMaxAttemptsReached, lastErr)
}

// onRetryNumber 根据计数基准将内部一基的尝试序号转换为对外回调的次数。
//
// internalAttempt 为内部循环使用的一基尝试序号（首次失败时为 1）。
// 默认返回一基序号（保持历史行为）；开启 onRetryZeroBased 时减 1
// 转为零基序号（首次重试回调 0）。
func (c *Config) onRetryNumber(internalAttempt int) int {
	if c.onRetryZeroBased {
		return internalAttempt - 1
	}
	return internalAttempt
}

// invokeOnRetry 在配置了 OnRetry 时按当前计数基准触发回调。
//
// 将"是否配置回调 + 计数基准换算"的判定集中于此，便于 Do 与
// DoWithContext 复用同一逻辑，避免在两处分别内联导致行为漂移。
func (c *Config) invokeOnRetry(internalAttempt int, err error) {
	if c.OnRetry != nil {
		c.OnRetry(c.onRetryNumber(internalAttempt), err)
	}
}
