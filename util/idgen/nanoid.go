package idgen

import (
	"crypto/rand"
	"math"
)

const (
	// DefaultAlphabet 默认字符集（URL 安全）
	DefaultAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"
	// DefaultSize 默认长度
	DefaultSize = 21
)

// NanoID 生成 NanoID（默认长度 21）
func NanoID() string {
	return NanoIDCustom(DefaultAlphabet, DefaultSize)
}

// NanoIDSize 生成指定长度的 NanoID
func NanoIDSize(size int) string {
	return NanoIDCustom(DefaultAlphabet, size)
}

// NanoIDCustom 生成自定义字符集和长度的 ID
func NanoIDCustom(alphabet string, size int) string {
	if size <= 0 {
		size = DefaultSize
	}
	if len(alphabet) == 0 {
		alphabet = DefaultAlphabet
	}

	alphabetLen := len(alphabet)
	mask := (2 << uint(math.Log2(float64(alphabetLen-1)))) - 1
	step := int(math.Ceil(1.6 * float64(mask*size) / float64(alphabetLen)))

	id := make([]byte, size)
	bytes := make([]byte, step)

	for i, j := 0, 0; ; {
		if _, err := rand.Read(bytes); err != nil {
			// crypto/rand 失败是严重问题，panic 比返回弱随机数更安全
			panic("crypto/rand.Read failed: " + err.Error())
		}

		for _, b := range bytes {
			idx := int(b) & mask
			// 检查索引是否在有效范围内
			if idx < alphabetLen {
				id[i] = alphabet[idx]
				i++
				if i == size {
					return string(id)
				}
			}

			j++
			if j == step {
				break
			}
		}
	}
}

// ShortID 生成短 ID（8 位）
func ShortID() string {
	return NanoIDSize(8)
}

// MediumID 生成中等长度 ID（16 位）
func MediumID() string {
	return NanoIDSize(16)
}
