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
// A name that is empty or repeated is no longer skipped: the graph is keyed by
// name, so neither can be placed, and Build refuses both rather than returning
// a shorter list. Asserting that here is what keeps the refusal from quietly
// regressing back into a silent drop — the failure this target was written for.
func FuzzBuildKeepsEveryItem(f *testing.F) {
	f.Add("a", "b", "c", "b", "")
	f.Add("a", "a", "a", "", "")
	f.Add("", "", "", "", "")
	f.Add("a", "b", "c", "c", "a")
	f.Add("a", "b", "c", "missing", "")

	f.Fuzz(func(t *testing.T, name1, name2, name3, dep1, dep2 string) {
		items := []*fuzzItem{
			{name: name1},
			{name: name2, deps: nonEmpty(dep1, dep2)},
			{name: name3},
		}

		sorted, err := Build(items, slog.New(slog.DiscardHandler))
		if err != nil {
			require.Nil(t, sorted, "a rejected graph must not also produce an ordering")

			// A name the graph cannot key by must be reported as such, not as
			// some other failure that happens to also stop the build. Build
			// walks the items in order and reports the first offender, so the
			// oracle walks the same way: with both an empty and a repeated name
			// present, neither kind outranks the other — position decides.
			if want := firstNameFault(name1, name2, name3); want != nil {
				require.ErrorIs(t, err, want)
			}
			return
		}

		// Build succeeded, so nothing unorderable can have been in the input.
		// Checking each name on its own rather than their concatenation: three
		// names where only one is empty still concatenate to a non-empty
		// string, which is how a check like that passes while saying nothing.
		require.Nil(t, firstNameFault(name1, name2, name3),
			"an unnamed or repeated name was ordered instead of refused")

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

// firstNameFault reports which contract the names break first, walking them in
// the order Build does, or nil when they are all distinct and non-empty.
func firstNameFault(names ...string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return ErrEmptyName
		}
		if _, dup := seen[name]; dup {
			return ErrDuplicateName
		}
		seen[name] = struct{}{}
	}
	return nil
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

// FuzzDedupeRemovesOnlyNamedRepeats pins what deduplication may and may not
// discard.
//
// Dedupe is the deliberate escape hatch for callers whose list can legitimately
// repeat, so its whole value is that the discard is theirs rather than the
// graph's. Two failures would take that away: dropping something that was not a
// repeat, or collapsing unnamed items — which are not duplicates of each other,
// since there is no name to match on. The second is the mistake this helper
// itself shipped with, and it is the one worth a target.
func FuzzDedupeRemovesOnlyNamedRepeats(f *testing.F) {
	f.Add("a", "b", "c")
	f.Add("a", "a", "a")
	f.Add("", "", "")
	f.Add("", "a", "")
	f.Add("a", "", "a")

	f.Fuzz(func(t *testing.T, name1, name2, name3 string) {
		items := []*fuzzItem{{name: name1}, {name: name2}, {name: name3}}
		deduped := Dedupe(items)

		// An unnamed item is never a duplicate; a named one survives exactly once.
		want := 0
		seen := map[string]struct{}{}
		for _, name := range []string{name1, name2, name3} {
			if name == "" {
				want++
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			want++
		}
		require.Len(t, deduped, want,
			"Dedupe discarded something that was not a named repeat: %v", names(deduped))

		// Order is preserved: the survivors appear in their original sequence.
		require.Equal(t, deduped, Dedupe(items)[:len(deduped)])

		// Idempotent — a second pass has nothing left to remove.
		require.Equal(t, names(deduped), names(Dedupe(deduped)))

		// And the result is orderable unless an unnamed item remains, which
		// Build refuses for a reason Dedupe deliberately does not pre-empt.
		_, err := Build(deduped, slog.New(slog.DiscardHandler))
		if name1 == "" || name2 == "" || name3 == "" {
			require.ErrorIs(t, err, ErrEmptyName)
			return
		}
		require.NoError(t, err, "a deduplicated, fully named list must be orderable")
	})
}
