// Package ip 提供 IP 地址工具函数
//
// 包括 IP 解析、验证和网络计算。
//
// 基本用法:
//
//	ip.IsValid("192.168.1.1")   // true
//	ip.IsIPv4("192.168.1.1")    // true
//	ip.IsIPv6("::1")            // true
//	ip.IsPrivate("192.168.1.1") // true
//
// 获取本地 IP:
//
//	localIP, err := ip.LocalIP()
//
// 解析 CIDR:
//
//	network, err := ip.ParseCIDR("192.168.1.0/24")
//	network.Contains("192.168.1.100")  // true
//
// --- English ---
//
// Package ip provides IP address utilities.
//
// Includes IP parsing, validation, and network calculations.
//
// Basic usage:
//
//	ip.IsValid("192.168.1.1")   // true
//	ip.IsIPv4("192.168.1.1")    // true
//	ip.IsIPv6("::1")            // true
//	ip.IsPrivate("192.168.1.1") // true
//
// Get local IP:
//
//	localIP, err := ip.LocalIP()
//
// Parse CIDR:
//
//	network, err := ip.ParseCIDR("192.168.1.0/24")
//	network.Contains("192.168.1.100")  // true
package ip
