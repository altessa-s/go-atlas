// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"log/slog"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBaseInterceptor(t *testing.T) {
	b := NewBaseInterceptor("test", nil)
	require.Equal(t, "test", b.Name())
	require.NotNil(t, b.Logger(), "Logger() should not be nil even with nil input")
}

func TestNewBaseInterceptorWithFilter(t *testing.T) {
	b := NewBaseInterceptorWithFilter("filtered", []string{"/grpc.health.v1.Health/Check"}, nil, nil)
	require.Equal(t, "filtered", b.Name())
}

func TestBaseInterceptor_ShouldIgnore(t *testing.T) {
	tests := []struct {
		name     string
		methods  []string
		patterns []*regexp.Regexp
		check    string
		want     bool
	}{
		{"exact_match", []string{"/grpc.health.v1.health/check"}, nil, "/grpc.health.v1.Health/Check", true},
		{"no_match", []string{"/other"}, nil, "/grpc.health.v1.Health/Check", false},
		{"pattern_match", nil, []*regexp.Regexp{regexp.MustCompile(`health`)}, "/grpc.health.v1.Health/Check", true},
		{"no_filter", nil, nil, "/any/method", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBaseInterceptorWithFilter("test", tt.methods, tt.patterns, nil)
			got := b.ShouldIgnore(tt.check)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBaseInterceptor_ShouldIgnoreFromContext(t *testing.T) {
	b := NewBaseInterceptor("test", nil)
	// Without metadata in context, should return nil, false
	meta, ignore := b.ShouldIgnoreFromContext(t.Context())
	require.Nil(t, meta)
	require.False(t, ignore, "expected nil, false; got %v, %v", meta, ignore)
}

func TestBaseInterceptor_InternMethod(t *testing.T) {
	b := NewBaseInterceptor("test", nil)
	m := b.InternMethod("/test/Method")
	require.Equal(t, "/test/Method", m)
}

func TestBaseInterceptor_LogMethods(t *testing.T) {
	b := NewBaseInterceptor("test", slog.New(slog.DiscardHandler))
	ctx := t.Context()
	// Just ensure they don't panic
	b.LogIgnored(ctx, "/test")
	b.LogDebug(ctx, "msg", "/test")
	b.LogWarn(ctx, "msg", "/test", nil)
	b.LogError(ctx, "msg", "/test", nil)
}
