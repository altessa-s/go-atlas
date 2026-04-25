// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func BenchmarkUnexpectedStatusError_Error(b *testing.B) {
	err := UnexpectedStatusError{Status: 404, Method: "GET", Host: "example.com", URI: "/users"}
	var s string
	for b.Loop() {
		s = err.Error()
	}
	_ = s
}

func BenchmarkResponseSizeError_Error(b *testing.B) {
	err := &ResponseSizeError{Limit: 1000, Size: 2000}
	var s string
	for b.Loop() {
		s = err.Error()
	}
	_ = s
}

func BenchmarkCircuitBreakerError_Error(b *testing.B) {
	err := &CircuitBreakerError{Name: "test-cb", State: "open"}
	var s string
	for b.Loop() {
		s = err.Error()
	}
	_ = s
}

func BenchmarkRateLimitError_Error(b *testing.B) {
	err := &RateLimitError{Host: "api.test", RetryAfter: 5 * time.Second}
	var s string
	for b.Loop() {
		s = err.Error()
	}
	_ = s
}

func BenchmarkIsTemporaryError(b *testing.B) {
	err := UnexpectedStatusError{Status: http.StatusTooManyRequests}
	for b.Loop() {
		IsTemporaryError(err)
	}
}

func BenchmarkIsResponseSizeError(b *testing.B) {
	err := &ResponseSizeError{Limit: 100, Size: 200}
	for b.Loop() {
		IsResponseSizeError(err)
	}
}

func BenchmarkIsResponseSizeError_Miss(b *testing.B) {
	err := errors.New("other")
	for b.Loop() {
		IsResponseSizeError(err)
	}
}
