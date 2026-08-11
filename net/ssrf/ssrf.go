// Package ssrf 提供 URL 级别的 SSRF（Server-Side Request Forgery）防护。
//
// ValidateURL 提供请求前策略校验；Transport 与 Client 在每次真实连接时重新解析、
// 校验全部地址并只拨已校验的字面量地址，从而关闭 DNS 重绑定、重定向和环境代理
// 绕过窗口。发起不可信 URL 请求时必须使用本包的 Transport 或 Client，不能把
// ValidateURL 的一次性预检当作连接期安全边界。
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	iputil "github.com/hexagon-codes/toolkit/net/ip"
)

var (
	// ErrBlocked 表示目标因 SSRF 策略被拒绝。
	ErrBlocked = errors.New("SSRF blocked")
	// ErrInvalidURL 表示 URL 不满足唯一、可验证的 HTTP(S) 规范形式。
	ErrInvalidURL = errors.New("invalid URL")
	// ErrInvalidContext 表示调用方传入了空上下文。
	ErrInvalidContext = errors.New("context must not be nil")
	// ErrInvalidTransport 表示无法构造可信的连接期防护边界。
	ErrInvalidTransport = errors.New("invalid SSRF transport")
)

// blockedHosts 直接阻止的主机名（云厂商元数据端点等）。
var blockedHosts = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true, // GCP metadata
	"169.254.169.254":          true, // AWS/Azure/GCP metadata endpoint
}

// ValidateLocalURL 只允许 loopback 主机（localhost / 127.0.0.0/8 / ::1）。
//
// 用于"按定义就是本地"的 Provider（如 Ollama）：其 base URL 必须是 loopback，
// 绝不能是任意内网地址（云元数据端点 / 局域网主机）——否则一个"本地"Provider
// 就变成了 SSRF 跳板。
func ValidateLocalURL(rawURL string) error {
	_, host, err := parseHTTPURL(rawURL)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// 非字面量、非 localhost 的主机可能解析到任意地址——拒绝。
		return fmt.Errorf("%w: local provider URL must be loopback, got host %q", ErrBlocked, host)
	}
	if ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: local provider URL must be loopback, got %q", ErrBlocked, host)
}

// ValidateURL 校验 URL 是否安全（非内网/私有 IP）。
//
// 该函数用于在发起连接前尽早返回明确错误；DNS 结果与后续连接之间仍可能变化，
// 所以真正发起不可信请求时还必须使用 NewTransport 或 NewClient。
func ValidateURL(ctx context.Context, rawURL string) error {
	return validateURLWithResolver(ctx, rawURL, net.DefaultResolver.LookupNetIP)
}

// lookupNetIPFunc 是确定性测试与连接期校验共用的最窄 DNS 注入边界。
type lookupNetIPFunc func(ctx context.Context, network, host string) ([]netip.Addr, error)

func validateURLWithResolver(ctx context.Context, rawURL string, lookupNetIP lookupNetIPFunc) error {
	if isNilContext(ctx) {
		return ErrInvalidContext
	}
	_, host, err := parseHTTPURL(rawURL)
	if err != nil {
		return err
	}
	if literal, _, policyErr := validateHostWithoutDNS(host); literal || policyErr != nil {
		return policyErr
	}
	if lookupNetIP == nil {
		return fmt.Errorf("%w: DNS resolver must not be nil", ErrInvalidTransport)
	}

	addresses, err := lookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("%w: DNS lookup failed for %s: %w", ErrBlocked, host, err)
	}
	_, err = validateResolvedAddresses(host, addresses)
	return err
}

func parseHTTPURL(rawURL string) (*url.URL, string, error) {
	if rawURL == "" || strings.TrimSpace(rawURL) != rawURL {
		return nil, "", fmt.Errorf("%w: surrounding whitespace is not allowed", ErrInvalidURL)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if parsed.Opaque != "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return nil, "", fmt.Errorf("%w: only HTTP and HTTPS schemes are allowed", ErrInvalidURL)
	}
	if parsed.User != nil {
		return nil, "", fmt.Errorf("%w: user information is not allowed", ErrInvalidURL)
	}
	if parsed.Fragment != "" {
		return nil, "", fmt.Errorf("%w: fragments are not allowed", ErrInvalidURL)
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, "", fmt.Errorf("%w: URL missing host", ErrInvalidURL)
	}
	if strings.Contains(host, "%") {
		return nil, "", fmt.Errorf("%w: IPv6 zone identifiers are not allowed", ErrInvalidURL)
	}
	if strings.HasSuffix(host, ".") {
		return nil, "", fmt.Errorf("%w: host must not use a trailing dot", ErrInvalidURL)
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return nil, "", fmt.Errorf("%w: port must not be empty", ErrInvalidURL)
	}
	host = strings.ToLower(host)
	if err := validateASCIIHost(host); err != nil {
		return nil, "", err
	}
	if port := parsed.Port(); port != "" {
		if len(port) > 1 && port[0] == '0' {
			return nil, "", fmt.Errorf("%w: port must not contain leading zeros", ErrInvalidURL)
		}
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return nil, "", fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidURL)
		}
	}
	return parsed, host, nil
}

