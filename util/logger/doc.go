// Package logger 提供结构化日志工具
//
// 支持分级日志、结构化字段和多种输出格式。
//
// 基本用法:
//
//	log := logger.New()
//	log.Info("message", "key", "value")
//	log.Error("error occurred", "error", err)
//
// 带配置:
//
//	log := logger.New(
//	    logger.WithLevel(logger.InfoLevel),
//	    logger.WithFormat(logger.JSONFormat),
//	)
//
// --- English ---
//
// Package logger provides structured logging utilities.
//
// Features leveled logging, structured fields, and multiple output formats.
//
// Basic usage:
//
//	log := logger.New()
//	log.Info("message", "key", "value")
//	log.Error("error occurred", "error", err)
//
// With configuration:
//
//	log := logger.New(
//	    logger.WithLevel(logger.InfoLevel),
//	    logger.WithFormat(logger.JSONFormat),
//	)
package logger
