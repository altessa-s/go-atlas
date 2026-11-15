// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
)

// listFilter is what the list-size scenarios need of a facade.
type listFilter interface {
	sharedFilter
	probfilter.RebuildLoadReporter
}

func requireLoaded(tb testing.TB, f probfilter.RebuildLoadReporter, want int64, wantKnown bool) {
	tb.Helper()
	loaded, known, err := f.LastRebuildLoaded(tb.Context())
	require.NoError(tb, err)
	require.Equal(tb, wantKnown, known)
	require.Equal(tb, want, loaded)
}

// TestRebuildLoaded_SharedAcrossInstances asserts, on a real server, that the
// commit records the size of the list the rebuild loaded and that every
// instance reads it: an empty committed list stays "0 loaded" while live
// inserts — what the OIDC provider's MarkRevoked does — raise the filter's
// item count, a peer's populated rebuild is seen by the instance that loaded
// nothing, and a failed rebuild leaves the size alone.
func TestRebuildLoaded_SharedAcrossInstances(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]func(*testing.T, *fixture) listFilter{
		"bloom":  func(t *testing.T, fx *fixture) listFilter { return fx.bloom(t) },
		"cuckoo": func(t *testing.T, fx *fixture) listFilter { return fx.cuckoo(t) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			a, b := open(t, fx), open(t, fx)
			ctx := t.Context()
			requireLoaded(t, b, 0, false)

			require.NoError(t, a.Rebuild(ctx, openLoader(nil, false)))
			requireLoaded(t, b, 0, true)

			require.NoError(t, b.Add(ctx, "marked:1"))
			require.NoError(t, a.Add(ctx, "marked:2"))
			stats, err := a.Stats(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(2), stats.ItemCount, "live inserts are in the filter")
			requireLoaded(t, a, 0, true)
			requireLoaded(t, b, 0, true)

			require.NoError(t, b.Rebuild(ctx, openLoader(values("listed", 30), true)))
			requireLoaded(t, a, 30, true)

			failing := openLoader(values("broken", 50), false)
			failing.failAfter = 10
			require.ErrorIs(t, a.Rebuild(ctx, failing), errSourceFailed)
			requireLoaded(t, b, 30, true)

			require.NoError(t, a.Rebuild(ctx, openLoader(nil, false)))
			requireLoaded(t, b, 0, true)
		})
	}
}

// TestRebuildLoaded_InsertsDuringRebuildDoNotCount asserts that values a peer
// inserts while a Bloom rebuild runs, which the commit replays onto the
// replacement, are in the rebuilt filter but not in the recorded list size.
func TestRebuildLoaded_InsertsDuringRebuildDoNotCount(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a, b := fx.bloom(t), fx.bloom(t)
	ctx := t.Context()
	require.NoError(t, a.Rebuild(ctx, openLoader(values("old", 5), false)))

	held := newGatedLoader(values("new", 20), false)
	done := rebuildInBackground(t, a, held)
	require.NoError(t, b.Add(ctx, "marked:during"))
	require.NoError(t, awaitRebuild(t, held, done))

	requirePresent(t, b, "marked:during")
	requireLoaded(t, b, 20, true)
}

// TestRebuildLoaded_OlderMarkerFormats asserts that ready markers written by
// earlier releases — the commit time alone, or "1" — report the list size as
// unknown on a real server, and that the next commit records it again.
func TestRebuildLoaded_OlderMarkerFormats(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a := fx.bloom(t)
	ctx := t.Context()
	require.NoError(t, a.Rebuild(ctx, openLoader(values("listed", 10), false)))
	requireLoaded(t, a, 10, true)
	committed := lastCommit(t, a)

	require.NoError(t, fx.admin.Set(ctx, fx.metaKey("ready"), strconv.FormatInt(committed.UnixMilli(), 10), 0).Err())
	requireLoaded(t, a, 0, false)
	require.True(t, committed.Equal(lastCommit(t, a)), "the commit time of a timestamp-only marker stays readable")

	require.NoError(t, fx.admin.Set(ctx, fx.metaKey("ready"), "1", 0).Err())
	requireLoaded(t, a, 0, false)
	require.True(t, lastCommit(t, a).IsZero())

	require.NoError(t, a.Rebuild(ctx, openLoader(values("listed", 12), false)))
	requireLoaded(t, a, 12, true)
}
