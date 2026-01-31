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
