package rand

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
)

// ErrInsufficientEntropy 表示底层加密熵源（crypto/rand）读取失败。
//
// 该错误通常在系统熵源不可用时出现（例如容器内 /dev/urandom 被禁用、
// 沙箱环境限制、内核熵池异常等），属于极少发生但必须可处理的场景。
//
// 本包提供的 Try* 系列函数在底层熵源失败时返回包装了该错误的值，
// 调用方可通过 errors.Is(err, rand.ErrInsufficientEntropy) 判定。
//
// 设计动机：
//   - String/StringFrom/Token/Int/Int64/Bytes 等函数在熵源失败时直接 panic，
//     适合"随机数失败即视为致命错误"的场景；
//   - 但在 OAuth state、CSRF token、一次性凭据等生成路径上，
//     调用方更希望以 error 形式优雅传播（返回 5xx 或重试），而非 panic 击穿协程。
//
// 因此提供 Try* 安全变体：行为与对应 panic 版完全一致，仅将 panic 替换为 error 返回。
var ErrInsufficientEntropy = errors.New("rand: 加密熵源读取失败")

// stringFrom 是 StringFrom 与 TryStringFrom 共享的内部核心实现。
//
// 该函数承载真正的随机字符串生成逻辑：从 charset 中按加密安全的方式
// 逐字符采样，长度为 length。所有错误（来自 crypto/rand.Int）以 error
// 形式返回，由上层决定是 panic（StringFrom）还是传播（TryStringFrom）。
//
// 边界约定（与原 StringFrom 保持完全一致）：
//   - length <= 0 时返回 ("", nil)，不视为错误；
//   - charset 为空字符串且 length > 0 时，无法采样，返回 ("", error)，
//     以避免对 big.NewInt(0) 调用 rand.Int 触发的 panic（保持安全语义）。
//
// 返回值：
//   - string: 生成的随机字符串；出错时为 ""。
//   - error:  底层熵源失败时返回包装了 ErrInsufficientEntropy 的错误；否则为 nil。
func stringFrom(charset string, length int) (string, error) {
	if length <= 0 {
		return "", nil
	}

	// charset 为空时无法采样：big.NewInt(0) 传给 rand.Int 会 panic，
	// 这里提前拦截并以 error 形式返回，保证 Try* 系列绝不 panic。
	charsetLen := len(charset)
	if charsetLen == 0 {
		return "", fmt.Errorf("rand: 字符集为空，无法生成长度为 %d 的随机字符串", length)
	}

	result := make([]byte, length)
	bound := big.NewInt(int64(charsetLen))

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, bound)
		if err != nil {
			// 包装底层错误并附带 ErrInsufficientEntropy 哨兵，便于上层 errors.Is 判定。
			return "", fmt.Errorf("%w: crypto/rand.Int 调用失败: %v", ErrInsufficientEntropy, err)
		}
		result[i] = charset[num.Int64()]
	}

	return string(result), nil
}

// TryStringFrom 是 StringFrom 的错误返回安全变体。
//
// 行为与 StringFrom 完全一致（从 charset 中加密安全地采样 length 个字符），
// 唯一区别是：当底层熵源失败时，返回 error 而非 panic。
//
// 参数：
//   - charset: 候选字符集，从中逐字符均匀采样。
//   - length:  目标字符串长度。length <= 0 时返回 ("", nil)。
//
// 返回：
//   - string: 生成的随机字符串；出错时为空串。
//   - error:  charset 为空（length > 0 时）或底层 crypto/rand 失败时返回非 nil；
//     可用 errors.Is(err, ErrInsufficientEntropy) 判定是否为熵源故障。
//
// 使用示例：
//
//	s, err := rand.TryStringFrom(rand.AlphaNumeric, 32)
//	if err != nil {
//	    return fmt.Errorf("生成随机串失败: %w", err)
//	}
func TryStringFrom(charset string, length int) (string, error) {
	return stringFrom(charset, length)
}

// TryString 是 String 的错误返回安全变体（字母+数字）。
//
// 行为与 String 一致，仅在底层熵源失败时返回 error 而非 panic。
// 适用于 OAuth state、会话 token 等需要错误传播的安全凭据生成路径。
//
// 参数 length 为目标长度，length <= 0 时返回 ("", nil)。
func TryString(length int) (string, error) {
	return stringFrom(AlphaNumeric, length)
}

// TryNumericString 是 NumericString 的错误返回安全变体（纯数字）。
//
// 行为与 NumericString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为目标长度，length <= 0 时返回 ("", nil)。
func TryNumericString(length int) (string, error) {
	return stringFrom(Numeric, length)
}

