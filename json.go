// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	rpc "github.com/gorilla/rpc/v2/json2"
)

const (
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond // Increased from 100ms
)

// newHTTPClient creates a fresh HTTP client with disabled connection reuse.
// This avoids EOF errors that can occur with connection pooling in complex
// process hierarchies (e.g., netrunner spawning luxd processes).
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true, // Disable connection reuse to avoid EOF issues
		},
	}
}

// CleanlyCloseBody drains and closes an HTTP response body to prevent
// HTTP/2 GOAWAY errors caused by closing bodies with unread data.
// See: https://github.com/golang/go/issues/46071
func CleanlyCloseBody(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	// Drain any remaining data to allow connection reuse
	_, _ = io.Copy(io.Discard, body)
	return body.Close()
}

// isRetryableError checks if an error is transient and worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// EOF errors are often transient connection issues
	if errors.Is(err, io.EOF) || strings.Contains(errStr, "EOF") {
		return true
	}
	// Connection reset/refused are also transient
	if strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "broken pipe") {
		return true
	}
	return false
}

func SendJSONRequest(
	ctx context.Context,
	uri *url.URL,
	method string,
	params interface{},
	reply interface{},
	options ...Option,
) error {
	log.Printf("[RPC-DEBUG] SendJSONRequest called: method=%s uri=%s", method, uri.String())
	requestBodyBytes, err := rpc.EncodeClientRequest(method, params)
	if err != nil {
		return fmt.Errorf("failed to encode client params: %w", err)
	}

	ops := NewOptions(options)
	uri.RawQuery = ops.queryParams.Encode()

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			waitTime := retryBaseWait * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}

		// Create fresh request for each attempt (body buffer is consumed)
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			uri.String(),
			bytes.NewBuffer(requestBodyBytes),
		)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		request.Header = ops.headers
		request.Header.Set("Content-Type", "application/json")

		// Use a fresh HTTP client to avoid connection pooling issues
		client := newHTTPClient()
		resp, err := client.Do(request)
		if err != nil {
			lastErr = err
			log.Printf("[RPC] Request attempt %d failed: %v (retryable=%v)", attempt+1, err, isRetryableError(err))
			if isRetryableError(err) {
				continue // Retry on transient errors
			}
			return fmt.Errorf("failed to issue request: %w", err)
		}
		if attempt > 0 {
			log.Printf("[RPC] Request succeeded on attempt %d", attempt+1)
		}

		// Return an error for any non successful status code
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			CleanlyCloseBody(resp.Body)
			return fmt.Errorf("received status code: %d", resp.StatusCode)
		}

		if err := rpc.DecodeClientResponse(resp.Body, reply); err != nil {
			CleanlyCloseBody(resp.Body)
			return fmt.Errorf("failed to decode client response: %w", err)
		}
		CleanlyCloseBody(resp.Body)
		return nil
	}

	return fmt.Errorf("failed to issue request after %d retries: %w", maxRetries, lastErr)
}
