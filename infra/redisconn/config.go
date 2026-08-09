// Package redisconn provides validated Redis connection configuration and
// factories for standalone, cluster, and Sentinel deployments.
package redisconn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"
)

// Mode identifies the Redis deployment topology.
type Mode string

const (
	// ModeSingle connects to exactly one standalone Redis endpoint.
	ModeSingle Mode = "single"
	// ModeCluster connects directly to one or more Redis Cluster nodes.
	ModeCluster Mode = "cluster"
	// ModeSentinel discovers a Redis primary through one or more Sentinels.
	ModeSentinel Mode = "sentinel"
)

var (
	// ErrInvalidConfig reports an invalid topology or connection setting.
	ErrInvalidConfig = errors.New("redisconn: invalid config")
	// ErrInvalidCredentials reports incomplete or conflicting credentials.
	ErrInvalidCredentials = errors.New("redisconn: invalid credentials")
	// ErrCredentialsProvider reports that a dynamic credentials provider failed.
	ErrCredentialsProvider = errors.New("redisconn: credentials provider failed")
	// ErrInvalidContext reports a nil context passed to an operation that can block.
	ErrInvalidContext = errors.New("redisconn: invalid context")
)

// Credentials contains a Redis ACL username and password. Static credentials
// must either both be empty or both be non-empty.
type Credentials struct {
	Username string
	Password string
}

// CredentialsProvider returns data-node credentials when a new Redis
// connection is initialized. It is mutually exclusive with DataCredentials.
type CredentialsProvider func(context.Context) (Credentials, error)

// Config describes a Redis deployment and its connection behavior.
type Config struct {
	Mode       Mode
	Addrs      []string
	MasterName string

	DataCredentials     Credentials
	SentinelCredentials Credentials
	CredentialsProvider CredentialsProvider

	DB        int
	TLSConfig *tls.Config

	MaxRetries      int
	MaxRedirects    int
	MinRetryBackoff time.Duration
	MaxRetryBackoff time.Duration

	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	PoolSize       int
	MinIdleConns   int
	MaxIdleConns   int
	MaxActiveConns int
	PoolTimeout    time.Duration

	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

// DefaultConfig returns a normalized Config for mode and addrs. Callers must
// still provide mode-specific required fields such as MasterName for Sentinel.
func DefaultConfig(mode Mode, addrs ...string) Config {
	return Config{Mode: mode, Addrs: addrs}.Normalize()
}

// Normalize applies production defaults and copies mutable configuration
// storage owned by this package. It does not validate topology or credentials.
func (c Config) Normalize() Config {
	c.Addrs = append([]string(nil), c.Addrs...)
	c.TLSConfig = cloneTLSConfig(c.TLSConfig)

	if c.MaxRetries == 0 {
		// Cluster commands already have a MaxRedirects retry loop. Preserve
		// go-redis's disabled per-node retry default to avoid nested retries.
		if c.Mode == ModeCluster {
			c.MaxRetries = -1
		} else {
			c.MaxRetries = 3
		}
	}
	if c.MaxRedirects == 0 {
		c.MaxRedirects = 3
	}
	if c.MinRetryBackoff == 0 {
		c.MinRetryBackoff = 8 * time.Millisecond
	}
	if c.MaxRetryBackoff == 0 {
		c.MaxRetryBackoff = 512 * time.Millisecond
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = 5 * time.Second
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 3 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = c.ReadTimeout
	}
	if c.PoolTimeout == 0 {
		if c.ReadTimeout > 0 {
			c.PoolTimeout = c.ReadTimeout + time.Second
		} else {
			c.PoolTimeout = 30 * time.Second
		}
	}
	if c.PoolSize == 0 {
		if c.Mode == ModeCluster {
			// Cluster PoolSize applies per node. Preserve go-redis's smaller
			// per-node default so discovered nodes do not multiply the pool size.
			c.PoolSize = 5 * runtime.GOMAXPROCS(0)
		} else {
			c.PoolSize = 10 * runtime.GOMAXPROCS(0)
		}
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 30 * time.Minute
	}

	return c
}

// Validate checks topology and authentication invariants without mutating c.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeSingle:
		if len(c.Addrs) != 1 {
			return fmt.Errorf("%w: single mode requires exactly one address", ErrInvalidConfig)
		}
	case ModeCluster:
		if len(c.Addrs) == 0 {
			return fmt.Errorf("%w: cluster mode requires at least one address", ErrInvalidConfig)
		}
		if c.DB != 0 {
			return fmt.Errorf("%w: cluster mode requires database zero", ErrInvalidConfig)
		}
	case ModeSentinel:
		if len(c.Addrs) == 0 {
			return fmt.Errorf("%w: sentinel mode requires at least one address", ErrInvalidConfig)
		}
		if strings.TrimSpace(c.MasterName) == "" {
			return fmt.Errorf("%w: sentinel mode requires a master name", ErrInvalidConfig)
		}
	default:
		return fmt.Errorf("%w: unsupported mode", ErrInvalidConfig)
	}

	for _, addr := range c.Addrs {
		if strings.TrimSpace(addr) == "" {
			return fmt.Errorf("%w: addresses must not be blank", ErrInvalidConfig)
		}
	}
	if c.DB < 0 {
		return fmt.Errorf("%w: database must not be negative", ErrInvalidConfig)
	}
	if err := c.validateNumericSettings(); err != nil {
		return err
	}

	if !c.DataCredentials.validPair() {
		return fmt.Errorf("%w: data username and password must be provided together", ErrInvalidCredentials)
	}
	if c.CredentialsProvider != nil && !c.DataCredentials.empty() {
		return fmt.Errorf("%w: data credentials and credentials provider are mutually exclusive", ErrInvalidCredentials)
	}

	if c.Mode != ModeSentinel && !c.SentinelCredentials.empty() {
		return fmt.Errorf("%w: sentinel credentials require sentinel mode", ErrInvalidConfig)
	}
	if !c.SentinelCredentials.validPair() {
		return fmt.Errorf("%w: sentinel username and password must be provided together", ErrInvalidCredentials)
	}

	return nil
}

