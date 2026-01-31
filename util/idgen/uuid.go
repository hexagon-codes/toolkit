package idgen

import (
	"github.com/google/uuid"
)

// UUID 生成 UUID v4
func UUID() string {
	return uuid.New().String()
}

// UUIDWithoutHyphen 生成不带连字符的 UUID
func UUIDWithoutHyphen() string {
	id := uuid.New()
	s := id.String()
	// 直接构建，避免多次字符串分配
	return s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
}

// MustUUID 生成 UUID，如果失败则 panic
func MustUUID() string {
	return UUID()
}
