// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import (
	"iter"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// DefaultInternalPrefix is the path prefix under which the HTTP server
// factory mounts its built-in endpoints (ping, healthz, readyz, metrics,
// pprof). The default HTTP path patterns accept it as the only nested form.
const DefaultInternalPrefix = "/internal"

// Pre-compiled patterns for infrastructure endpoints that are typically
// excluded from logging, tracing, and metrics middleware. Every pattern is
// anchored at both ends, so an application route that merely ends in the
// same segment (/users/metrics, /files/x/pprof/heap) never matches.
// These are safe to share across goroutines.
var (
	// ReflectionMethodPattern matches the methods of the gRPC server
	// reflection services (grpc.reflection.v1.ServerReflection and
	// grpc.reflection.v1alpha.ServerReflection) and nothing else.
	ReflectionMethodPattern = regexp.MustCompile(`^(?i)/grpc\.reflection\.v1(alpha)?\.serverReflection/[^/]+$`)

	// HealthMethodPattern matches the methods of the gRPC health service
	// (grpc.health.v1.Health/Check, /Watch, /List) and nothing else.
	HealthMethodPattern = regexp.MustCompile(`^(?i)/grpc\.health\.v1\.health/[^/]+$`)

	// HealthPathPattern matches the health, readiness, and liveness probe
	// paths at the root or directly under [DefaultInternalPrefix]
	// (/healthz, /internal/readyz).
	HealthPathPattern = healthPathPattern(DefaultInternalPrefix)

	// MetricsPathPattern matches the Prometheus /metrics endpoint, with or
	// without a trailing slash, at the root or directly under
	// [DefaultInternalPrefix] (/metrics, /internal/metrics/).
	MetricsPathPattern = metricsPathPattern(DefaultInternalPrefix)

	// PprofPathPattern matches the Go pprof index and one profile segment
	// below it, mounted at /pprof or /debug/pprof, at the root or directly
	// under [DefaultInternalPrefix] (/debug/pprof/heap, /internal/pprof/heap).
	PprofPathPattern = pprofPathPattern(DefaultInternalPrefix)
)

// InfrastructurePathPatterns returns the anchored health, metrics, and pprof
// path patterns for endpoints served at the root or directly under prefix.
// An empty prefix matches root-level endpoints only. Use it when the
// built-in endpoints are mounted under a prefix other than
// [DefaultInternalPrefix].
func InfrastructurePathPatterns(prefix string) []*regexp.Regexp {
	return []*regexp.Regexp{
		healthPathPattern(prefix),
		metricsPathPattern(prefix),
		pprofPathPattern(prefix),
	}
}

func healthPathPattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile(`^(?i)` + optionalPrefix(prefix) + `/(health|healthz|ready|readyz|live|livez)$`)
}

func metricsPathPattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile(`^(?i)` + optionalPrefix(prefix) + `/metrics/?$`)
}

func pprofPathPattern(prefix string) *regexp.Regexp {
	return regexp.MustCompile(`^(?i)` + optionalPrefix(prefix) + `(/debug)?/pprof(/[^/]*)?$`)
}

// optionalPrefix renders prefix as an optional, literal regex group.
func optionalPrefix(prefix string) string {
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return `(?:` + regexp.QuoteMeta(prefix) + `)?`
}

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

// maxCachedPathLen bounds the length of a path whose verdict is cached, so
// a full cache holds at most cacheSize short strings.
const maxCachedPathLen = 256

// minCacheEpoch is the minimum age of a full verdict cache before it is
// replaced, so a sustained stream of distinct paths refills it at most once
// per epoch instead of allocating on every request.
const minCacheEpoch = 10 * time.Second

