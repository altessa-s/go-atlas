// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"regexp"
	"testing"
)

func TestNewBaseMiddleware(t *testing.T) {
	b := NewBaseMiddleware("test", nil)
	if b.Name() != "test" {
		t.Fatalf("Name() = %q", b.Name())
	}
	if b.Logger() == nil {
		t.Fatal("Logger() should not be nil")
	}
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
			if got := b.ShouldIgnore(tt.check); got != tt.want {
				t.Fatalf("ShouldIgnore(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}

func TestBaseMiddleware_InternPath(t *testing.T) {
	b := NewBaseMiddleware("test", nil)
	if b.InternPath("/test") != "/test" {
		t.Fatal("InternPath failed")
	}
}

func TestBaseMiddleware_LogMethods(t *testing.T) {
	b := NewBaseMiddleware("test", nil)
	ctx := t.Context()
	b.LogIgnored(ctx, "/test")
	b.LogDebug(ctx, "msg", "/test")
	b.LogWarn(ctx, "msg", "/test", nil)
	b.LogError(ctx, "msg", "/test", nil)
}
