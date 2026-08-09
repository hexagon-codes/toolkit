// Package redis provides an explicitly configured go-redis client wrapper and
// ownership-based distributed locking helpers.
package redis

import "github.com/hexagon-codes/toolkit/infra/redisconn"

type (
	// Config is the canonical Redis connection configuration.
	Config = redisconn.Config
	// Mode identifies the Redis deployment topology.
	Mode = redisconn.Mode
	// Credentials contains a Redis ACL username and password.
	Credentials = redisconn.Credentials
	// CredentialsProvider resolves data-node ACL credentials.
	CredentialsProvider = redisconn.CredentialsProvider
)

const (
	// ModeSingle connects to one standalone Redis endpoint.
	ModeSingle = redisconn.ModeSingle
	// ModeCluster connects directly to Redis Cluster seed nodes.
	ModeCluster = redisconn.ModeCluster
	// ModeSentinel discovers a Redis primary through Sentinel endpoints.
	ModeSentinel = redisconn.ModeSentinel
)

var (
	// ErrInvalidContext reports a nil context passed to a blocking operation.
	ErrInvalidContext = redisconn.ErrInvalidContext
	// ErrInvalidConfig reports an invalid topology or connection setting.
	ErrInvalidConfig = redisconn.ErrInvalidConfig
	// ErrInvalidCredentials reports incomplete or conflicting credentials.
	ErrInvalidCredentials = redisconn.ErrInvalidCredentials
	// ErrCredentialsProvider reports that a dynamic credentials provider failed.
	ErrCredentialsProvider = redisconn.ErrCredentialsProvider
)

// DefaultConfig delegates connection defaults to the canonical package.
func DefaultConfig(mode Mode, addrs ...string) Config {
	return redisconn.DefaultConfig(mode, addrs...)
}
