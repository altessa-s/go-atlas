// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"iter"
	"regexp"

	"github.com/altessa-s/go-atlas/data/cache/lru"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Pre-compiled patterns for infrastructure endpoints that are typically
// excluded from logging, tracing, and authentication middleware.
// These are safe to share across goroutines.
var (
	// ReflectionMethodPattern matches gRPC server reflection RPCs
	// (grpc.reflection.v1.ServerReflection/* and grpc.reflection.v1alpha.ServerReflection/*).
	ReflectionMethodPattern = regexp.MustCompile(`^(?i)/grpc\.reflection\.v1(alpha)?\.serverReflection/`)

	// HealthMethodPattern matches gRPC health check RPCs
	// (grpc.health.v1.Health/*).
	HealthMethodPattern = regexp.MustCompile(`^(?i)/grpc\.health\.v1\.health/*`)

	// HealthPathPattern matches common HTTP health/readiness/liveness
	// probe paths, including those nested under a prefix
	// (e.g. /internal/readyz, /health).
	HealthPathPattern = regexp.MustCompile(`(?i)/(health|healthz|ready|readyz|live|livez)$`)

	// MetricsPathPattern matches the Prometheus-style /metrics endpoint.
	MetricsPathPattern = regexp.MustCompile(`^(?i)/metrics$`)

	// PprofPathPattern matches Go pprof debug paths (/pprof, /pprof/heap, etc.).
	PprofPathPattern = regexp.MustCompile(`(?i)/pprof(/.*)?$`)
)

// Filter determines whether a given HTTP path or gRPC full-method name
// should be excluded from middleware processing.
type Filter interface {
	// ShouldFilter reports whether path should be skipped.
	ShouldFilter(path string) bool
	// Paths returns an iterator over the exact paths registered for filtering.
	Paths() iter.Seq[string]
	// Methods is a semantic alias for [Filter.Paths] in gRPC contexts.
	Methods() iter.Seq[string]
}

// emptyStruct is a reusable empty struct to avoid allocations.
var emptyStruct = struct{}{}

// Checker implements [Filter] by matching paths against an exact set
// (case-insensitive) and a list of compiled regular expressions.
// Positive results are cached in an LRU to amortize repeated lookups.
//
// Safe for concurrent use when the underlying LRU implementation is
// thread-safe.
type Checker struct {
	ignorePaths    map[string]struct{}
	ignorePatterns []*regexp.Regexp
	cache          lru.Cacher[string, struct{}]
	options        *options
}

// New creates a new Checker with the specified paths and options.
// Paths are normalized to lowercase for case-insensitive matching.
//
// Example:
//
//	checker, err := endpointfilter.New(
//		[]string{"/health", "/metrics"},
//		endpointfilter.WithCacheSize(1024),
//	)
func New(paths []string, opt ...Option) (*Checker, error) {
	ic := &Checker{
		ignorePaths: precomputeIgnorePaths(paths),
		options:     newOptions(opt...),
	}

	// Initialize cache if size > 0
	if ic.options.cacheSize > 0 {
		var err error
		if ic.cache, err = lru.NewCache[string, struct{}](ic.options.cacheSize); err != nil {
			return nil, err
		}
	}

	ic.ignorePatterns = ic.options.ignorePatterns

	return ic, nil
}

// ShouldFilter reports whether path matches any exact path or compiled
// pattern. The check order is: LRU cache, exact match, regex patterns.
// Matched paths are added to the cache for subsequent calls.
func (ic *Checker) ShouldFilter(path string) bool {
	// Fast path: Check LRU cache if available
	if ic.cache != nil && ic.cache.Has(path) {
		return true
	}

	// Convert to lowercase once for all comparisons (intern the result)
	pathLower := corestrings.InternLowerString(path)

	// Check exact path match
	if ic.ignorePaths != nil {
		if _, ok := ic.ignorePaths[pathLower]; ok {
			if ic.cache != nil {
				ic.cache.Put(path, emptyStruct)
			}
			return true
		}
	}

	// Check pattern matches
	for _, rx := range ic.ignorePatterns {
		if rx.MatchString(path) {
			if ic.cache != nil {
				ic.cache.Put(path, emptyStruct)
			}
			return true
		}
	}

	return false
}

// Paths returns an iterator over the ignored paths.
func (ic *Checker) Paths() iter.Seq[string] {
	return coremaps.Keys(ic.ignorePaths)
}

// Methods returns an iterator over the ignored methods.
// This is a semantic alias for Paths() for gRPC use cases.
func (ic *Checker) Methods() iter.Seq[string] {
	return ic.Paths()
}

// precomputeIgnorePaths creates a map of lowercased and interned path names.
func precomputeIgnorePaths(paths []string) map[string]struct{} {
	if len(paths) == 0 {
		return nil
	}

	result := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		// Intern the lowercased path name to reduce memory usage and ensure consistency
		internedPath := corestrings.InternLowerString(path)
		result[internedPath] = emptyStruct
	}
	return result
}

// CompilePatterns compiles a list of pattern strings into regex patterns.
//
// Example:
//
//	patterns, err := endpointfilter.CompilePatterns([]string{`^/api/public/.*`, `^/static/.*`})
func CompilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		rx, err := regexp.Compile(pattern)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "invalid pattern %q", pattern)
		}
		compiled = append(compiled, rx)
	}
	return compiled, nil
}

// NewOrNoop creates a [Checker] with the given paths and options.
// If construction fails (e.g. invalid cache size), it returns a [Noop]
// filter that never matches, ensuring callers always receive a usable
// [Filter].
func NewOrNoop(paths []string, opt ...Option) Filter {
	checker, err := New(paths, opt...)
	if err != nil {
		return NewNoop()
	}
	return checker
}
