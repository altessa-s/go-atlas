// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"log/slog"
	"testing"
)

func TestWithDomain(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{"string", WithDomain[string]("example.com"), "example.com"},
		{"string_trimmed", WithDomain[string]("  example.com  "), "example.com"},
		{"empty_string_ignored", WithDomain[string](""), ""},
		{"whitespace_only_ignored", WithDomain[string]("   "), ""},
		{"pointer", WithDomain[*string](strPtr("ptr.example.com")), "ptr.example.com"},
		{"nil_pointer_ignored", WithDomain[*string](nil), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := newOptions(tt.opt)
			if opts.domain != tt.want {
				t.Fatalf("domain = %q, want %q", opts.domain, tt.want)
			}
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	if opts.cacheSize != DefaultCacheSize {
		t.Fatalf("cacheSize = %d, want %d", opts.cacheSize, DefaultCacheSize)
	}
	if opts.logger == nil {
		t.Fatal("logger should not be nil")
	}
	if opts.cacheDisabled {
		t.Fatal("cacheDisabled should be false by default")
	}
	if opts.cacheOnlySentinel {
		t.Fatal("cacheOnlySentinel should be false by default")
	}
}

func TestWithCacheDisabled(t *testing.T) {
	opts := newOptions(WithCacheDisabled())
	if !opts.cacheDisabled {
		t.Fatal("cacheDisabled should be true")
	}
}

func TestWithCacheOnlySentinel(t *testing.T) {
	opts := newOptions(WithCacheOnlySentinel())
	if !opts.cacheOnlySentinel {
		t.Fatal("cacheOnlySentinel should be true")
	}
}

func TestWithCacheSize(t *testing.T) {
	opts := newOptions(WithCacheSize(500))
	if opts.cacheSize != 500 {
		t.Fatalf("cacheSize = %d, want 500", opts.cacheSize)
	}
}

func TestWithLogger(t *testing.T) {
	t.Run("nil_ignored", func(t *testing.T) {
		opts := newOptions(WithLogger(nil))
		if opts.logger == nil {
			t.Fatal("nil logger should fallback to default (non-nil)")
		}
	})

	t.Run("set", func(t *testing.T) {
		l := slog.Default()
		opts := newOptions(WithLogger(l))
		if opts.logger != l {
			t.Fatal("logger should be set")
		}
	})
}

func TestWithSentinelErrors_Empty_Ignored(t *testing.T) {
	opts := newOptions(WithSentinelErrors())
	if len(opts.sentinelErrors) != 0 {
		t.Fatalf("sentinelErrors len = %d, want 0", len(opts.sentinelErrors))
	}
}

func TestWithFinalizer(t *testing.T) {
	opts := newOptions(WithFinalizer(DefaultFinalizer))
	if opts.finalizer == nil {
		t.Fatal("finalizer should be set")
	}
}

func strPtr(s string) *string { return &s }
