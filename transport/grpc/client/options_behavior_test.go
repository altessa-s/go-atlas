// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"
	"time"
)

func TestWithRetryEmptyPreservesDefaultsAndEnablesRetry(t *testing.T) {
	opts := defaultOptions()
	WithRetry()(opts)

	if !opts.enableRetry {
		t.Fatal("WithRetry() did not enable retries")
	}
	if opts.retryOptions == nil {
		t.Fatal("WithRetry() did not construct retry defaults")
	}
	if opts.retryOptions.maxAttempts != DefaultMaxRetryAttempts {
		t.Fatalf("maxAttempts = %d, want %d", opts.retryOptions.maxAttempts, DefaultMaxRetryAttempts)
	}
	if opts.retryOptions.initialBackoff != DefaultInitialBackoff {
		t.Fatalf("initialBackoff = %s, want %s", opts.retryOptions.initialBackoff, DefaultInitialBackoff)
	}
	if opts.retryOptions.maxBackoff != DefaultMaxBackoff {
		t.Fatalf("maxBackoff = %s, want %s", opts.retryOptions.maxBackoff, DefaultMaxBackoff)
	}
}

func TestWithRetryStatusCodesEmptyPreservesDefaults(t *testing.T) {
	opts := newRetryOptions(WithRetryStatusCodes())
	if len(opts.statusCodes) != len(DefaultRetryableStatusCodes) {
		t.Fatalf("statusCodes = %v, want %v", opts.statusCodes, DefaultRetryableStatusCodes)
	}
	for i := range DefaultRetryableStatusCodes {
		if opts.statusCodes[i] != DefaultRetryableStatusCodes[i] {
			t.Fatalf("statusCodes = %v, want %v", opts.statusCodes, DefaultRetryableStatusCodes)
		}
	}
	if opts.initialBackoff != time.Second {
		t.Fatalf("initialBackoff = %s, want 1s", opts.initialBackoff)
	}
}
