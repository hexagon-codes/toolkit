// Package tokenizer 提供 AI 模型的 Token 计数功能
//
// Token 计数对于以下场景非常重要：
//   - 预估 API 调用成本
//   - 确保请求不超过模型上下文限制
//   - 计算限流配额
//
// 本包提供多种估算方法：
//   - 基于字符的快速估算
//   - 基于规则的中等精度估算
//   - 支持中文、英文等多语言
//
// 基本用法：
//
//	counter := tokenizer.New(tokenizer.GPT4)
//	count := counter.Count("Hello, 世界!")
//
// 估算消息列表：
//
//	messages := []tokenizer.Message{
//	    {Role: "system", Content: "You are a helpful assistant."},
//	    {Role: "user", Content: "Hello!"},
//	}
//	count := counter.CountMessages(messages)
//
// 注意：本包提供的是估算值，实际 Token 数可能因模型而异。
// 如需精确计数，请使用官方 tiktoken 库。
package tokenizer
