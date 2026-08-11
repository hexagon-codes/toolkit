package rand

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// MaxGeneratedLength 是字符串和字节 API 的单次安全分配上限。
const MaxGeneratedLength = 1 << 20

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
var (
	ErrInsufficientEntropy = errors.New("cryptographic entropy source failed")
	// ErrInvalidLength 表示请求长度为负数或超过单次安全分配上限。
	ErrInvalidLength = errors.New("length must be between 0 and 1048576")
	// ErrInvalidCharset 表示字符集无法提供均匀且有意义的随机采样。
	ErrInvalidCharset = errors.New("charset must contain at least two unique characters")
	// ErrInvalidRange 表示整数区间不是有效的左闭右开区间。
	ErrInvalidRange = errors.New("lower bound must be less than upper bound")
)

// stringFrom 是 StringFrom 与 TryStringFrom 共享的内部核心实现。
//
// 该函数承载真正的随机字符串生成逻辑：从 charset 中按加密安全的方式
// 逐字符采样，长度为 length。所有错误（来自 crypto/rand.Int）以 error
// 形式返回，由上层决定是 panic（StringFrom）还是传播（TryStringFrom）。
//
// 边界约定：
//   - length 为 0 时返回 ("", nil)；负数或超过 MaxGeneratedLength 时返回 ErrInvalidLength；
//   - charset 必须包含至少两个互不重复的字符，否则返回 ErrInvalidCharset。
//
// 返回值：
//   - string: 生成的随机字符串；出错时为 ""。
//   - error:  底层熵源失败时返回包装了 ErrInsufficientEntropy 的错误；否则为 nil。
func stringFrom(reader io.Reader, charset string, length int) (string, error) {
	if err := validateLength(length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}

	charsetRunes := []rune(charset)
	if len(charsetRunes) < 2 || containsDuplicateRunes(charsetRunes) {
		return "", fmt.Errorf("%w: characters=%d", ErrInvalidCharset, len(charsetRunes))
	}

	result := make([]rune, length)
	bound := big.NewInt(int64(len(charsetRunes)))

	for i := 0; i < length; i++ {
		num, err := rand.Int(reader, bound)
		if err != nil {
			// 包装底层错误并附带 ErrInsufficientEntropy 哨兵，便于上层 errors.Is 判定。
			return "", fmt.Errorf("%w: crypto/rand.Int failed: %w", ErrInsufficientEntropy, err)
		}
		result[i] = charsetRunes[num.Int64()]
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
//   - length:  目标字符串长度。0 返回空串，其他值必须位于安全分配边界内。
//
// 返回：
//   - string: 生成的随机字符串；出错时为空串。
//   - error: 输入无效或底层 crypto/rand 失败时返回非 nil；
//     可分别用 errors.Is 判定输入错误或熵源故障。
//
// 使用示例：
//
//	s, err := rand.TryStringFrom(rand.AlphaNumeric, 32)
//	if err != nil {
//	    return fmt.Errorf("failed to generate random string: %w", err)
//	}
func TryStringFrom(charset string, length int) (string, error) {
	return stringFrom(rand.Reader, charset, length)
}

// TryString 是 String 的错误返回安全变体（字母+数字）。
//
// 行为与 String 一致，仅在底层熵源失败时返回 error 而非 panic。
// 适用于 OAuth state、会话 token 等需要错误传播的安全凭据生成路径。
//
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
func TryString(length int) (string, error) {
	return stringFrom(rand.Reader, AlphaNumeric, length)
}

// TryNumericString 是 NumericString 的错误返回安全变体（纯数字）。
//
// 行为与 NumericString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
func TryNumericString(length int) (string, error) {
	return stringFrom(rand.Reader, Numeric, length)
}

// TryAlphaString 是 AlphaString 的错误返回安全变体（纯字母）。
//
// 行为与 AlphaString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
func TryAlphaString(length int) (string, error) {
	return stringFrom(rand.Reader, Alpha, length)
}

// TryLowerString 是 LowerString 的错误返回安全变体（小写字母）。
//
// 行为与 LowerString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
func TryLowerString(length int) (string, error) {
	return stringFrom(rand.Reader, AlphaLower, length)
}

// TryUpperString 是 UpperString 的错误返回安全变体（大写字母）。
//
// 行为与 UpperString 一致，仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
func TryUpperString(length int) (string, error) {
	return stringFrom(rand.Reader, AlphaUpper, length)
}

// TryToken 是 Token 的错误返回安全变体（字母+数字）。
//
// 行为与 Token 一致（等价于 TryString），仅在底层熵源失败时返回 error 而非 panic。
//
// 典型用途：OAuth state、CSRF token、一次性访问凭据等需要将随机数失败
// 以错误形式传播（而非 panic 击穿请求协程）的场景。
//
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
//
// 使用示例：
//
//	state, err := rand.TryToken(32)
//	if err != nil {
//	    http.Error(w, "internal server error", http.StatusInternalServerError)
//	    return
//	}
func TryToken(length int) (string, error) {
	return stringFrom(rand.Reader, AlphaNumeric, length)
}

// TryCode 是 Code 的错误返回安全变体（纯数字验证码）。
//
// 行为与 Code 一致（等价于 TryNumericString），仅在底层熵源失败时返回 error 而非 panic。
// 参数 length 为 0 时返回空串，负数或超出上限时返回 ErrInvalidLength。
func TryCode(length int) (string, error) {
	return stringFrom(rand.Reader, Numeric, length)
}

// TryInt 是 Int 的错误返回安全变体，生成范围 [min, max) 内的随机整数。
//
// 输入区间无效时返回 ErrInvalidRange，熵源失败时返回 ErrInsufficientEntropy。
//
// 返回：
//   - int:   生成的随机整数；出错时为 min。
//   - error: 底层 crypto/rand 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryInt(lower, upper int) (int, error) {
	if lower >= upper {
		return lower, fmt.Errorf("%w: lower=%d upper=%d", ErrInvalidRange, lower, upper)
	}

	num, err := randomInt64(rand.Reader, int64(lower), int64(upper))
	if err != nil {
		return lower, err
	}
	return int(num), nil
}

// TryInt64 是 Int64 的错误返回安全变体，生成范围 [min, max) 内的随机 int64。
//
// 输入区间无效时返回 ErrInvalidRange，熵源失败时返回 ErrInsufficientEntropy。
//
// 返回：
//   - int64: 生成的随机整数；出错时为 min。
//   - error: 底层 crypto/rand 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryInt64(lower, upper int64) (int64, error) {
	if lower >= upper {
		return lower, fmt.Errorf("%w: lower=%d upper=%d", ErrInvalidRange, lower, upper)
	}

	num, err := randomInt64(rand.Reader, lower, upper)
	if err != nil {
		return lower, err
	}
	return num, nil
}

