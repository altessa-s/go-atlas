// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package endpointfilter decides whether a given HTTP path or gRPC method
// should be excluded from middleware/interceptor processing (e.g. health
// checks, metrics endpoints, pprof).
//
// Matching is case-insensitive for exact paths and supports compiled
// [regexp.Regexp] patterns for prefix or wildcard rules. The exact set is
// immutable, and verdicts are memoized in a lock-free map bounded by the
// cache size, so repeated requests to the same path avoid re-evaluation
// without contending on a lock. A full map is replaced by a fresh one once it
// is old enough, so an early burst of distinct paths cannot keep later routes
// out of it. Cached keys are copies; request paths are never interned.
//
// Use [New] for production, [NewNoop] for a no-op implementation, or
// [NewOrNoop] when initialization errors should fall back silently.
//
// Example:
//
//	filter, _ := endpointfilter.New(
//	    []string{"/health", "/metrics"},
//	    endpointfilter.WithCacheSize(1024),
//	)
//	if filter.ShouldFilter(path) {
//	    // skip middleware for this path
//	}
package endpointfilter
