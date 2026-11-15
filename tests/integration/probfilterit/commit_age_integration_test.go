// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
)

// ageFilter is what the commit-age scenarios need of a facade.
type ageFilter interface {
	sharedFilter
	probfilter.RebuildCommitAgeReporter
}

// TestRebuildCommitAge_MeasuredOnServerClock asserts, on a real server, that
// the age of a commit is computed inside Redis from its own clock: a peer
// reads a fresh commit as fresh, and a marker offset into the server's past
// or future — what a replica whose clock disagrees with the server's would
// otherwise misjudge — yields the offset as the age, never a negative one.
func TestRebuildCommitAge_MeasuredOnServerClock(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]func(*testing.T, *fixture) ageFilter{
		"bloom":  func(t *testing.T, fx *fixture) ageFilter { return fx.bloom(t) },
		"cuckoo": func(t *testing.T, fx *fixture) ageFilter { return fx.cuckoo(t) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fx := newFixture(t)
			a, b := open(t, fx), open(t, fx)
			ctx := t.Context()

			_, _, known, err := b.RebuildCommitAge(ctx)
			require.NoError(t, err)
			require.False(t, known)

			require.NoError(t, a.Rebuild(ctx, openLoader(values("listed", 10), false)))
			age, committed, known, err := b.RebuildCommitAge(ctx)
			require.NoError(t, err)
			require.True(t, known)
			require.Less(t, age, 5*time.Second, "a commit just made is fresh for a peer")
			require.True(t, committed.Equal(lastCommit(t, a)))

			// The commit as if made ten minutes ago on the server clock.
			past := serverTime(t, fx).Add(-10 * time.Minute)
			require.NoError(t, fx.admin.Set(ctx, fx.metaKey("ready"), strconv.FormatInt(past.UnixMilli(), 10), 0).Err())
			age, committed, known, err = b.RebuildCommitAge(ctx)
			require.NoError(t, err)
			require.True(t, known)
			require.True(t, past.Equal(committed))
			require.GreaterOrEqual(t, age, 10*time.Minute)
			require.Less(t, age, 10*time.Minute+5*time.Second)

			// A marker ahead of the server clock never yields a negative age.
			future := serverTime(t, fx).Add(10 * time.Minute)
			require.NoError(t, fx.admin.Set(ctx, fx.metaKey("ready"), strconv.FormatInt(future.UnixMilli(), 10)+":10", 0).Err())
			age, committed, known, err = b.RebuildCommitAge(ctx)
			require.NoError(t, err)
			require.True(t, known)
			require.True(t, future.Equal(committed))
			require.Zero(t, age)
		})
	}
}
