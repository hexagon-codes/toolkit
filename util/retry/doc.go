// Package retry provides retry functionality with exponential backoff.
//
// It supports configurable retry attempts, delays, and custom retry conditions.
//
// Basic usage:
//
//	err := retry.Do(func() error {
//	    return someOperation()
//	}, retry.WithAttempts(3), retry.WithDelay(time.Second))
//
// With exponential backoff:
//
//	err := retry.Do(func() error {
//	    return httpCall()
//	}, retry.WithBackoff(retry.ExponentialBackoff{
//	    InitialInterval: time.Second,
//	    MaxInterval:     time.Minute,
//	    Multiplier:      2,
//	}))
package retry
