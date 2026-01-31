// Package streamx 提供 AI API 流式响应的统一抽象层
//
// 本包将不同 AI 厂商（OpenAI, Claude, Gemini 等）的流式响应
// 统一为一致的接口，简化流式处理的复杂度。
//
// 基本用法：
//
//	stream := streamx.NewStream(resp.Body, streamx.OpenAIFormat)
//	for chunk := range stream.Chunks() {
//	    fmt.Print(chunk.Content)
//	}
//
// 收集完整响应：
//
//	result, err := stream.Collect()
//	fmt.Println(result.Content)
//	fmt.Printf("Tokens: %d\n", result.Usage.TotalTokens)
//
// 使用回调处理：
//
//	stream.OnChunk(func(chunk *Chunk) {
//	    fmt.Print(chunk.Content)
//	}).OnDone(func(result *Result) {
//	    fmt.Printf("\nTotal: %d tokens\n", result.Usage.TotalTokens)
//	}).OnError(func(err error) {
//	    log.Printf("Error: %v", err)
//	}).Start()
package streamx
