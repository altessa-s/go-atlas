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

func requireCommitTime(t *testing.T, core *Core, want time.Time) {
	t.Helper()
	got, err := core.LastRebuildCommit(t.Context())
	require.NoError(t, err)
	require.True(t, want.Equal(got), "want %s, got %s", want, got)
}

// The commit time is the server clock at the promotion, and every commit
// moves it on.
func TestCore_LastRebuildCommit_IsServerTimeOfCommit(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	requireCommitTime(t, core, time.Time{})

	first := time.UnixMilli(1_700_000_000_123)
	mr.SetTime(first)
	commitStaged(t, mr, core)
	requireCommitTime(t, core, first)

	second := first.Add(90 * time.Minute)
	mr.SetTime(second)
	commitStaged(t, mr, core)
	requireCommitTime(t, core, second)
}

// Every process sharing the filter reads the commit of any of them.
func TestCore_LastRebuildCommit_VisibleToPeer(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	peer := New(client, "t:f", readyCommands)

	at := time.UnixMilli(1_700_000_000_000)
	mr.SetTime(at)
	commitStaged(t, mr, core)
	requireCommitTime(t, peer, at)
}

// A filter key that is gone, and a ready marker written by a release that did
// not record the time, both report no commit time.
func TestCore_LastRebuildCommit_UnknownIsZero(t *testing.T) {
	t.Parallel()

	t.Run("live key gone", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitStaged(t, mr, core)
		mr.Del("t:f")
		requireCommitTime(t, core, time.Time{})
	})

	t.Run("legacy marker", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitStaged(t, mr, core)
		require.NoError(t, mr.Set(core.readyKey, "1"))
		requireCommitTime(t, core, time.Time{})
		requireReady(t, core, true)
	})

	t.Run("malformed marker", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitStaged(t, mr, core)
		require.NoError(t, mr.Set(core.readyKey, "soon"))
		_, err := core.LastRebuildCommit(t.Context())
		require.Error(t, err)
	})
}

func TestCore_LastRebuildCommit_ReservedKey(t *testing.T) {
	t.Parallel()
	mr, _, _ := newReadyCore(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	_, err := New(client, MetaPrefix+"x", readyCommands).LastRebuildCommit(t.Context())
	require.ErrorIs(t, err, ErrReservedKey)
}
