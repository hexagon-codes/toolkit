// Package sse 提供 Server-Sent Events (SSE) 处理功能
//
// SSE 是一种服务器推送技术，广泛用于 AI API 的流式响应。
// 本包提供了 SSE 事件解析、客户端连接和服务器端写入等功能。
//
// 基本用法:
//
//	// 解析 SSE 事件
//	reader := sse.NewReader(resp.Body)
//	for {
//	    event, err := reader.Read()
//	    if err == io.EOF {
//	        break
//	    }
//	    fmt.Println(event.Data)
//	}
//
//	// 连接 SSE 端点
//	client := sse.NewClient("https://api.example.com/stream")
//	stream, err := client.Connect(ctx)
//	for event := range stream.Events() {
//	    fmt.Println(event.Data)
//	}
package sse
