// Package circuit 提供熔断器实现
//
// 熔断器用于防止级联故障，当下游服务不可用时自动断开，
// 避免持续的失败请求消耗系统资源。
//
// 熔断器有三种状态：
//   - Closed（关闭）：正常状态，所有请求通过
//   - Open（打开）：熔断状态，拒绝所有请求
//   - HalfOpen（半开）：探测状态，允许部分请求通过以检测恢复
//
// 基本用法：
//
//	breaker := circuit.New(
//	    circuit.WithThreshold(5),        // 5 次失败后熔断
//	    circuit.WithTimeout(30*time.Second), // 熔断 30 秒后进入半开
//	)
//
//	result, err := breaker.Execute(func() (any, error) {
//	    return callExternalAPI()
//	})
//
// AI API 专用：
//
//	breaker := circuit.NewAIBreaker(circuit.OpenAIConfig)
//
// 支持观察者模式：
//
//	breaker.OnStateChange(func(from, to circuit.State) {
//	    log.Printf("breaker state changed: %s -> %s", from, to)
//	})
package circuit
