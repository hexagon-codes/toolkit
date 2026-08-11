package idgen

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/google/uuid"
)

// UUID 生成 UUID v4
func UUID() string {
	id, err := TryUUID()
	if err != nil {
		panic(err)
	}
	return id
}

// TryUUID 生成 UUID v4，并将熵源故障作为错误返回。
func TryUUID() (string, error) {
	return generateUUID(rand.Reader, false)
}

// UUIDWithoutHyphen 生成不带连字符的 UUID
func UUIDWithoutHyphen() string {
	id, err := TryUUIDWithoutHyphen()
	if err != nil {
		panic(err)
	}
	return id
}

// TryUUIDWithoutHyphen 生成无连字符 UUID v4，并将熵源故障作为错误返回。
func TryUUIDWithoutHyphen() (string, error) {
	return generateUUID(rand.Reader, true)
}

// MustUUID 生成 UUID，如果失败则 panic
func MustUUID() string {
	return UUID()
}

// generateUUID 是所有 UUID API 共享的熵源读取与格式化实现。
func generateUUID(reader io.Reader, compact bool) (string, error) {
	id, err := uuid.NewRandomFromReader(reader)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInsufficientEntropy, err)
	}
	formatted := id.String()
	if !compact {
		return formatted, nil
	}
	// UUID 的标准格式长度固定，直接拼接可避免额外扫描。
	return formatted[0:8] + formatted[9:13] + formatted[14:18] + formatted[19:23] + formatted[24:], nil
}
