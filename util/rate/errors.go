package rate

import "errors"

var (
	// ErrRateLimitExceeded 表示请求超过速率限制。
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	// ErrInsufficientTokens 表示请求量超过限流器可提供的容量。
	ErrInsufficientTokens = errors.New("insufficient tokens")
	// ErrInvalidCapacity 表示限流器容量无效。
	ErrInvalidCapacity = errors.New("capacity must be greater than zero")
	// ErrInvalidRate 表示令牌生成速率或漏水间隔无效。
	ErrInvalidRate = errors.New("rate must be finite and greater than zero")
	// ErrInvalidWindow 表示滑动窗口时长无效。
	ErrInvalidWindow = errors.New("window must be greater than zero")
	// ErrInvalidTokenCount 表示请求的令牌数无效。
	ErrInvalidTokenCount = errors.New("token count must be greater than zero")
	// ErrNilContext 表示调用方传入了 nil context。
	ErrNilContext = errors.New("context must not be nil")
	// ErrNilLimiter 表示多维限流器包含 nil 实例。
	ErrNilLimiter = errors.New("limiter must not be nil")
	// ErrUnsupportedLimiter 表示限流器不支持原子多维事务。
	ErrUnsupportedLimiter = errors.New("limiter does not support atomic multi-dimensional transactions")
	// ErrDuplicateLimiter 表示同一限流器在一个多维事务中被重复引用。
	ErrDuplicateLimiter = errors.New("limiter must not be duplicated")
	// ErrNoLimiters 表示多维限流器没有配置任何维度。
	ErrNoLimiters = errors.New("at least one limiter is required")
)