func validateASCIIHost(host string) error {
	if host == "" || len(host) > 253 {
		return fmt.Errorf("%w: host length is invalid", ErrInvalidURL)
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	for _, character := range host {
		if character > 0x7f {
			return fmt.Errorf("%w: Unicode hostnames must use an ASCII IDNA form", ErrInvalidURL)
		}
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || !isASCIIAlphaNumeric(label[0]) ||
			!isASCIIAlphaNumeric(label[len(label)-1]) {
			return fmt.Errorf("%w: host %q is not canonical", ErrInvalidURL, host)
		}
		for index := range len(label) {
			character := label[index]
			if !isASCIIAlphaNumeric(character) && character != '-' {
				return fmt.Errorf("%w: host %q is not canonical", ErrInvalidURL, host)
			}
		}
	}
	return nil
}

func isASCIIAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func isLegacyIPv4Literal(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	values := make([]uint64, len(parts))
	for index, part := range parts {
		if part == "" {
			return false
		}
		base := 10
		digits := part
		if len(part) > 2 && part[0:2] == "0x" {
			base = 16
			digits = part[2:]
		} else if len(part) > 1 && part[0] == '0' {
			base = 8
			digits = part[1:]
		}
		if digits == "" {
			return false
		}
		value, err := strconv.ParseUint(digits, base, 32)
		if err != nil {
			return false
		}
		values[index] = value
	}

	switch len(values) {
	case 1:
		return values[0] < 1<<32
	case 2:
		return values[0] <= 0xff && values[1] <= 0xffffff
	case 3:
		return values[0] <= 0xff && values[1] <= 0xff && values[2] <= 0xffff
	case 4:
		return values[0] <= 0xff && values[1] <= 0xff && values[2] <= 0xff && values[3] <= 0xff
	default:
		return false
	}
}

func validateHostWithoutDNS(host string) (literal bool, address netip.Addr, err error) {
	if blockedHosts[host] || strings.HasSuffix(host, ".localhost") {
		return false, netip.Addr{}, fmt.Errorf("%w: host %q is not allowed", ErrBlocked, host)
	}
	if parsed, parseErr := netip.ParseAddr(host); parseErr == nil {
		parsed = parsed.Unmap()
		if err := validateResolvedAddress(host, parsed); err != nil {
			return true, netip.Addr{}, err
		}
		return true, parsed, nil
	}
	if isLegacyIPv4Literal(host) {
		return false, netip.Addr{}, fmt.Errorf("%w: non-canonical IP literal %q is not allowed", ErrBlocked, host)
	}
	return false, netip.Addr{}, nil
}

func validateResolvedAddresses(host string, addresses []netip.Addr) ([]netip.Addr, error) {
	if len(addresses) == 0 {
		return nil, fmt.Errorf("%w: DNS resolver returned no addresses for %q", ErrBlocked, host)
	}
	validated := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		if !address.IsValid() || address.Zone() != "" {
			return nil, fmt.Errorf("%w: DNS resolver returned an invalid address for %q", ErrBlocked, host)
		}
		address = address.Unmap()
		if err := validateResolvedAddress(host, address); err != nil {
			return nil, err
		}
		validated = append(validated, address)
	}
	return validated, nil
}

func validateResolvedAddress(host string, address netip.Addr) error {
	if !address.IsValid() || iputil.IsPrivateOrReservedIP(net.IP(address.AsSlice())) {
		return fmt.Errorf("%w: %q resolves to private or reserved IP %s", ErrBlocked, host, address)
	}
	return nil
}

func isNilContext(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	value := reflect.ValueOf(ctx)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
