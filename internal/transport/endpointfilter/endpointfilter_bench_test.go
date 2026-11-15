// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"regexp"
	"strconv"
	"testing"
)

func BenchmarkChecker_ShouldFilter_ExactMatch(b *testing.B) {
	c, _ := New([]string{"/health", "/ready", "/metrics"})
	for b.Loop() {
		c.ShouldFilter("/health")
	}
}

func BenchmarkChecker_ShouldFilter_NoMatch(b *testing.B) {
	c, _ := New([]string{"/health", "/ready", "/metrics"})
	for b.Loop() {
		c.ShouldFilter("/api/v1/users")
	}
}

func BenchmarkChecker_ShouldFilter_Pattern(b *testing.B) {
	pattern := regexp.MustCompile(`^/api/public/.*`)
	c, _ := New(nil, WithIgnorePatterns(pattern))
	for b.Loop() {
		c.ShouldFilter("/api/public/foo")
	}
}

func BenchmarkCompilePatterns(b *testing.B) {
	patterns := []string{`^/api/.*`, `^/static/.*`, `^/health$`}
	for b.Loop() {
		CompilePatterns(patterns)
	}
}

// BenchmarkChecker_ShouldFilter_Parallel measures the check under contention
// with the default infrastructure patterns, as every middleware and
// interceptor runs it once per request.
func BenchmarkChecker_ShouldFilter_Parallel(b *testing.B) {
	c, _ := New([]string{"/grpc.health.v1.Health/Check"},
		WithIgnorePatterns(HealthPathPattern, MetricsPathPattern, PprofPathPattern))

	b.Run("exact", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				c.ShouldFilter("/grpc.health.v1.Health/Check")
			}
		})
	})
	b.Run("no_match", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				c.ShouldFilter("/api/v1/users")
			}
		})
	})
}

// BenchmarkChecker_ShouldFilter_UniquePaths feeds high-cardinality paths
// (REST routes with IDs), which never repeat.
func BenchmarkChecker_ShouldFilter_UniquePaths(b *testing.B) {
	c, _ := New(nil, WithIgnorePatterns(HealthPathPattern, MetricsPathPattern, PprofPathPattern))
	paths := make([]string, 1<<16)
	for i := range paths {
		paths[i] = "/api/v1/users/" + strconv.Itoa(i*7919)
	}

	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c.ShouldFilter(paths[i&(len(paths)-1)])
		i++
	}
}

// BenchmarkChecker_ShouldFilter_AfterBurst measures a bounded route set once
// the epoch filled by a burst of distinct paths (a scanner right after
// startup) has aged out.
func BenchmarkChecker_ShouldFilter_AfterBurst(b *testing.B) {
	c, _ := New(nil, WithIgnorePatterns(HealthPathPattern, MetricsPathPattern, PprofPathPattern))
	for i := range 4 * DefaultIgnoreCacheSize {
		c.ShouldFilter("/scan/" + strconv.Itoa(i))
	}
	c.minEpoch = 0
	routes := []string{"/api/v1/Users", "/api/v1/Orders", "/api/v1/Items", "/api/v1/Carts"}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.ShouldFilter(routes[i&3])
			i++
		}
	})
}
