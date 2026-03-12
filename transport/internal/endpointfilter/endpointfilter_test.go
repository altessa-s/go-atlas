// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"regexp"
	"testing"
)

func TestChecker_ShouldFilter(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		input string
		want  bool
	}{
		{"exact_match", []string{"/health"}, "/health", true},
		{"case_insensitive", []string{"/HEALTH"}, "/health", true},
		{"no_match", []string{"/health"}, "/api/data", false},
		{"empty_paths", nil, "/anything", false},
		{"multiple_paths", []string{"/health", "/ready"}, "/ready", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.paths)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if got := c.ShouldFilter(tt.input); got != tt.want {
				t.Fatalf("ShouldFilter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestChecker_WithPatterns(t *testing.T) {
	pattern := regexp.MustCompile(`^/api/public/.*`)
	c, err := New(nil, WithIgnorePatterns(pattern))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !c.ShouldFilter("/api/public/foo") {
		t.Fatal("ShouldFilter(/api/public/foo) = false, want true")
	}
	if c.ShouldFilter("/api/private/foo") {
		t.Fatal("ShouldFilter(/api/private/foo) = true, want false")
	}
}

func TestChecker_CacheHit(t *testing.T) {
	c, _ := New([]string{"/health"})

	// First call populates cache
	c.ShouldFilter("/health")
	// Second call should hit cache
	if !c.ShouldFilter("/health") {
		t.Fatal("second call should still return true")
	}
}

func TestChecker_Paths(t *testing.T) {
	c, _ := New([]string{"/health", "/ready"})

	count := 0
	for range c.Paths() {
		count++
	}
	if count != 2 {
		t.Fatalf("Paths() yielded %d items, want 2", count)
	}
}

func TestChecker_Methods(t *testing.T) {
	c, _ := New([]string{"/health"})
	count := 0
	for range c.Methods() {
		count++
	}
	if count != 1 {
		t.Fatalf("Methods() yielded %d items, want 1", count)
	}
}

func TestNoop(t *testing.T) {
	n := NewNoop()
	if n.ShouldFilter("/anything") {
		t.Fatal("Noop.ShouldFilter() = true")
	}
	count := 0
	for range n.Paths() {
		count++
	}
	if count != 0 {
		t.Fatalf("Noop.Paths() yielded %d items", count)
	}
	count = 0
	for range n.Methods() {
		count++
	}
	if count != 0 {
		t.Fatalf("Noop.Methods() yielded %d items", count)
	}
}

func TestNewOrNoop_Success(t *testing.T) {
	f := NewOrNoop([]string{"/health"})
	if !f.ShouldFilter("/health") {
		t.Fatal("expected filter to work")
	}
}

func TestNewOrNoop_AlwaysReturnsFilter(t *testing.T) {
	f := NewOrNoop([]string{"/health"})
	if f == nil {
		t.Fatal("NewOrNoop returned nil")
	}
	// Verify it implements Filter interface
	var _ Filter = f
}

func TestCompilePatterns(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantErr bool
		wantLen int
	}{
		{"valid", []string{`^/api/.*`, `^/static/.*`}, false, 2},
		{"empty", nil, false, 0},
		{"invalid", []string{`[invalid`}, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompilePatterns(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CompilePatterns() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(result) != tt.wantLen {
				t.Fatalf("len(result) = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestPrecompiledPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern *regexp.Regexp
		match   string
		noMatch string
	}{
		{"ReflectionMethodV1Alpha", ReflectionMethodPattern, "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo", "/api/v1/users"},
		{"ReflectionMethodV1", ReflectionMethodPattern, "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", "/api/v1/users"},
		{"HealthMethod", HealthMethodPattern, "/grpc.health.v1.Health/Check", "/api/v1/data"},
		{"HealthPath", HealthPathPattern, "/health", "/api/data"},
		{"HealthPathPrefixed", HealthPathPattern, "/internal/readyz", "/readyzoo"},
		{"MetricsPath", MetricsPathPattern, "/metrics", "/api/metrics"},
		{"PprofPath", PprofPathPattern, "/pprof", "/api/data"},
		{"PprofSubpath", PprofPathPattern, "/pprof/heap", "/pprofx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.pattern.MatchString(tt.match) {
				t.Fatalf("pattern should match %q", tt.match)
			}
			if tt.pattern.MatchString(tt.noMatch) {
				t.Fatalf("pattern should not match %q", tt.noMatch)
			}
		})
	}
}

func TestCheckerWithCacheDisabled(t *testing.T) {
	c, err := New([]string{"/health"}, WithCacheSize(0))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !c.ShouldFilter("/health") {
		t.Fatal("ShouldFilter should still work without cache")
	}
}
