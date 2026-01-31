// Package errorx provides error handling utilities.
//
// It includes error wrapping, error chains, and common error types
// for building robust error handling in Go applications.
//
// Basic usage:
//
//	err := errorx.Wrap(originalErr, "context message")
//	if errorx.Is(err, targetErr) {
//	    // handle specific error
//	}
package errorx
