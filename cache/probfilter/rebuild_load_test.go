// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"
	"github.com/altessa-s/go-atlas/cache/probfilter/cuckoo"

	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	cuckoomemory "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/memory"
)

// loadReportingFilter is what the load-count scenarios need of a facade.
type loadReportingFilter interface {
	probfilter.RebuildableFilter
	probfilter.RebuildLoadReporter
}

func requireRebuildLoaded(t *testing.T, f probfilter.RebuildLoadReporter, want int64, wantKnown bool) {
	t.Helper()
	loaded, known, err := f.LastRebuildLoaded(t.Context())
	require.NoError(t, err)
	require.Equal(t, wantKnown, known)
	require.Equal(t, want, loaded)
}

// A memory-backed filter reports the size of the list its own last committed
// rebuild loaded: values added to the filter do not count, and a failed
// rebuild leaves the report alone.
func TestMemoryFilter_LastRebuildLoaded(t *testing.T) {
	t.Parallel()

	errSource := errors.New("source failed")
	for name, f := range map[string]loadReportingFilter{
		"bloom":  bloom.New(bloommemory.New()),
		"cuckoo": cuckoo.New(cuckoomemory.New()),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			requireRebuildLoaded(t, f, 0, false)

			require.NoError(t, f.Add(ctx, "added"))
			requireRebuildLoaded(t, f, 0, false)

			values := []string{"a", "b", "c"}
			require.NoError(t, f.Rebuild(ctx, probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values(values) })))
			requireRebuildLoaded(t, f, 3, true)

			require.NoError(t, f.Add(ctx, "added later"))
			requireRebuildLoaded(t, f, 3, true)

			counted := probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values(values[:2]) }, probfilter.WithCount(2))
			require.NoError(t, f.Rebuild(ctx, counted))
			requireRebuildLoaded(t, f, 2, true)

			failing := probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) {
					if yield("a", nil) {
						yield("", errSource)
					}
				}
			})
			require.ErrorIs(t, f.Rebuild(ctx, failing), errSource)
			requireRebuildLoaded(t, f, 2, true)

			require.NoError(t, f.Rebuild(ctx, probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values([]string(nil)) })))
			requireRebuildLoaded(t, f, 0, true)
		})
	}
}
