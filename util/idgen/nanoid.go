package idgen

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/bits"
)

const (
	// DefaultAlphabet 默认字符集（URL 安全）
	DefaultAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"
	// DefaultSize 默认长度
	DefaultSize = 21

	// maxNanoIDAlphabetSize 受单字节拒绝采样的索引空间限制。
	maxNanoIDAlphabetSize = 256
	// maxNanoIDSize 限制单次分配，避免不可信长度导致进程内存耗尽。
	maxNanoIDSize = 1 << 20
)

var (
	// ErrInvalidAlphabet 表示 NanoID 字符集无法提供均匀且有意义的随机采样。
	ErrInvalidAlphabet = errors.New("alphabet must contain between 2 and 256 unique characters")
	// ErrInvalidSize 表示 NanoID 长度超出安全分配边界。
	ErrInvalidSize = errors.New("size must be between 1 and 1048576")
	// ErrInsufficientEntropy 表示操作系统加密熵源读取失败。
	ErrInsufficientEntropy = errors.New("cryptographic entropy source failed")
)

// NanoID 生成 NanoID（默认长度 21）
func NanoID() string {
	id, err := TryNanoID()
	if err != nil {
		panic(err)
	}
	return id
}

// TryNanoID 生成默认长度的 NanoID，并将熵源故障作为错误返回。
func TryNanoID() (string, error) {
	return generateNanoID(rand.Reader, DefaultAlphabet, DefaultSize)
}

// NanoIDSize 生成指定长度的 NanoID
func NanoIDSize(size int) string {
	id, err := TryNanoIDSize(size)
	if err != nil {
		panic(err)
	}
	return id
}

// TryNanoIDSize 生成指定长度的 NanoID，并将输入或熵源故障作为错误返回。
func TryNanoIDSize(size int) (string, error) {
	return generateNanoID(rand.Reader, DefaultAlphabet, size)
}

// NanoIDCustom 生成自定义字符集和长度的 ID
func NanoIDCustom(alphabet string, size int) string {
	id, err := TryNanoIDCustom(alphabet, size)
	if err != nil {
		panic(err)
	}
	return id
}

// TryNanoIDCustom 使用自定义字符集生成 NanoID，并将输入或熵源故障作为错误返回。
func TryNanoIDCustom(alphabet string, size int) (string, error) {
	return generateNanoID(rand.Reader, alphabet, size)
}

// generateNanoID 是所有 NanoID API 共享的验证与无偏采样实现。
func generateNanoID(reader io.Reader, alphabet string, size int) (string, error) {
	if size < 1 || size > maxNanoIDSize {
		return "", fmt.Errorf("%w: %d", ErrInvalidSize, size)
	}

	alphabetRunes := []rune(alphabet)
	if len(alphabetRunes) < 2 || len(alphabetRunes) > maxNanoIDAlphabetSize || hasDuplicateRunes(alphabetRunes) {
		return "", fmt.Errorf("%w: characters=%d", ErrInvalidAlphabet, len(alphabetRunes))
	}

	// 取不小于字符集大小的二次幂减一作为掩码，并通过拒绝采样消除取模偏差。
	mask := 1<<bits.Len(uint(len(alphabetRunes)-1)) - 1
	stepNumerator := 8*int64(mask)*int64(size) + 5*int64(len(alphabetRunes)) - 1
	step := int(stepNumerator / (5 * int64(len(alphabetRunes))))
	if step < 1 {
		step = 1
	}

	id := make([]rune, size)
	randomBytes := make([]byte, step)
	for index := 0; index < size; {
		if _, err := io.ReadFull(reader, randomBytes); err != nil {
			return "", fmt.Errorf("%w: %w", ErrInsufficientEntropy, err)
		}
		for _, value := range randomBytes {
			alphabetIndex := int(value) & mask
			if alphabetIndex >= len(alphabetRunes) {
				continue
			}
			id[index] = alphabetRunes[alphabetIndex]
			index++
			if index == size {
				return string(id), nil
			}
		}
	}
	return string(id), nil
}

func hasDuplicateRunes(values []rune) bool {
	seen := make(map[rune]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

// ShortID 生成短 ID（8 位）
func ShortID() string {
	return NanoIDSize(8)
}

// MediumID 生成中等长度 ID（16 位）
func MediumID() string {
	return NanoIDSize(16)
}
