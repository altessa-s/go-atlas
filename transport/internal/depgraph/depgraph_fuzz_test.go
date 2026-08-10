// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package depgraph

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// fuzzItem is a named node with declared dependencies.
type fuzzItem struct {
	name string
	deps []string
}

func (f *fuzzItem) Name() string           { return f.name }
func (f *fuzzItem) Dependencies() []string { return f.deps }

// FuzzBuildKeepsEveryItem pins the property the whole ordering rests on:
// sorting a list must not change what is in it.
//
// Build orders middlewares and interceptors, so an item silently dropped is a
// control that stops running — an auth or a rate limiter absent from the chain
// with nothing in the response to say so. A duplicated item is the mirror
// failure: a middleware that runs twice, double-counting or double-charging.
//
// Items sharing a name are excluded, and the exclusion is itself worth knowing:
// the graph is keyed by name, so registering two items under one name keeps
// only the last and reports nothing. Whether that should be an error is a
// contract decision this target does not make on its own.
func FuzzBuildKeepsEveryItem(f *testing.F) {
	f.Add("a", "b", "c", "b", "")
	f.Add("a", "a", "a", "", "")
	f.Add("", "", "", "", "")
	f.Add("a", "b", "c", "c", "a")
	f.Add("a", "b", "c", "missing", "")

	f.Fuzz(func(t *testing.T, name1, name2, name3, dep1, dep2 string) {
		if name1 == name2 || name2 == name3 || name1 == name3 {
			t.Skip("the graph is keyed by name; duplicates collapse rather than order")
		}

		items := []*fuzzItem{
			{name: name1},
			{name: name2, deps: nonEmpty(dep1, dep2)},
			{name: name3},
		}

		sorted, err := Build(items, slog.New(slog.DiscardHandler))
		if err != nil {
			require.Nil(t, sorted, "a rejected graph must not also produce an ordering")
			return
		}

		require.Len(t, sorted, len(items),
			"the ordering has a different length than its input: %v", names(sorted))

		for _, item := range items {
			require.True(t, slices.Contains(sorted, item),
				"item %q was dropped from the ordering: %v", item.Name(), names(sorted))
		}
	})
}

// FuzzBuildPlacesDependenciesFirst pins what the ordering is for: a dependency
// runs before whatever declared it.
//
// This is the entire contract. An ordering that satisfies "same items, same
// count" while putting a dependent ahead of its dependency looks correct in
// every length check and produces a chain where, say, the request-id middleware
// runs after the logger that was supposed to record it.
func FuzzBuildPlacesDependenciesFirst(f *testing.F) {
	f.Add("logger", "auth", "metrics", "logger", "")
	f.Add("a", "b", "c", "a", "c")
	f.Add("a", "b", "c", "", "")
	f.Add("x", "y", "z", "z", "y")

	f.Fuzz(func(t *testing.T, name1, name2, name3, dep1, dep2 string) {
		if name1 == name2 || name2 == name3 || name1 == name3 {
			t.Skip("the graph is keyed by name; duplicates collapse rather than order")
		}

		items := []*fuzzItem{
			{name: name1},
			{name: name2, deps: nonEmpty(dep1, dep2)},
			{name: name3},
		}

		sorted, err := Build(items, slog.New(slog.DiscardHandler))
		if err != nil {
			return
		}

		position := make(map[string]int, len(sorted))
		for i, item := range sorted {
			position[item.Name()] = i
		}

		for _, item := range sorted {
			for _, dep := range item.Dependencies() {
				depAt, declared := position[dep]
				if !declared {
					continue // An unregistered optional dependency is documented as ignored.
				}
				if dep == item.Name() {
					continue // A self-edge orders nothing.
				}
				require.Less(t, depAt, position[item.Name()],
					"%q runs before its dependency %q: %v", item.Name(), dep, names(sorted))
			}
		}
	})
}

// nonEmpty drops the empty strings the fuzzer supplies for "no dependency".
func nonEmpty(deps ...string) []string {
	var out []string
	for _, dep := range deps {
		if dep != "" {
			out = append(out, dep)
		}
	}
	return out
}

// names renders an ordering for a failure message.
func names(items []*fuzzItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name())
	}
	return out
}
