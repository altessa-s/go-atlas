// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.want, opts.domain)
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultCacheSize, opts.cacheSize)
	require.NotNil(t, opts.logger, "logger should not be nil")
	require.False(t, opts.cacheDisabled, "cacheDisabled should be false by default")
	require.False(t, opts.cacheOnlySentinel, "cacheOnlySentinel should be false by default")
}

func TestWithCacheDisabled(t *testing.T) {
	opts := newOptions(WithCacheDisabled())
	require.True(t, opts.cacheDisabled, "cacheDisabled should be true")
}

func TestWithCacheOnlySentinel(t *testing.T) {
	opts := newOptions(WithCacheOnlySentinel())
	require.True(t, opts.cacheOnlySentinel, "cacheOnlySentinel should be true")
}

func TestWithCacheSize(t *testing.T) {
	opts := newOptions(WithCacheSize(500))
	require.Equal(t, 500, opts.cacheSize)
}

func TestWithLogger(t *testing.T) {
	t.Run("nil_ignored", func(t *testing.T) {
		opts := newOptions(WithLogger(nil))
		require.NotNil(t, opts.logger, "nil logger should fallback to default (non-nil)")
	})

	t.Run("set", func(t *testing.T) {
		l := slog.Default()
		opts := newOptions(WithLogger(l))
		require.Equal(t, l, opts.logger)
	})
}

func TestWithSentinelErrors_Empty_Ignored(t *testing.T) {
	opts := newOptions(WithSentinelErrors())
	require.Len(t, opts.sentinelErrors, 0)
}

func TestWithFinalizer(t *testing.T) {
	opts := newOptions(WithFinalizer(DefaultFinalizer))
	require.NotNil(t, opts.finalizer, "finalizer should be set")
}

func strPtr(s string) *string { return &s }
