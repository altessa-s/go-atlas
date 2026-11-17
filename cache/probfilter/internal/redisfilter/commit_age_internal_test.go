// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	goredis "github.com/redis/go-redis/v9"
)

// The age of a commit is the server clock's current time minus the commit
// time it recorded: it grows with the server clock only, and the commit time
// stays the identity of the commit until the next one.
func TestCore_RebuildCommitAge_MeasuredOnServerClock(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)

	_, _, known, err := core.RebuildCommitAge(t.Context())
	require.NoError(t, err)
	require.False(t, known)

	// A server clock far from the local one: nothing here depends on it.
	at := time.UnixMilli(1_700_000_000_000)
	mr.SetTime(at)
	commitStaged(t, mr, core)

	for _, elapsed := range []time.Duration{0, 90 * time.Second, 3 * time.Hour} {
		mr.SetTime(at.Add(elapsed))
		age, committed, known, err := core.RebuildCommitAge(t.Context())
		require.NoError(t, err)
		require.True(t, known)
		require.Equal(t, elapsed, age)
		require.True(t, at.Equal(committed), "re-reading the same commit reports the same commit time")
	}

	next := at.Add(4 * time.Hour)
	mr.SetTime(next)
	commitStaged(t, mr, core)
	age, committed, known, err := core.RebuildCommitAge(t.Context())
	require.NoError(t, err)
	require.True(t, known)
	require.Zero(t, age)
	require.True(t, next.Equal(committed))
}

func TestCore_RebuildCommitAge_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("server clock set back reports zero", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		at := time.UnixMilli(1_700_000_000_000)
		mr.SetTime(at)
		commitStaged(t, mr, core)
		mr.SetTime(at.Add(-time.Hour))
		age, _, known, err := core.RebuildCommitAge(t.Context())
		require.NoError(t, err)
		require.True(t, known)
		require.Zero(t, age)
	})

	t.Run("live key gone", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitStaged(t, mr, core)
		mr.Del("t:f")
		_, _, known, err := core.RebuildCommitAge(t.Context())
		require.NoError(t, err)
		require.False(t, known)
	})

	t.Run("marker with a list size", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		at := time.UnixMilli(1_700_000_000_000)
		mr.SetTime(at)
		commitLoaded(t, mr, core, 5)
		mr.SetTime(at.Add(time.Minute))
		age, committed, known, err := core.RebuildCommitAge(t.Context())
		require.NoError(t, err)
		require.True(t, known)
		require.Equal(t, time.Minute, age)
		require.True(t, at.Equal(committed))
	})

	t.Run("malformed marker", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitStaged(t, mr, core)
		require.NoError(t, mr.Set(core.readyKey, "soon"))
		_, _, _, err := core.RebuildCommitAge(t.Context())
		require.Error(t, err)
	})

	t.Run("reserved key", func(t *testing.T) {
		t.Parallel()
		mr, _, _ := newReadyCore(t)
		client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		_, _, _, err := New(client, MetaPrefix+"x", readyCommands).RebuildCommitAge(t.Context())
		require.ErrorIs(t, err, ErrReservedKey)
	})
}
