package retry

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestIsRetryableHTTPError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "429 Too Many Requests",
			err:      &HTTPError{StatusCode: 429},
			expected: true,
		},
		{
			name:     "500 Internal Server Error",
			err:      &HTTPError{StatusCode: 500},
			expected: true,
		},
		{
			name:     "502 Bad Gateway",
			err:      &HTTPError{StatusCode: 502},
			expected: true,
		},
		{
			name:     "503 Service Unavailable",
			err:      &HTTPError{StatusCode: 503},
			expected: true,
		},
		{
			name:     "504 Gateway Timeout",
			err:      &HTTPError{StatusCode: 504},
			expected: true,
		},
		{
			name:     "400 Bad Request",
			err:      &HTTPError{StatusCode: 400},
			expected: false,
		},
		{
			name:     "401 Unauthorized",
			err:      &HTTPError{StatusCode: 401},
			expected: false,
		},
		{
			name:     "404 Not Found",
			err:      &HTTPError{StatusCode: 404},
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableHTTPError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryableHTTPError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "429 error",
			err:      &HTTPError{StatusCode: 429},
			expected: true,
		},
		{
			name:     "500 error",
			err:      &HTTPError{StatusCode: 500},
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRateLimitError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRateLimitError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestGetRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected time.Duration
	}{
		{
			name:     "seconds",
			header:   "120",
			expected: 120 * time.Second,
		},
		{
			name:     "empty",
			header:   "",
			expected: 0,
		},
		{
			name:     "invalid",
			header:   "invalid",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{},
			}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}

			result := GetRetryAfter(resp)
			if result != tt.expected {
				t.Errorf("GetRetryAfter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHTTPError(t *testing.T) {
	err := &HTTPError{
		StatusCode: 429,
		Status:     "Too Many Requests",
	}

	expected := "HTTP 429: Too Many Requests"
	if err.Error() != expected {
		t.Errorf("HTTPError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestRetryWithHTTPError(t *testing.T) {
	attempts := 0

	err := Do(func() error {
		attempts++
		if attempts < 3 {
			return &HTTPError{StatusCode: 503}
		}
		return nil
	},
		Attempts(5),
		RetryIf(IsRetryableHTTPError),
		Delay(10*time.Millisecond),
	)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWith429(t *testing.T) {
	attempts := 0

	err := Do(func() error {
		attempts++
		if attempts < 2 {
			return &HTTPError{StatusCode: 429}
		}
		return nil
	},
		Attempts(5),
		RetryIf(IsRetryableHTTPError),
		Delay(10*time.Millisecond),
	)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestNoRetryOn400(t *testing.T) {
	attempts := 0

	err := Do(func() error {
		attempts++
		return &HTTPError{StatusCode: 400}
	},
		Attempts(5),
		RetryIf(IsRetryableHTTPError),
		Delay(10*time.Millisecond),
	)

	if err == nil {
		t.Error("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}
