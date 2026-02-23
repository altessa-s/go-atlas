// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.retryMax != DefaultRetryMax {
		t.Fatalf("retryMax = %d, want %d", opts.retryMax, DefaultRetryMax)
	}
	if opts.retryWaitMin != DefaultRetryWaitMin {
		t.Fatalf("retryWaitMin = %v, want %v", opts.retryWaitMin, DefaultRetryWaitMin)
	}
	if opts.retryWaitMax != DefaultRetryWaitMax {
		t.Fatalf("retryWaitMax = %v, want %v", opts.retryWaitMax, DefaultRetryWaitMax)
	}
	if opts.breakerMaxRequests != DefaultBreakerMaxRequests {
		t.Fatalf("breakerMaxRequests = %d, want %d", opts.breakerMaxRequests, DefaultBreakerMaxRequests)
	}
	if opts.breakerInterval != DefaultBreakerInterval {
		t.Fatalf("breakerInterval = %v", opts.breakerInterval)
	}
	if opts.breakerTimeout != DefaultBreakerTimeout {
		t.Fatalf("breakerTimeout = %v", opts.breakerTimeout)
	}
	if opts.client == nil {
		t.Fatal("client is nil")
	}
}

func TestWithRetryMax(t *testing.T) {
	opts := newOptions(WithRetryMax(10))
	if opts.retryMax != 10 {
		t.Fatalf("retryMax = %d", opts.retryMax)
	}
}

func TestWithRetryWait(t *testing.T) {
	opts := newOptions(WithRetryWait(1*time.Second, 30*time.Second))
	if opts.retryWaitMin != 1*time.Second {
		t.Fatalf("retryWaitMin = %v", opts.retryWaitMin)
	}
	if opts.retryWaitMax != 30*time.Second {
		t.Fatalf("retryWaitMax = %v", opts.retryWaitMax)
	}
}

func TestWithBreakerName(t *testing.T) {
	opts := newOptions(WithBreakerName("my-breaker"))
	if opts.breakerName != "my-breaker" {
		t.Fatalf("breakerName = %q", opts.breakerName)
	}
}

func TestWithBreakerName_StringPtr(t *testing.T) {
	name := "ptr-breaker"
	opts := newOptions(WithBreakerName(&name))
	if opts.breakerName != "ptr-breaker" {
		t.Fatalf("breakerName = %q", opts.breakerName)
	}
}

func TestWithBreakerName_NilPtr(t *testing.T) {
	opts := newOptions(WithBreakerName[*string](nil))
	if opts.breakerName != "" {
		t.Fatalf("breakerName = %q", opts.breakerName)
	}
}

func TestWithClient(t *testing.T) {
	c := &http.Client{}
	opts := newOptions(WithClient(c))
	if opts.client != c {
		t.Fatal("client not set")
	}
}

func TestWithClient_Nil(t *testing.T) {
	opts := newOptions(WithClient(nil))
	if opts.client == nil {
		t.Fatal("nil client should keep default")
	}
}

func TestWithLogger(t *testing.T) {
	l := slog.New(slog.DiscardHandler)
	opts := newOptions(WithLogger(l))
	if opts.logger != l {
		t.Fatal("logger not set")
	}
}

func TestWithLogger_Nil(t *testing.T) {
	opts := newOptions(WithLogger(nil))
	if opts.logger == nil {
		t.Fatal("nil logger should keep default")
	}
}

func TestWithMaxResponseSize(t *testing.T) {
	opts := newOptions(WithMaxResponseSize(1024))
	if opts.maxResponseSize != 1024 {
		t.Fatalf("maxResponseSize = %d", opts.maxResponseSize)
	}
}

func TestWithBreakerInterval_Negative(t *testing.T) {
	opts := newOptions(WithBreakerInterval(-1))
	if opts.breakerInterval != DefaultBreakerInterval {
		t.Fatalf("negative should keep default, got %v", opts.breakerInterval)
	}
}

func TestWithBreakerTimeout_Negative(t *testing.T) {
	opts := newOptions(WithBreakerTimeout(-1))
	if opts.breakerTimeout != DefaultBreakerTimeout {
		t.Fatalf("negative should keep default, got %v", opts.breakerTimeout)
	}
}

func TestWithCircuitBreakerSettings(t *testing.T) {
	settings := &CircuitBreakerSettings{MaxRequests: 5, Timeout: 10 * time.Second}
	opts := newOptions(WithCircuitBreakerSettings("api.test", settings))
	if opts.hostBreakerSettings == nil {
		t.Fatal("hostBreakerSettings is nil")
	}
	if opts.hostBreakerSettings["api.test"] != settings {
		t.Fatal("settings not stored")
	}
}

func TestWithTransport_Nil(t *testing.T) {
	opts := newOptions(WithTransport(nil))
	if opts.transport != nil {
		t.Fatal("nil transport should not be set")
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultRetryWaitMin <= 0 {
		t.Fatalf("DefaultRetryWaitMin = %v", DefaultRetryWaitMin)
	}
	if DefaultRetryWaitMax <= 0 {
		t.Fatalf("DefaultRetryWaitMax = %v", DefaultRetryWaitMax)
	}
	if DefaultRetryMax <= 0 {
		t.Fatalf("DefaultRetryMax = %d", DefaultRetryMax)
	}
	if DefaultBreakerTimeout <= 0 {
		t.Fatalf("DefaultBreakerTimeout = %v", DefaultBreakerTimeout)
	}
	if DefaultBreakerInterval <= 0 {
		t.Fatalf("DefaultBreakerInterval = %v", DefaultBreakerInterval)
	}
	if DefaultBreakerMaxRequests == 0 {
		t.Fatal("DefaultBreakerMaxRequests = 0")
	}
}
