// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"regexp"
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
