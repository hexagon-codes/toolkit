// Package logger 提供并发安全的结构化日志工具。
//
// 包内置 JSON、文本和文件输出，支持动态日志级别、上下文字段与多个输出目标。
// password、secret、token、authorization、cookie 等敏感属性键会在写出前统一脱敏。
//
// 基本用法：
//
//	log, err := logger.New(nil)
//	if err != nil {
//		return err
//	}
//	defer log.Close()
//	log.Info("message", "key", "value")
//
// 使用配置：
//
//	log, err := logger.New(&logger.Config{
//		Level:  "info",
//		Format: "json",
//		Output: "/var/log/app.log",
//	})
package logger
