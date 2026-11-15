// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"regexp"
	"strconv"
	"testing"
	"unsafe"

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

// TestPrecompiledPatterns_Anchored pins that the infrastructure patterns
// match only the real endpoints at the root or under the default internal
// prefix, never an application route that merely ends in the same segment.
func TestPrecompiledPatterns_Anchored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern *regexp.Regexp
		match   []string
		noMatch []string
	}{
		{
			name:    "HealthPath",
			pattern: HealthPathPattern,
			match:   []string{"/healthz", "/READYZ", "/internal/livez", "/internal/health"},
			noMatch: []string{"/admin/healthz", "/users/ready", "/internal/x/readyz", "/healthz/extra", "/internalx/healthz"},
		},
		{
			name:    "MetricsPath",
			pattern: MetricsPathPattern,
			match:   []string{"/metrics", "/metrics/", "/internal/metrics/"},
			noMatch: []string{"/users/metrics", "/api/v1/metrics", "/metrics/extra"},
		},
		{
			name:    "PprofPath",
			pattern: PprofPathPattern,
			match:   []string{"/debug/pprof/", "/debug/pprof/heap", "/internal/pprof/heap", "/pprof"},
			noMatch: []string{"/x/pprof/y", "/admin/debug/pprof/heap", "/debug/pprof/heap/../../admin", "/internal/pprof/a/b"},
		},
		{
			name:    "HealthMethod",
			pattern: HealthMethodPattern,
			match:   []string{"/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/Watch"},
			noMatch: []string{"/grpc.health.v1.HealthAdmin/Reset", "/grpc.health.v1.Healthx/Check", "/grpc.health.v1.Health/", "/grpc.health.v1.Health/a/b"},
		},
		{
			name:    "ReflectionMethod",
			pattern: ReflectionMethodPattern,
			match:   []string{"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"},
			noMatch: []string{"/grpc.reflection.v1.ServerReflectionAdmin/X", "/grpc.reflection.v1.ServerReflection/a/b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, p := range tt.match {
				require.True(t, tt.pattern.MatchString(p), "pattern should match %q", p)
			}
			for _, p := range tt.noMatch {
				require.False(t, tt.pattern.MatchString(p), "pattern should not match %q", p)
			}
		})
	}
}

func TestInfrastructurePathPatterns_Prefix(t *testing.T) {
	t.Parallel()

	matchAny := func(patterns []*regexp.Regexp, path string) bool {
		for _, p := range patterns {
			if p.MatchString(path) {
				return true
			}
		}
		return false
	}

	custom := InfrastructurePathPatterns("/ops.v1/")
	for _, p := range []string{"/ops.v1/healthz", "/ops.v1/metrics/", "/ops.v1/pprof/heap", "/healthz", "/debug/pprof/"} {
		require.True(t, matchAny(custom, p), "custom-prefix patterns should match %q", p)
	}
	for _, p := range []string{"/internal/healthz", "/opsxv1/healthz", "/users/ops.v1/healthz"} {
		require.False(t, matchAny(custom, p), "custom-prefix patterns should not match %q", p)
	}

	root := InfrastructurePathPatterns("")
	require.True(t, matchAny(root, "/metrics"))
	require.False(t, matchAny(root, "/internal/metrics"))
}

func TestChecker_FoldCase(t *testing.T) {
	t.Parallel()

	pattern := regexp.MustCompile(`^/api/`)

	raw, err := New(nil, WithIgnorePatterns(pattern))
	require.NoError(t, err)
	require.False(t, raw.ShouldFilter("/API/x"), "patterns see the raw path by default")

	folded, err := New(nil, WithIgnorePatterns(pattern), WithFoldCase())
	require.NoError(t, err)
	require.True(t, folded.ShouldFilter("/API/x"), "patterns see the lowercased path with WithFoldCase")
	require.True(t, folded.ShouldFilter("/api/x"))
}

// TestChecker_CacheIsBounded pins that request-derived paths cannot grow the
// verdict cache past its capacity, and that verdicts stay correct once it is
// full.
func TestChecker_CacheIsBounded(t *testing.T) {
	t.Parallel()

	const capacity = 8
	c, err := New([]string{"/healthz"}, WithIgnorePatterns(regexp.MustCompile(`^/internal/`)), WithCacheSize(capacity))
	require.NoError(t, err)

	for i := range 10 * capacity {
		require.False(t, c.ShouldFilter("/api/users/"+strconv.Itoa(i)))
	}
	require.LessOrEqual(t, c.verdicts.Load().size.Load(), int64(capacity))

	require.True(t, c.ShouldFilter("/internal/x"), "uncached match after the cache filled")
	require.True(t, c.ShouldFilter("/HEALTHZ"), "exact match after the cache filled")
	require.False(t, c.ShouldFilter("/api/users/1"), "cached miss stays a miss")
}

func TestCheckerWithCacheDisabled(t *testing.T) {
	c, err := New([]string{"/health"}, WithCacheSize(0))
	require.NoError(t, err)
	require.True(t, c.ShouldFilter("/health"), "ShouldFilter should still work without cache")
}

// cachedKey returns the key the verdict cache holds for path, if any.
func cachedKey(c *Checker, path string) (string, bool) {
	var key string
	var found bool
	c.verdicts.Load().m.Range(func(k, _ any) bool {
		if k.(string) == path {
			key, found = k.(string), true
			return false
		}
		return true
	})
	return key, found
}

// TestChecker_CacheRecoversFromBurst pins that an early burst of distinct
// paths (a scanner right after startup) cannot occupy the verdict cache for
// the life of the process: routes seen after the burst still get memoized.
func TestChecker_CacheRecoversFromBurst(t *testing.T) {
	t.Parallel()

	const capacity = 8
	c, err := New(nil, WithIgnorePatterns(regexp.MustCompile(`^/internal/`)), WithCacheSize(capacity))
	require.NoError(t, err)

	for i := range 10 * capacity {
		require.False(t, c.ShouldFilter("/scan/"+strconv.Itoa(i)))
	}
	require.False(t, c.ShouldFilter("/api/orders"))
	_, ok := cachedKey(c, "/api/orders")
	require.False(t, ok, "a full epoch younger than minEpoch refuses new entries")

	c.minEpoch = 0 // the epoch has aged out
	for range 2 * capacity {
		require.False(t, c.ShouldFilter("/api/orders"))
	}
	_, ok = cachedKey(c, "/api/orders")
	require.True(t, ok, "a route seen after the burst must be cached")
}

// TestChecker_CacheCopiesKeys pins that a cached verdict does not retain the
// caller's string, which is usually a substring of the request URL.
func TestChecker_CacheCopiesKeys(t *testing.T) {
	t.Parallel()

	c, err := New(nil, WithIgnorePatterns(regexp.MustCompile(`^/internal/`)))
	require.NoError(t, err)

	url := "/api/orders?page=2"
	path := url[:len("/api/orders")]
	require.False(t, c.ShouldFilter(path))

	key, ok := cachedKey(c, path)
	require.True(t, ok)
	require.NotSame(t, unsafe.StringData(path), unsafe.StringData(key), "cache key must be a copy")
}
