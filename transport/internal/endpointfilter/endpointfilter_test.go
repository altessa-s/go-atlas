// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
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
			require.NoError(t, err)
			got := c.ShouldFilter(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestChecker_WithPatterns(t *testing.T) {
	pattern := regexp.MustCompile(`^/api/public/.*`)
	c, err := New(nil, WithIgnorePatterns(pattern))
	require.NoError(t, err)

	require.True(t, c.ShouldFilter("/api/public/foo"), "ShouldFilter(/api/public/foo) = false, want true")
	require.False(t, c.ShouldFilter("/api/private/foo"), "ShouldFilter(/api/private/foo) = true, want false")
}

func TestChecker_CacheHit(t *testing.T) {
	c, _ := New([]string{"/health"})

	// First call populates cache
	c.ShouldFilter("/health")
	// Second call should hit cache
	require.True(t, c.ShouldFilter("/health"), "second call should still return true")
}

func TestChecker_Paths(t *testing.T) {
	c, _ := New([]string{"/health", "/ready"})

	count := 0
	for range c.Paths() {
		count++
	}
	require.Equal(t, 2, count)
}

func TestChecker_Methods(t *testing.T) {
	c, _ := New([]string{"/health"})
	count := 0
	for range c.Methods() {
		count++
	}
	require.Equal(t, 1, count)
}

func TestNoop(t *testing.T) {
	n := NewNoop()
	require.False(t, n.ShouldFilter("/anything"), "Noop.ShouldFilter() = true")
	count := 0
	for range n.Paths() {
		count++
	}
	require.Equal(t, 0, count)
	count = 0
	for range n.Methods() {
		count++
	}
	require.Equal(t, 0, count)
}

func TestNewOrNoop_Success(t *testing.T) {
	f := NewOrNoop([]string{"/health"})
	require.True(t, f.ShouldFilter("/health"), "expected filter to work")
}

func TestNewOrNoop_AlwaysReturnsFilter(t *testing.T) {
	f := NewOrNoop([]string{"/health"})
	require.NotNil(t, f)
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
			require.Equal(t, tt.wantErr, (err != nil))
			if !tt.wantErr {
				require.Len(t, result, tt.wantLen)
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
		{"MetricsPath", MetricsPathPattern, "/metrics", "/metrics/extra"},
		{"MetricsPathTrailingSlash", MetricsPathPattern, "/metrics/", "/metricsx"},
		{"MetricsPathPrefixed", MetricsPathPattern, "/internal/metrics/", "/api/metricsx"},
		{"PprofPath", PprofPathPattern, "/pprof", "/api/data"},
		{"PprofSubpath", PprofPathPattern, "/pprof/heap", "/pprofx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, tt.pattern.MatchString(tt.match))
			require.False(t, tt.pattern.MatchString(tt.noMatch), "pattern should not match %q", tt.noMatch)
		})
	}
}

func TestCheckerWithCacheDisabled(t *testing.T) {
	c, err := New([]string{"/health"}, WithCacheSize(0))
	require.NoError(t, err)
	require.True(t, c.ShouldFilter("/health"), "ShouldFilter should still work without cache")
}
