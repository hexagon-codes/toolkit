package encoding

import (
	"encoding/base64"
)

// Base64Encode 标准 Base64 编码
func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// Base64EncodeString 标准 Base64 编码（字符串输入）
func Base64EncodeString(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// Base64Decode 标准 Base64 解码
func Base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// Base64DecodeString 标准 Base64 解码（返回字符串）
func Base64DecodeString(s string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Base64URLEncode URL 安全的 Base64 编码
func Base64URLEncode(data []byte) string {
	return base64.URLEncoding.EncodeToString(data)
}

// Base64URLEncodeString URL 安全的 Base64 编码（字符串输入）
func Base64URLEncodeString(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

// Base64URLDecode URL 安全的 Base64 解码
func Base64URLDecode(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}

// Base64URLDecodeString URL 安全的 Base64 解码（返回字符串）
func Base64URLDecodeString(s string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Base64RawEncode 无填充的 Base64 编码
func Base64RawEncode(data []byte) string {
	return base64.RawStdEncoding.EncodeToString(data)
}

// Base64RawDecode 无填充的 Base64 解码
func Base64RawDecode(s string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(s)
}

// Base64RawURLEncode 无填充的 URL 安全 Base64 编码
func Base64RawURLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// Base64RawURLDecode 无填充的 URL 安全 Base64 解码
func Base64RawURLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