// TryAlphaString 是 AlphaString 的错误返回安全变体（纯字母）。
//
// 行为与 AlphaString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为目标长度，length <= 0 时返回 ("", nil)。
func TryAlphaString(length int) (string, error) {
	return stringFrom(Alpha, length)
}

// TryLowerString 是 LowerString 的错误返回安全变体（小写字母）。
//
// 行为与 LowerString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为目标长度，length <= 0 时返回 ("", nil)。
func TryLowerString(length int) (string, error) {
	return stringFrom(AlphaLower, length)
}

// TryUpperString 是 UpperString 的错误返回安全变体（大写字母）。
//
// 行为与 UpperString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为目标长度，length <= 0 时返回 ("", nil)。
func TryUpperString(length int) (string, error) {
	return stringFrom(AlphaUpper, length)
}

// TryToken 是 Token 的错误返回安全变体（字母+数字）。
//
// 行为与 Token 一致（等价于 TryString），仅在底层熵源失败时返回 error 而非 panic。
//
// 典型用途：OAuth state、CSRF token、一次性访问凭据等需要将随机数失败
// 以错误形式传播（而非 panic 击穿请求协程）的场景。
//
// 参数 length 为目标 Token 长度，length <= 0 时返回 ("", nil)。
//
// 使用示例：
//
//	state, err := rand.TryToken(32)
//	if err != nil {
//	    http.Error(w, "内部错误", http.StatusInternalServerError)
//	    return
//	}
func TryToken(length int) (string, error) {
	return stringFrom(AlphaNumeric, length)
}

// TryCode 是 Code 的错误返回安全变体（纯数字验证码）。
//
// 行为与 Code 一致（等价于 TryNumericString），仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为目标验证码长度，length <= 0 时返回 ("", nil)。
func TryCode(length int) (string, error) {
	return stringFrom(Numeric, length)
}

// TryInt 是 Int 的错误返回安全变体，生成范围 [min, max) 内的随机整数。
//
// 行为与 Int 一致，仅在底层熵源失败时返回 error 而非 panic。
//
// 边界约定（与 Int 完全一致）：
//   - min >= max 时返回 (min, nil)，不视为错误。
//
// 返回：
//   - int:   生成的随机整数；出错时为 min。
//   - error: 底层 crypto/rand 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryInt(min, max int) (int, error) {
	if min >= max {
		return min, nil
	}

	diff := max - min
	num, err := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	if err != nil {
		return min, fmt.Errorf("%w: crypto/rand.Int 调用失败: %v", ErrInsufficientEntropy, err)
	}
	return int(num.Int64()) + min, nil
}

// TryInt64 是 Int64 的错误返回安全变体，生成范围 [min, max) 内的随机 int64。
//
// 行为与 Int64 一致，仅在底层熵源失败时返回 error 而非 panic。
//
// 边界约定（与 Int64 完全一致）：
//   - min >= max 时返回 (min, nil)，不视为错误。
//
// 返回：
//   - int64: 生成的随机整数；出错时为 min。
//   - error: 底层 crypto/rand 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryInt64(min, max int64) (int64, error) {
	if min >= max {
		return min, nil
	}

	diff := max - min
	num, err := rand.Int(rand.Reader, big.NewInt(diff))
	if err != nil {
		return min, fmt.Errorf("%w: crypto/rand.Int 调用失败: %v", ErrInsufficientEntropy, err)
	}
	return num.Int64() + min, nil
}

// TryBytes 是 Bytes 的错误返回安全变体，生成 length 个加密安全随机字节。
//
// 行为与 Bytes 一致，仅在底层熵源失败时返回 error 而非 panic。
//
// 边界约定（与 Bytes 完全一致）：
//   - length <= 0 时返回 (nil, nil)，不视为错误。
//
// 返回：
//   - []byte: 生成的随机字节切片；出错时为 nil。
//   - error:  底层 crypto/rand.Read 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryBytes(length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("%w: crypto/rand.Read 调用失败: %v", ErrInsufficientEntropy, err)
	}
	return b, nil
}

// TryBool 是 Bool 的错误返回安全变体，生成随机布尔值。
//
// 行为与 Bool 一致，仅在底层熵源失败时返回 error 而非 panic。
//
// 返回：
//   - bool:  随机布尔值；出错时为 false。
//   - error: 底层 crypto/rand 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryBool() (bool, error) {
	n, err := TryInt(0, 2)
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