func (c Config) validateNumericSettings() error {
	if c.MaxRetries < -1 {
		return fmt.Errorf("%w: maximum retries must be non-negative or -1", ErrInvalidConfig)
	}
	if c.MaxRedirects < -1 {
		return fmt.Errorf("%w: maximum redirects must be non-negative or -1", ErrInvalidConfig)
	}
	if c.MinRetryBackoff < -1 {
		return fmt.Errorf("%w: minimum retry backoff must be non-negative or -1", ErrInvalidConfig)
	}
	if c.MaxRetryBackoff < -1 {
		return fmt.Errorf("%w: maximum retry backoff must be non-negative or -1", ErrInvalidConfig)
	}
	if c.DialTimeout < 0 {
		return fmt.Errorf("%w: dial timeout must not be negative", ErrInvalidConfig)
	}
	if c.ReadTimeout < -2 {
		return fmt.Errorf("%w: read timeout must be non-negative, -1, or -2", ErrInvalidConfig)
	}
	if c.WriteTimeout < -2 {
		return fmt.Errorf("%w: write timeout must be non-negative, -1, or -2", ErrInvalidConfig)
	}
	if c.PoolTimeout < 0 {
		return fmt.Errorf("%w: pool timeout must not be negative", ErrInvalidConfig)
	}

	poolCounts := []struct {
		name  string
		value int
	}{
		{name: "pool size", value: c.PoolSize},
		{name: "minimum idle connections", value: c.MinIdleConns},
		{name: "maximum idle connections", value: c.MaxIdleConns},
		{name: "maximum active connections", value: c.MaxActiveConns},
	}
	for _, count := range poolCounts {
		if count.value < 0 || count.value > math.MaxInt32 {
			return fmt.Errorf("%w: %s must fit a non-negative int32", ErrInvalidConfig, count.name)
		}
	}

	return nil
}

func (c Credentials) empty() bool {
	return c.Username == "" && c.Password == ""
}

func (c Credentials) validPair() bool {
	return c.empty() || (c.Username != "" && c.Password != "")
}

//nolint:staticcheck // Deep-copy deprecated NameToCertificate when callers still supply it.
func cloneTLSConfig(source *tls.Config) *tls.Config {
	if source == nil {
		return nil
	}

	cloned := source.Clone()
	cloned.NextProtos = append([]string(nil), source.NextProtos...)
	cloned.CipherSuites = append([]uint16(nil), source.CipherSuites...)
	cloned.CurvePreferences = append([]tls.CurveID(nil), source.CurvePreferences...)
	cloned.EncryptedClientHelloConfigList = append([]byte(nil), source.EncryptedClientHelloConfigList...)

	if source.RootCAs != nil {
		cloned.RootCAs = source.RootCAs.Clone()
	}
	if source.ClientCAs != nil {
		cloned.ClientCAs = source.ClientCAs.Clone()
	}

	cloned.Certificates = make([]tls.Certificate, len(source.Certificates))
	for index := range source.Certificates {
		cloned.Certificates[index] = cloneTLSCertificate(source.Certificates[index])
	}
	if source.NameToCertificate != nil {
		cloned.NameToCertificate = make(map[string]*tls.Certificate, len(source.NameToCertificate))
		for name, certificate := range source.NameToCertificate {
			if certificate == nil {
				cloned.NameToCertificate[name] = nil
				continue
			}
			certificateCopy := cloneTLSCertificate(*certificate)
			cloned.NameToCertificate[name] = &certificateCopy
		}
	}

	cloned.EncryptedClientHelloKeys = append([]tls.EncryptedClientHelloKey(nil), source.EncryptedClientHelloKeys...)
	for index := range cloned.EncryptedClientHelloKeys {
		cloned.EncryptedClientHelloKeys[index].Config = append([]byte(nil), source.EncryptedClientHelloKeys[index].Config...)
		cloned.EncryptedClientHelloKeys[index].PrivateKey = append([]byte(nil), source.EncryptedClientHelloKeys[index].PrivateKey...)
	}

	return cloned
}

func cloneTLSCertificate(source tls.Certificate) tls.Certificate {
	cloned := source
	cloned.Certificate = cloneByteSlices(source.Certificate)
	cloned.SupportedSignatureAlgorithms = append([]tls.SignatureScheme(nil), source.SupportedSignatureAlgorithms...)
	cloned.OCSPStaple = append([]byte(nil), source.OCSPStaple...)
	cloned.SignedCertificateTimestamps = cloneByteSlices(source.SignedCertificateTimestamps)
	return cloned
}

func cloneByteSlices(source [][]byte) [][]byte {
	if source == nil {
		return nil
	}
	cloned := make([][]byte, len(source))
	for index := range source {
		cloned[index] = append([]byte(nil), source[index]...)
	}
	return cloned
}
