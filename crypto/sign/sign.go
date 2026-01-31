package sign

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"time"
)

// --- HMAC 签名 ---

// HMACSHA256 使用 HMAC-SHA256 签名
func HMACSHA256(message, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

// HMACSHA256Hex 返回 Hex 编码
func HMACSHA256Hex(message, key []byte) string {
	return hex.EncodeToString(HMACSHA256(message, key))
}

// HMACSHA256Base64 返回 Base64 编码
func HMACSHA256Base64(message, key []byte) string {
	return base64.StdEncoding.EncodeToString(HMACSHA256(message, key))
}

// HMACSHA256String 字符串签名
func HMACSHA256String(message, key string) string {
	return HMACSHA256Hex([]byte(message), []byte(key))
}

// HMACSHA512 使用 HMAC-SHA512 签名
func HMACSHA512(message, key []byte) []byte {
	h := hmac.New(sha512.New, key)
	h.Write(message)
	return h.Sum(nil)
}

// HMACSHA512Hex 返回 Hex 编码
func HMACSHA512Hex(message, key []byte) string {
	return hex.EncodeToString(HMACSHA512(message, key))
}

// HMACSHA512Base64 返回 Base64 编码
func HMACSHA512Base64(message, key []byte) string {
	return base64.StdEncoding.EncodeToString(HMACSHA512(message, key))
}

// HMACSHA512String 字符串签名
func HMACSHA512String(message, key string) string {
	return HMACSHA512Hex([]byte(message), []byte(key))
}

// --- HMAC 验证 ---

// VerifyHMACSHA256 验证 HMAC-SHA256 签名
func VerifyHMACSHA256(message, key, signature []byte) bool {
	expected := HMACSHA256(message, key)
	return hmac.Equal(expected, signature)
}

// VerifyHMACSHA256Hex 验证 Hex 编码的签名
func VerifyHMACSHA256Hex(message, key []byte, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return VerifyHMACSHA256(message, key, signature)
}

// VerifyHMACSHA256Base64 验证 Base64 编码的签名
func VerifyHMACSHA256Base64(message, key []byte, signatureBase64 string) bool {
	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false
	}
	return VerifyHMACSHA256(message, key, signature)
}

// VerifyHMACSHA256String 验证字符串签名
func VerifyHMACSHA256String(message, key, signatureHex string) bool {
	return VerifyHMACSHA256Hex([]byte(message), []byte(key), signatureHex)
}

// VerifyHMACSHA512 验证 HMAC-SHA512 签名
func VerifyHMACSHA512(message, key, signature []byte) bool {
	expected := HMACSHA512(message, key)
	return hmac.Equal(expected, signature)
}

// VerifyHMACSHA512Hex 验证 Hex 编码的签名
func VerifyHMACSHA512Hex(message, key []byte, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return VerifyHMACSHA512(message, key, signature)
}

// VerifyHMACSHA512String 验证字符串签名
func VerifyHMACSHA512String(message, key, signatureHex string) bool {
	return VerifyHMACSHA512Hex([]byte(message), []byte(key), signatureHex)
}

// --- 通用 HMAC ---

// HMACHash 哈希算法类型
type HMACHash int

const (
	SHA256 HMACHash = iota
	SHA512
	SHA384
	SHA224
)

// HMAC 使用指定哈希算法计算 HMAC
func HMAC(message, key []byte, hashType HMACHash) []byte {
	var h func() hash.Hash
	switch hashType {
	case SHA256:
		h = sha256.New
	case SHA512:
		h = sha512.New
	case SHA384:
		h = sha512.New384
	case SHA224:
		h = sha256.New224
	default:
		h = sha256.New
	}

	mac := hmac.New(h, key)
	mac.Write(message)
	return mac.Sum(nil)
}

// HMACHex 返回 Hex 编码
func HMACHex(message, key []byte, hashType HMACHash) string {
	return hex.EncodeToString(HMAC(message, key, hashType))
}

// VerifyHMAC 验证 HMAC 签名
func VerifyHMAC(message, key, signature []byte, hashType HMACHash) bool {
	expected := HMAC(message, key, hashType)
	return hmac.Equal(expected, signature)
}

// --- 时间戳签名 ---

// TimestampSigner 带时间戳的签名器
type TimestampSigner struct {
	key      []byte
	hashType HMACHash
}

// NewTimestampSigner 创建时间戳签名器
func NewTimestampSigner(key []byte) *TimestampSigner {
	return &TimestampSigner{
		key:      key,
		hashType: SHA256,
	}
}

