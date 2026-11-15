// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
)

// sharedFilter is what the commit-time scenarios need of a facade.
type sharedFilter interface {
	probfilter.RebuildableFilter
	probfilter.RebuildCommitTimeReporter
	probfilter.StatsProvider
}

// serverTime reads the Redis clock, truncated to the millisecond the commit
// script records.
func serverTime(tb testing.TB, fx *fixture) time.Time {
	tb.Helper()
	now, err := fx.admin.Time(tb.Context()).Result()
	require.NoError(tb, err)
	return now.Truncate(time.Millisecond)
}

func lastCommit(tb testing.TB, f probfilter.RebuildCommitTimeReporter) time.Time {
	tb.Helper()
	at, err := f.LastRebuildCommit(tb.Context())
	require.NoError(tb, err)
	return at
}

// TestRebuildCommitTime_SharedAcrossInstances asserts, on a real server, that
// the commit script records the Redis clock at the promotion and that every
// instance sharing the filter reads it: a peer sees the commit of a rebuild
// it did not run, a failed rebuild leaves the time alone, and the next commit
// moves it on.
func TestRebuildCommitTime_SharedAcrossInstances(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]func(*testing.T, *fixture) sharedFilter{
		"bloom":  func(t *testing.T, fx *fixture) sharedFilter { return fx.bloom(t) },
		"cuckoo": func(t *testing.T, fx *fixture) sharedFilter { return fx.cuckoo(t) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			a, b := open(t, fx), open(t, fx)
			ctx := t.Context()
			require.True(t, lastCommit(t, b).IsZero(), "a filter nobody rebuilt has no commit time")

			before := serverTime(t, fx)
			require.NoError(t, a.Rebuild(ctx, openLoader(values("first", 50), true)))
			after := serverTime(t, fx)

			first := lastCommit(t, b)
			require.False(t, first.Before(before), "commit %s precedes the rebuild (%s)", first, before)
			require.False(t, first.After(after), "commit %s follows the rebuild (%s)", first, after)
			require.True(t, first.Equal(lastCommit(t, a)), "both instances read the same instant")
			require.True(t, b.LastRebuild().IsZero(), "LastRebuild stays local to the instance that rebuilt")

			failing := openLoader(values("broken", 50), false)
			failing.failAfter = 10
			require.ErrorIs(t, b.Rebuild(ctx, failing), errSourceFailed)
			require.True(t, first.Equal(lastCommit(t, a)), "a failed rebuild publishes nothing")

			time.Sleep(5 * time.Millisecond)
			require.NoError(t, b.Rebuild(ctx, openLoader(values("second", 50), false)))
			require.True(t, lastCommit(t, a).After(first), "the next commit moves the time on")
		})
	}
}

// TestRebuildCommitTime_PeerRebuildInProgress is the sequence the OIDC
// revocation sync depends on: while a peer holds the rebuild lease this
// instance is refused, sees no new commit, and sees the peer's commit once
// the peer publishes.
func TestRebuildCommitTime_PeerRebuildInProgress(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	a, b := fx.bloom(t), fx.bloom(t)
	ctx := t.Context()
	require.NoError(t, a.Rebuild(ctx, openLoader(values("old", 20), false)))
	old := lastCommit(t, b)

	time.Sleep(5 * time.Millisecond)
	held := newGatedLoader(values("new", 20), false)
	done := rebuildInBackground(t, a, held)
	require.ErrorIs(t, b.Rebuild(ctx, openLoader(values("mine", 20), false)), probfilter.ErrRebuildInProgress)
	require.True(t, old.Equal(lastCommit(t, b)), "a rebuild in progress has committed nothing")

	require.NoError(t, awaitRebuild(t, held, done))
	require.True(t, lastCommit(t, b).After(old))
}

// TestRebuild_EmptySnapshotReportsNoItems asserts the item count the OIDC
// empty-list guard reads: zero after a rebuild from an empty source, and the
// loaded count after a peer rebuilt from a populated one.
func TestRebuild_EmptySnapshotReportsNoItems(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]func(*testing.T, *fixture) sharedFilter{
		"bloom":  func(t *testing.T, fx *fixture) sharedFilter { return fx.bloom(t) },
		"cuckoo": func(t *testing.T, fx *fixture) sharedFilter { return fx.cuckoo(t) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			a, b := open(t, fx), open(t, fx)
			itemCount := func(f sharedFilter) int64 {
				stats, err := f.Stats(context.WithoutCancel(t.Context()))
				require.NoError(t, err)
				return stats.ItemCount
			}

			require.NoError(t, a.Rebuild(t.Context(), openLoader(nil, false)))
			require.Zero(t, itemCount(a))
			require.False(t, lastCommit(t, a).IsZero(), "an empty rebuild commits too")

			require.NoError(t, b.Rebuild(t.Context(), openLoader(values("listed", 30), false)))
			require.Equal(t, int64(30), itemCount(a), "the instance that loaded nothing sees the peer's list")
		})
	}
}
