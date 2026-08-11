package redisconn

import (
	"context"
	"errors"
	"fmt"

	redis "github.com/redis/go-redis/v9"
)

// Factory owns a normalized connection configuration snapshot. Dynamic
// providers and opaque handles referenced by TLSConfig remain caller-owned.
// Construct factories with NewFactory; the zero value is not usable.
type Factory struct {
	config Config
}

// NewFactory validates config and stores an independent normalized snapshot.
func NewFactory(config Config) (*Factory, error) {
	normalized := config.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return &Factory{config: normalized}, nil
}

// NewClient constructs a lazy go-redis client for the configured topology. It
// does not perform network I/O; use Open when startup connectivity is required.
func (f *Factory) NewClient() redis.UniversalClient {
	switch f.config.Mode {
	case ModeSingle:
		return redis.NewClient(f.singleOptions())
	case ModeCluster:
		return redis.NewClusterClient(f.clusterOptions())
	case ModeSentinel:
		return redis.NewFailoverClient(f.failoverOptions())
	default:
		panic("redisconn: factory contains unsupported mode")
	}
}

// Open constructs a client and verifies connectivity and authentication with
// PING. A client that fails verification is closed and never returned.
func (f *Factory) Open(ctx context.Context) (redis.UniversalClient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: Open requires a non-nil context", ErrInvalidContext)
	}
	client := f.NewClient()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, openFailure(err, client.Close())
	}
	return client, nil
}

func openFailure(pingErr, closeErr error) error {
	if closeErr == nil {
		return fmt.Errorf("redisconn: ping failed: %w", pingErr)
	}
	return fmt.Errorf(
		"redisconn: ping failed and client close failed: %w",
		errors.Join(pingErr, closeErr),
	)
}

func (f *Factory) singleOptions() *redis.Options {
	config := f.config
	return &redis.Options{
		Addr:                       config.Addrs[0],
		Username:                   config.DataCredentials.Username,
		Password:                   config.DataCredentials.Password,
		CredentialsProviderContext: adaptCredentialsProvider(config.CredentialsProvider),
		DB:                         config.DB,
		MaxRetries:                 config.MaxRetries,
		MinRetryBackoff:            config.MinRetryBackoff,
		MaxRetryBackoff:            config.MaxRetryBackoff,
		DialTimeout:                config.DialTimeout,
		ReadTimeout:                config.ReadTimeout,
		WriteTimeout:               config.WriteTimeout,
		PoolSize:                   config.PoolSize,
		MinIdleConns:               config.MinIdleConns,
		MaxIdleConns:               config.MaxIdleConns,
		MaxActiveConns:             config.MaxActiveConns,
		PoolTimeout:                config.PoolTimeout,
		ConnMaxIdleTime:            config.ConnMaxIdleTime,
		ConnMaxLifetime:            config.ConnMaxLifetime,
		TLSConfig:                  cloneTLSConfig(config.TLSConfig),
	}
}

func (f *Factory) clusterOptions() *redis.ClusterOptions {
	config := f.config
	return &redis.ClusterOptions{
		Addrs:                      append([]string(nil), config.Addrs...),
		Username:                   config.DataCredentials.Username,
		Password:                   config.DataCredentials.Password,
		CredentialsProviderContext: adaptCredentialsProvider(config.CredentialsProvider),
		MaxRedirects:               config.MaxRedirects,
		MaxRetries:                 config.MaxRetries,
		MinRetryBackoff:            config.MinRetryBackoff,
		MaxRetryBackoff:            config.MaxRetryBackoff,
		DialTimeout:                config.DialTimeout,
		ReadTimeout:                config.ReadTimeout,
		WriteTimeout:               config.WriteTimeout,
		PoolSize:                   config.PoolSize,
		MinIdleConns:               config.MinIdleConns,
		MaxIdleConns:               config.MaxIdleConns,
		MaxActiveConns:             config.MaxActiveConns,
		PoolTimeout:                config.PoolTimeout,
		ConnMaxIdleTime:            config.ConnMaxIdleTime,
		ConnMaxLifetime:            config.ConnMaxLifetime,
		TLSConfig:                  cloneTLSConfig(config.TLSConfig),
	}
}

func (f *Factory) failoverOptions() *redis.FailoverOptions {
	config := f.config
	return &redis.FailoverOptions{
		MasterName:                 config.MasterName,
		SentinelAddrs:              append([]string(nil), config.Addrs...),
		SentinelUsername:           config.SentinelCredentials.Username,
		SentinelPassword:           config.SentinelCredentials.Password,
		Username:                   config.DataCredentials.Username,
		Password:                   config.DataCredentials.Password,
		CredentialsProviderContext: adaptCredentialsProvider(config.CredentialsProvider),
		DB:                         config.DB,
		MaxRetries:                 config.MaxRetries,
		MinRetryBackoff:            config.MinRetryBackoff,
		MaxRetryBackoff:            config.MaxRetryBackoff,
		DialTimeout:                config.DialTimeout,
		ReadTimeout:                config.ReadTimeout,
		WriteTimeout:               config.WriteTimeout,
		PoolSize:                   config.PoolSize,
		MinIdleConns:               config.MinIdleConns,
		MaxIdleConns:               config.MaxIdleConns,
		MaxActiveConns:             config.MaxActiveConns,
		PoolTimeout:                config.PoolTimeout,
		ConnMaxIdleTime:            config.ConnMaxIdleTime,
		ConnMaxLifetime:            config.ConnMaxLifetime,
		TLSConfig:                  cloneTLSConfig(config.TLSConfig),
	}
}

func adaptCredentialsProvider(provider CredentialsProvider) func(context.Context) (string, string, error) {
	if provider == nil {
		return nil
	}
	return func(ctx context.Context) (string, string, error) {
		credentials, err := provider(ctx)
		if err != nil {
			return "", "", &credentialsProviderFailure{cause: err}
		}
		if !credentials.validPair() {
			return "", "", fmt.Errorf(
				"%w: provider username and password must be returned together",
				ErrInvalidCredentials,
			)
		}
		return credentials.Username, credentials.Password, nil
	}
}

// credentialsProviderFailure 保留底层错误身份，同时避免把提供器错误文本中的凭据写入日志。
type credentialsProviderFailure struct {
	cause error
}

func (*credentialsProviderFailure) Error() string {
	return ErrCredentialsProvider.Error()
}

func (e *credentialsProviderFailure) Unwrap() []error {
	return []error{ErrCredentialsProvider, e.cause}
}
