// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/endpointfilter"
)

// ignored is the exact-path list every target configures.
var ignored = []string{"/healthz", "/metrics", "/debug/pprof"}

// pattern matches the versioned internal namespace.
var pattern = regexp.MustCompile(`^/internal/v[0-9]+/`)

// pathSeeds are request paths worth starting from: the configured ones, and the
// near-misses that decide whether a filter is prefix-based or exact.
var pathSeeds = []string{
	"/healthz",
	"/healthz/",
	"/healthzx",
	"/HEALTHZ",
	"/metrics",
	"/internal/v1/admin",
	"/internal/vX/admin",
	"/api/orders",
	"",
	"/",
	"/debug/pprof/heap",
}

// FuzzShouldFilterMatchesTheConfiguration restates the filter rule
// independently and requires the checker to agree.
//
// The filter decides which requests skip a middleware — audit, metrics, auth in
// some deployments — so a path that filters when it should not is a request
// that silently escapes whatever the middleware enforces. Exact-vs-prefix is
// the distinction that goes wrong: "/healthzx" must not inherit "/healthz"'s
// exemption.
//
// The two mechanisms deliberately differ in case sensitivity — the exact set is
// matched lowercased, the patterns against the raw path — and that asymmetry is
// worth pinning precisely because it is the kind of detail a refactor
// harmonizes without noticing which way.
func FuzzShouldFilterMatchesTheConfiguration(f *testing.F) {
	for _, seed := range pathSeeds {
		f.Add(seed)
	}

	checker, err := endpointfilter.New(ignored, endpointfilter.WithIgnorePatterns(pattern))
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, path string) {
		want := slices.Contains(ignored, strings.ToLower(path)) || pattern.MatchString(path)

		require.Equal(t, want, checker.ShouldFilter(path),
			"the checker disagrees with its own configuration for %q", path)
	})
}

// FuzzShouldFilterIsCacheStable pins that the verdict does not depend on how
// the checker was warmed.
//
// Both outcomes are memoized in an LRU, so a hit and a miss reach the same
// decision by different paths. If they can disagree, whether a request is
// exempt depends on which other paths happened to arrive first — an
// intermittent bypass that no single request reproduces.
func FuzzShouldFilterIsCacheStable(f *testing.F) {
	for _, seed := range pathSeeds {
		f.Add(seed, seed)
	}
	f.Add("/healthz", "/api/orders")

	cached, err := endpointfilter.New(ignored, endpointfilter.WithIgnorePatterns(pattern))
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, warm, path string) {
		// Warm the cache with an unrelated path first.
		cached.ShouldFilter(warm)

		first := cached.ShouldFilter(path)
		second := cached.ShouldFilter(path)
		require.Equal(t, first, second, "a warm cache changed the verdict for %q", path)

		fresh, err := endpointfilter.New(ignored, endpointfilter.WithIgnorePatterns(pattern))
		require.NoError(t, err)
		require.Equal(t, fresh.ShouldFilter(path), first,
			"a cold checker disagrees with a warm one for %q", path)
	})
}
