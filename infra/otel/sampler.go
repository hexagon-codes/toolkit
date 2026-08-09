package otel

import (
	"sync"
	"time"
)

// Sampler 采样器接口
type Sampler interface {
	// ShouldSample 决定是否采样
	ShouldSample(traceID, spanName string) bool
}

// AlwaysSampler 总是采样
type AlwaysSampler struct{}

// ShouldSample 总是返回 true
func (s *AlwaysSampler) ShouldSample(traceID, spanName string) bool {
	return true
}

// NeverSampler 从不采样
type NeverSampler struct{}

// ShouldSample 总是返回 false
func (s *NeverSampler) ShouldSample(traceID, spanName string) bool {
	return false
}

// ProbabilitySampler 概率采样器
type ProbabilitySampler struct {
	rate float64
}

// NewProbabilitySampler 创建概率采样器
func NewProbabilitySampler(rate float64) *ProbabilitySampler {
	if rate < 0 {
		rate = 0
	}
	if rate > 1 {
		rate = 1
	}
	return &ProbabilitySampler{rate: rate}
}

// ShouldSample 基于概率决定是否采样
func (s *ProbabilitySampler) ShouldSample(traceID, spanName string) bool {
	if s.rate >= 1 {
		return true
	}
	if s.rate <= 0 {
		return false
	}

	// 基于 traceID 的确定性采样
	hash := 0
	for _, c := range traceID {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}

	return float64(hash%10000)/10000 < s.rate
}

// RateLimitingSampler 限流采样器
type RateLimitingSampler struct {
	rate     float64
	budget   float64
	lastTick time.Time
	mu       sync.Mutex
}

// NewRateLimitingSampler 创建限流采样器
func NewRateLimitingSampler(rate float64) *RateLimitingSampler {
	return &RateLimitingSampler{
		rate:     rate,
		budget:   rate,
		lastTick: time.Now(),
	}
}

// ShouldSample 基于限流决定是否采样
func (s *RateLimitingSampler) ShouldSample(traceID, spanName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastTick).Seconds()
	s.lastTick = now

	// 补充预算
	s.budget += elapsed * s.rate
	if s.budget > s.rate {
		s.budget = s.rate
	}

	// 检查预算
	if s.budget >= 1 {
		s.budget--
		return true
	}

	return false
}