// Checker implements [Filter] by matching paths against an exact set
// (case-insensitive) and a list of compiled regular expressions.
//
// The exact set is immutable after construction. Pattern verdicts, positive
// and negative, are memoized in a lock-free map of at most cacheSize entries.
// A full map refuses new entries, so high-cardinality request paths fall
// through to the patterns instead of churning it, until it is at least
// minCacheEpoch old; the next miss then starts a fresh epoch. A burst of
// distinct paths (a scanner right after startup) therefore cannot keep the
// bounded endpoint set (gRPC methods, templated routes) out of the cache for
// the life of the process. Keys are copied on store so a cached verdict never
// pins the request URL it came from. A cache hit takes no lock and writes no
// shared state.
//
// Safe for concurrent use.
type Checker struct {
	ignorePaths    *coremaps.ImmutableMap[string, struct{}]
	ignorePatterns []*regexp.Regexp
	foldCase       bool
	cacheSize      int64
	minEpoch       time.Duration
	verdicts       atomic.Pointer[verdictCache]
}

// verdictCache is one epoch of memoized verdicts.
type verdictCache struct {
	m    sync.Map // path -> bool
	size atomic.Int64
	born time.Time
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
	opts := newOptions(opt...)
	if opts.cacheSize < 0 {
		return nil, coreerrs.Wrapf(coreerrs.ErrInvalidArgument, "cache size must not be negative, got %d", opts.cacheSize)
	}

	c := &Checker{
		ignorePaths:    precomputeIgnorePaths(paths),
		ignorePatterns: opts.ignorePatterns,
		foldCase:       opts.foldCase,
		cacheSize:      int64(opts.cacheSize),
		minEpoch:       minCacheEpoch,
	}
	c.verdicts.Store(&verdictCache{born: time.Now()})
	return c, nil
}

// ShouldFilter reports whether path matches any exact path or compiled
// pattern. The exact set is matched case-insensitively; the patterns see the
// raw path, or the lowercased path when the Checker was built with
// [WithFoldCase].
func (ic *Checker) ShouldFilter(path string) bool {
	if len(ic.ignorePatterns) == 0 && ic.ignorePaths.Len() == 0 {
		return false
	}

	cache := ic.verdicts.Load()
	if v, ok := cache.m.Load(path); ok {
		return v.(bool) //nolint:errcheck,forcetypeassert // only bool values are stored
	}

	matched := ic.matches(path)
	ic.remember(cache, path, matched)
	return matched
}

// remember stores a verdict in the current epoch. A full epoch refuses the
// store until it is minEpoch old, then is replaced by a fresh one; losing
// that race only drops this one store.
func (ic *Checker) remember(cache *verdictCache, path string, matched bool) {
	if ic.cacheSize == 0 || len(path) > maxCachedPathLen {
		return
	}
	if cache.size.Load() >= ic.cacheSize {
		if time.Since(cache.born) < ic.minEpoch {
			return
		}
		fresh := &verdictCache{born: time.Now()}
		if !ic.verdicts.CompareAndSwap(cache, fresh) {
			return
		}
		cache = fresh
	}
	if _, loaded := cache.m.LoadOrStore(strings.Clone(path), matched); !loaded {
		cache.size.Add(1)
	}
}

// matches runs the full check: exact path set (case-insensitive), then the
// compiled patterns.
func (ic *Checker) matches(path string) bool {
	lower := strings.ToLower(path)
	if ic.ignorePaths.Contains(lower) {
		return true
	}

	subject := path
	if ic.foldCase {
		subject = lower
	}
	for _, rx := range ic.ignorePatterns {
		if rx.MatchString(subject) {
			return true
		}
	}

	return false
}

// Paths returns an iterator over the ignored paths.
func (ic *Checker) Paths() iter.Seq[string] {
	return ic.ignorePaths.Keys()
}

// Methods returns an iterator over the ignored methods.
// This is a semantic alias for Paths() for gRPC use cases.
func (ic *Checker) Methods() iter.Seq[string] {
	return ic.Paths()
}

// precomputeIgnorePaths builds the immutable set of lowercased path names.
func precomputeIgnorePaths(paths []string) *coremaps.ImmutableMap[string, struct{}] {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		set[strings.ToLower(path)] = struct{}{}
	}
	return coremaps.NewImmutableMap(set)
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
