// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"log/slog"
	"regexp"
	"testing"
)

func TestNewBaseInterceptor(t *testing.T) {
	b := NewBaseInterceptor("test", nil)
	if b.Name() != "test" {
		t.Fatalf("Name() = %q", b.Name())
	}
	if b.Logger() == nil {
		t.Fatal("Logger() should not be nil even with nil input")
	}
}

func TestNewBaseInterceptorWithFilter(t *testing.T) {
	b := NewBaseInterceptorWithFilter("filtered", []string{"/grpc.health.v1.Health/Check"}, nil, nil)
	if b.Name() != "filtered" {
		t.Fatalf("Name() = %q", b.Name())
	}
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
			if got := b.ShouldIgnore(tt.check); got != tt.want {
				t.Fatalf("ShouldIgnore(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}

func TestBaseInterceptor_ShouldIgnoreFromContext(t *testing.T) {
	b := NewBaseInterceptor("test", nil)
	// Without metadata in context, should return nil, false
	meta, ignore := b.ShouldIgnoreFromContext(t.Context())
	if meta != nil || ignore {
		t.Fatalf("expected nil, false; got %v, %v", meta, ignore)
	}
}

func TestBaseInterceptor_InternMethod(t *testing.T) {
	b := NewBaseInterceptor("test", nil)
	m := b.InternMethod("/test/Method")
	if m != "/test/Method" {
		t.Fatalf("InternMethod = %q", m)
	}
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
