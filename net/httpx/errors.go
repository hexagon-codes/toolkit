package httpx

import "errors"

var (
	// ErrInvalidClientConfig 表示封装客户端配置无效。
	ErrInvalidClientConfig = errors.New("httpx: invalid client configuration")
	// ErrInvalidRawClientConfig 表示原生客户端配置无效。
	ErrInvalidRawClientConfig = errors.New("httpx: invalid raw client configuration")
	// ErrInvalidPoolConfig 表示连接池配置无效。
	ErrInvalidPoolConfig = errors.New("httpx: invalid pool configuration")
	// ErrHostPoolCapacity 表示主机连接池已达到配置容量。
	ErrHostPoolCapacity = errors.New("httpx: host pool capacity reached")
	// ErrInvalidContext 表示调用方传入了空上下文。
	ErrInvalidContext = errors.New("httpx: context must not be nil")
	// ErrInvalidRequest 表示调用方传入了空请求或缺少请求目标。
	ErrInvalidRequest = errors.New("httpx: invalid request")
	// ErrPoolClosed 表示连接池已经关闭。
	ErrPoolClosed = errors.New("httpx: pool is closed")
)