// TryBytes 是 Bytes 的错误返回安全变体，生成 length 个加密安全随机字节。
//
// length 为 0 时返回 nil；负数或超出上限时返回 ErrInvalidLength。
//
// 返回：
//   - []byte: 生成的随机字节切片；出错时为 nil。
//   - error:  底层 crypto/rand.Read 失败时返回非 nil，可用 errors.Is 判定熵源故障。
func TryBytes(length int) ([]byte, error) {
	return randomBytes(rand.Reader, length)
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

// randomInt64 在 big.Int 上计算区间宽度，避免有符号整数减法溢出。
func randomInt64(reader io.Reader, lower, upper int64) (int64, error) {
	if lower >= upper {
		return lower, fmt.Errorf("%w: lower=%d upper=%d", ErrInvalidRange, lower, upper)
	}
	lowerValue := big.NewInt(lower)
	rangeSize := new(big.Int).Sub(big.NewInt(upper), lowerValue)
	num, err := rand.Int(reader, rangeSize)
	if err != nil {
		return lower, fmt.Errorf("%w: crypto/rand.Int failed: %w", ErrInsufficientEntropy, err)
	}
	return num.Add(num, lowerValue).Int64(), nil
}

func randomBytes(reader io.Reader, length int) ([]byte, error) {
	if err := validateLength(length); err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, nil
	}

	bytes := make([]byte, length)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return nil, fmt.Errorf("%w: crypto/rand.Read failed: %w", ErrInsufficientEntropy, err)
	}
	return bytes, nil
}

func validateLength(length int) error {
	if length < 0 || length > MaxGeneratedLength {
		return fmt.Errorf("%w: %d", ErrInvalidLength, length)
	}
	return nil
}

func containsDuplicateRunes(values []rune) bool {
	seen := make(map[rune]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