// NewTimestampSignerWithHash 创建指定哈希算法的时间戳签名器
func NewTimestampSignerWithHash(key []byte, hashType HMACHash) *TimestampSigner {
	return &TimestampSigner{
		key:      key,
		hashType: hashType,
	}
}

// Sign 签名（消息 + 时间戳）
func (s *TimestampSigner) Sign(message string, timestamp int64) string {
	data := message + ":" + formatInt64(timestamp)
	return HMACHex([]byte(data), s.key, s.hashType)
}

// Verify 验证签名
// 注意：此方法不检查时间戳过期，可能受重放攻击
// 推荐使用 VerifyWithExpiry 进行时间戳验证
func (s *TimestampSigner) Verify(message string, timestamp int64, signature string) bool {
	expected := s.Sign(message, timestamp)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// VerifyWithExpiry 验证签名并检查时间戳是否过期
// maxAge: 签名的最大有效期（秒，例如 300 表示 5 分钟）
// 返回 false 如果签名无效或时间戳已过期
func (s *TimestampSigner) VerifyWithExpiry(message string, timestamp int64, signature string, maxAge int64) bool {
	// 检查时间戳是否过期
	now := time.Now().Unix()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff // 绝对值
	}
	if diff > maxAge {
		return false // 时间戳过期或来自未来
	}
	return s.Verify(message, timestamp, signature)
}

// formatInt64 格式化 int64 为字符串
func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	var buf [20]byte
	i := len(buf)

	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}

	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// --- API 签名 ---

// APISigner API 签名器
type APISigner struct {
	appKey    string
	appSecret string
}

// NewAPISigner 创建 API 签名器
func NewAPISigner(appKey, appSecret string) *APISigner {
	return &APISigner{
		appKey:    appKey,
		appSecret: appSecret,
	}
}

// Sign 签名请求参数
// 签名算法：HMAC-SHA256(sortedParams + timestamp + nonce, appSecret)
func (s *APISigner) Sign(params map[string]string, timestamp int64, nonce string) string {
	// 按 key 排序拼接参数
	sortedParams := sortAndJoinParams(params)

	// 拼接签名字符串
	signStr := sortedParams + formatInt64(timestamp) + nonce

	// 计算签名
	return HMACSHA256Hex([]byte(signStr), []byte(s.appSecret))
}

// Verify 验证签名
// 注意：此方法不检查时间戳过期，可能受重放攻击
// 推荐使用 VerifyWithExpiry 进行时间戳验证
func (s *APISigner) Verify(params map[string]string, timestamp int64, nonce, signature string) bool {
	expected := s.Sign(params, timestamp, nonce)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// VerifyWithExpiry 验证签名并检查时间戳是否过期
// maxAge: 签名的最大有效期（秒，例如 300 表示 5 分钟）
// 返回 false 如果签名无效或时间戳已过期
// 注意：调用方仍需自行检查 nonce 唯一性以完全防止重放攻击
func (s *APISigner) VerifyWithExpiry(params map[string]string, timestamp int64, nonce, signature string, maxAge int64) bool {
	// 检查时间戳是否过期
	now := time.Now().Unix()
	diff := now - timestamp
	if diff < 0 {
		diff = -diff // 绝对值
	}
	if diff > maxAge {
		return false // 时间戳过期或来自未来
	}
	return s.Verify(params, timestamp, nonce, signature)
}

// sortAndJoinParams 排序并拼接参数
func sortAndJoinParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	// 获取所有 key 并排序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sortStrings(keys)

	// 拼接
	var result string
	for _, k := range keys {
		result += k + "=" + params[k] + "&"
	}

	// 移除最后的 &
	if len(result) > 0 {
		result = result[:len(result)-1]
	}

	return result
}

// sortStrings 字符串排序（快速排序，避免导入 sort 包以保持零依赖）
func sortStrings(s []string) {
	if len(s) <= 1 {
		return
	}
	quickSort(s, 0, len(s)-1)
}

func quickSort(s []string, low, high int) {
	if low < high {
		pivot := partition(s, low, high)
		quickSort(s, low, pivot-1)
		quickSort(s, pivot+1, high)
	}
}

func partition(s []string, low, high int) int {
	pivot := s[high]
	i := low - 1
	for j := low; j < high; j++ {
		if s[j] <= pivot {
			i++
			s[i], s[j] = s[j], s[i]
		}
	}
	s[i+1], s[high] = s[high], s[i+1]
	return i + 1
}
