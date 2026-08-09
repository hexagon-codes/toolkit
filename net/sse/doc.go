// Package sse 提供 Server-Sent Events（SSE）的解析、客户端连接和服务端写入能力。
//
// Reader 遵循 WHATWG 事件流格式，支持 CRLF、LF、CR、多行 data、控制字段及安全字节上限。
// Client 负责校验 HTTP 响应并管理响应体生命周期；Writer 负责生成合法的事件流帧。
//
// 基本用法：
//
//	// 解析 SSE 事件
//	reader, err := sse.NewReader(resp.Body)
//	if err != nil {
//		return err
//	}
//	defer reader.Close()
//	for {
//		event, err := reader.Read()
//		if errors.Is(err, io.EOF) {
//			break
//		}
//		if err != nil {
//			return err
//		}
//		fmt.Println(event.Data)
//	}
//
//	// 连接 SSE 端点
//	client, err := sse.NewClient("https://api.example.com/stream")
//	if err != nil {
//		return err
//	}
//	stream, err := client.Connect(ctx)
//	if err != nil {
//		return err
//	}
//	defer stream.Close()
//	for event := range stream.Events() {
//		fmt.Println(event.Data)
//	}
package sse
