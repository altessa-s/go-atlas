// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBaseMiddleware(t *testing.T) {
	b := NewBaseMiddleware("test", nil)
	require.Equal(t, "test", b.Name())
	require.NotNil(t, b.Logger())
}

func TestBaseMiddleware_ShouldIgnore(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		pats  []*regexp.Regexp
		check string
		want  bool
	}{
		{"match", []string{"/health"}, nil, "/health", true},
		{"no_match", []string{"/health"}, nil, "/api", false},
		{"pattern", nil, []*regexp.Regexp{regexp.MustCompile(`^/internal`)}, "/internal/debug", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBaseMiddlewareWithFilter("test", tt.paths, tt.pats, nil)
			got := b.ShouldIgnore(tt.check)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBaseMiddleware_InternPath(t *testing.T) {
	b := NewBaseMiddleware("test", nil)
	require.Equal(t, "/test", b.InternPath("/test"))
}

func TestBaseMiddleware_LogMethods(t *testing.T) {
	b := NewBaseMiddleware("test", nil)
	ctx := t.Context()
	b.LogIgnored(ctx, "/test")
	b.LogDebug(ctx, "msg", "/test")
	b.LogWarn(ctx, "msg", "/test", nil)
	b.LogError(ctx, "msg", "/test", nil)
}
