package ip

import (
	"net"
	"net/http"
	"strings"
)

// IsValid 验证 IP 地址是否有效
func IsValid(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsIPv4 判断是否为 IPv4 地址
func IsIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.To4() != nil
}

// IsIPv6 判断是否为 IPv6 地址
func IsIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.To4() == nil && parsed.To16() != nil
}

// IsPrivate 判断是否为私有 IP
func IsPrivate(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsPrivate()
}

// IsLoopback 判断是否为回环地址
func IsLoopback(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

// IsPublic 判断是否为公网 IP
func IsPublic(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return !parsed.IsPrivate() && !parsed.IsLoopback() && !parsed.IsUnspecified()
}

// IsInCIDR 判断 IP 是否在 CIDR 范围内
func IsInCIDR(ip, cidr string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	return network.Contains(parsed)
}

// IsInRange 判断 IP 是否在指定范围内
func IsInRange(ip, start, end string) bool {
	ipParsed := net.ParseIP(ip)
	startParsed := net.ParseIP(start)
	endParsed := net.ParseIP(end)

	if ipParsed == nil || startParsed == nil || endParsed == nil {
		return false
	}

	// 转换为相同格式进行比较
	ip16 := ipParsed.To16()
	start16 := startParsed.To16()
	end16 := endParsed.To16()

	return bytes16Compare(ip16, start16) >= 0 && bytes16Compare(ip16, end16) <= 0
}

// bytes16Compare 比较两个 16 字节的 IP
func bytes16Compare(a, b net.IP) int {
	for i := 0; i < 16; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// GetLocalIPs 获取本机所有 IP 地址
func GetLocalIPs() ([]string, error) {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}

	return ips, nil
}

// GetLocalIP 获取本机首选 IP 地址
func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// 如果无法连接外网，尝试获取本地接口 IP
		ips, err := GetLocalIPs()
		if err != nil || len(ips) == 0 {
			return "127.0.0.1", err
		}
		return ips[0], nil
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// GetOutboundIP 获取出站 IP（连接外网时使用的 IP）
func GetOutboundIP() (string, error) {
	return GetLocalIP()
}

// FromRequest 从 HTTP 请求中获取客户端 IP
//
// 警告: 此函数信任代理请求头（X-Forwarded-For 等），仅应在可信反向代理后使用
// 直接暴露到公网时，攻击者可以伪造这些头绕过 IP 限制
// 对于安全敏感场景，应使用 FromRequestDirect 或 FromRequestWithTrustedProxies
func FromRequest(r *http.Request) string {
	// 按优先级检查代理头
	headers := []string{
		"X-Real-IP",
		"X-Forwarded-For",
		"CF-Connecting-IP", // Cloudflare
		"True-Client-IP",   // Akamai
	}

	for _, header := range headers {
		ip := r.Header.Get(header)
		if ip != "" {
			// X-Forwarded-For 可能包含多个 IP，取第一个
			if header == "X-Forwarded-For" {
				parts := strings.Split(ip, ",")
				ip = strings.TrimSpace(parts[0])
			}
			if IsValid(ip) {
				return ip
			}
		}
	}

	// 从 RemoteAddr 获取
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// FromRequestDirect 从 HTTP 请求直接获取客户端 IP（忽略代理头）
//
// 安全方法：只从 RemoteAddr 获取 IP，不信任任何代理请求头。
// 适用于直接暴露到公网或安全敏感的场景。
//
// 对于在反向代理后部署的服务，应使用 FromRequestWithTrustedProxies。
func FromRequestDirect(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// FromRequestWithTrustedProxies 从 HTTP 请求中获取客户端 IP（带可信代理校验）
//
// 安全方法：只有当 RemoteAddr 在可信代理列表中时，才信任代理头
// trustedProxies 支持单个 IP（如 "10.0.0.1"）和 CIDR（如 "10.0.0.0/8"）
//
// 示例:
//
//	// 信任来自 10.0.0.0/8 网段的代理
//	ip := FromRequestWithTrustedProxies(r, []string{"10.0.0.0/8", "172.16.0.0/12"})
func FromRequestWithTrustedProxies(r *http.Request, trustedProxies []string) string {
	remoteIP := FromRequestDirect(r)

	// 检查 RemoteAddr 是否在可信代理列表中
	trusted := false
	for _, proxy := range trustedProxies {
		if strings.Contains(proxy, "/") {
			// CIDR 格式
			if IsInCIDR(remoteIP, proxy) {
				trusted = true
				break
			}
		} else {
			// 单个 IP
			if remoteIP == proxy {
				trusted = true
				break
			}
		}
	}

	// 如果不在可信代理列表，直接返回 RemoteAddr
	if !trusted {
		return remoteIP
	}

	// 从可信代理，读取代理头
	return FromRequest(r)
}

// ParseCIDR 解析 CIDR
func ParseCIDR(cidr string) (ip net.IP, network *net.IPNet, err error) {
	return net.ParseCIDR(cidr)
}

// CIDRContains 判断 CIDR 是否包含指定 IP
func CIDRContains(cidr, ip string) bool {
	return IsInCIDR(ip, cidr)
}

// IPv4ToInt 将 IPv4 地址转换为整数
func IPv4ToInt(ip string) uint32 {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0
	}
	ip4 := parsed.To4()
	if ip4 == nil {
		return 0
	}
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

// IntToIPv4 将整数转换为 IPv4 地址
func IntToIPv4(n uint32) string {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)).String()
}

// Mask 对 IP 应用掩码
func Mask(ip string, mask int) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}

	ip4 := parsed.To4()
	if ip4 != nil {
		// IPv4
		masked := ip4.Mask(net.CIDRMask(mask, 32))
		return masked.String()
	}

	// IPv6
	masked := parsed.Mask(net.CIDRMask(mask, 128))
	return masked.String()
}

// GetMACAddress 获取本机 MAC 地址
func GetMACAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		// 跳过回环接口和无 MAC 地址的接口
		if iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}
		// 跳过未启用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		return iface.HardwareAddr.String(), nil
	}

	return "", nil
}

// ResolveHost 解析主机名为 IP 地址
func ResolveHost(host string) ([]string, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	result := make([]string, len(ips))
	for i, ip := range ips {
		result[i] = ip.String()
	}
	return result, nil
}

// ReverseLookup 反向 DNS 查询
func ReverseLookup(ip string) ([]string, error) {
	return net.LookupAddr(ip)
}
