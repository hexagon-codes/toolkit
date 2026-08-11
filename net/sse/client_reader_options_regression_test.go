package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientAppliesReaderResourceLimits(t *testing.T) {
	t.Parallel()

	client := mustClient(NewClient(
		"https://example.com/events",
		WithReaderOptions(WithMaxTotalBytes(8)),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: oversized\n\n")),
			}, nil
		})}),
	))
	stream, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for event := range stream.Events() {
		_ = event
	}
	var terminalErr error
	for err := range stream.Errors() {
		terminalErr = errors.Join(terminalErr, err)
	}
	if !errors.Is(terminalErr, ErrMaxBytesExceeded) {
		t.Fatalf("stream error = %v, want ErrMaxBytesExceeded", terminalErr)
	}
}

func TestClientRejectsInvalidReaderOptionsAtConstruction(t *testing.T) {
	t.Parallel()

	client, err := NewClient(
		"https://example.com/events",
		WithReaderOptions(WithMaxLineBytes(-1)),
	)
	if client != nil || !errors.Is(err, ErrInvalidClientConfig) {
		t.Fatalf("NewClient() = (%v, %v), want (nil, ErrInvalidClientConfig)", client, err)
	}
}
