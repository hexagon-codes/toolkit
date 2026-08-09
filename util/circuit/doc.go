// Package circuit 提供并发安全的熔断器。
//
// 熔断器在关闭、打开和半开三种状态间切换。Acquire 返回的 Permit 将每次
// 放行与完成结果绑定，调用方必须且只能调用一次 Complete。直接执行函数时
// 优先使用 Execute 或 ExecuteContext，由熔断器自动管理许可。
//
// 基本用法：
//
//	breaker, err := circuit.New(
//		circuit.WithThreshold(5),
//		circuit.WithTimeout(30*time.Second),
//	)
//	if err != nil {
//		return err
//	}
//	defer breaker.Close()
//	result, err := breaker.Execute(func() (any, error) {
//		return callExternalAPI()
//	})
//
// 手动管理异步请求：
//
//	permit, err := breaker.Acquire()
//	if err != nil {
//		return err
//	}
//	resultErr := callExternalAPIAsync()
//	if err := permit.Complete(resultErr); err != nil {
//		return err
//	}
package circuit
