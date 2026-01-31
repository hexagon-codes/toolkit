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
