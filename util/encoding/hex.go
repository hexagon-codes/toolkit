package encoding

import (
	"encoding/hex"
)

// HexEncode 十六进制编码
func HexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

// HexEncodeString 十六进制编码（字符串输入）
func HexEncodeString(s string) string {
	return hex.EncodeToString([]byte(s))
}

// HexDecode 十六进制解码
func HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// HexDecodeString 十六进制解码（返回字符串）
func HexDecodeString(s string) (string, error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HexEncodeUpper 大写十六进制编码
func HexEncodeUpper(data []byte) string {
	const hextable = "0123456789ABCDEF"
	dst := make([]byte, hex.EncodedLen(len(data)))
	j := 0
	for _, v := range data {
		dst[j] = hextable[v>>4]
		dst[j+1] = hextable[v&0x0f]
		j += 2
	}
	return string(dst)
}
