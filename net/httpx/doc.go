// Package httpx 提供增强型 HTTP 客户端。
//
// 支持重试、超时、熔断器、请求/响应日志记录和有界主机连接池。
//
// 基本用法:
//
//	client, err := httpx.NewClient(
//	    httpx.WithTimeout(10*time.Second),
//	)
//	if err != nil {
//	    return err
//	}
//	resp, err := client.R().SetContext(ctx).Get("https://api.example.com/data")
//
// 有界主机连接池:
//
//	config := httpx.DefaultHostPoolConfig()
//	config.MaxHosts = 128
//	hostPool, err := httpx.NewHostPool(config)
//	if err != nil {
//	    return err
//	}
//	defer hostPool.Close()
//	if err := hostPool.RemoveHost("api.example.com"); err != nil {
//	    return err
//	}
//
// --- English ---
//
// Package httpx provides an enhanced HTTP client.
//
// 支持重试、超时、熔断器、请求/响应日志记录和有界主机连接池。
//
// Basic usage:
//
//	client, err := httpx.NewClient(
//	    httpx.WithTimeout(10*time.Second),
//	)
//	if err != nil {
//	    return err
//	}
//	resp, err := client.R().SetContext(ctx).Get("https://api.example.com/data")
//
// 有界主机连接池:
//
//	config := httpx.DefaultHostPoolConfig()
//	config.MaxHosts = 128
//	hostPool, err := httpx.NewHostPool(config)
//	if err != nil {
//	    return err
//	}
//	defer hostPool.Close()
//	if err := hostPool.RemoveHost("api.example.com"); err != nil {
//	    return err
//	}
package httpx
