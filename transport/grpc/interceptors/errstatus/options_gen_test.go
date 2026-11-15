// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/status"
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
		{"pointer", WithDomain[*string](new("ptr.example.com")), "ptr.example.com"},
		{"nil_pointer_ignored", WithDomain[*string](nil), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := newOptions(tt.opt)
			require.Equal(t, tt.want, opts.domain)
		})
	}
}

func TestMigratedConverterOptionsAppendAtApplicationTime(t *testing.T) {
	first := ErrorConverter{}
	second := ErrorConverter{Matcher: func(context.Context, error) bool { return true }}
	errorInput := []ErrorConverter{first}
	errorOption := WithErrorConverters(errorInput...)
	errorInput[0] = second
	errorOptions := &options{errorConverters: []ErrorConverter{first}}
	errorOption(errorOptions)
	require.Len(t, errorOptions.errorConverters, 2)
	require.NotNil(t, errorOptions.errorConverters[1].Matcher, "WithErrorConverters did not append the value present at apply time")
	errorInput[0] = first
	require.NotNil(t, errorOptions.errorConverters[1].Matcher, "mutating input after applying changed stored converters")
	WithErrorConverters()(errorOptions)
	require.Len(t, errorOptions.errorConverters, 2)

	statusInput := []StatusConverter{{}}
	statusOption := WithStatusConverters(statusInput...)
	statusInput[0] = StatusConverter{Matcher: func(context.Context, *status.Status) bool { return true }}
	statusOptions := &options{statusConverter: []StatusConverter{{}}}
	statusOption(statusOptions)
	require.Len(t, statusOptions.statusConverter, 2)
	require.NotNil(t, statusOptions.statusConverter[1].Matcher)
	statusInput[0] = StatusConverter{}
	require.NotNil(t, statusOptions.statusConverter[1].Matcher)
	WithStatusConverters()(statusOptions)
	require.Len(t, statusOptions.statusConverter, 2)
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultCacheSize, opts.cacheSize)
	require.NotNil(t, opts.logger, "logger should not be nil")
	require.False(t, opts.cacheDisabled, "cacheDisabled should be false by default")
}

func TestWithCacheDisabled(t *testing.T) {
	opts := newOptions(WithCacheDisabled())
	require.True(t, opts.cacheDisabled, "cacheDisabled should be true")
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

func TestWithFinalizer(t *testing.T) {
	opts := newOptions(WithFinalizer(DefaultFinalizer))
	require.NotNil(t, opts.finalizer, "finalizer should be set")
}
