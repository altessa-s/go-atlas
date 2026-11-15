// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	goredis "github.com/redis/go-redis/v9"
)

// commitLoaded stages a replacement, reports loaded as its list size and
// commits it.
func commitLoaded(t *testing.T, mr *miniredis.Miniredis, core *Core, loaded int64) {
	t.Helper()
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "rebuilt"))
	st.SetLoaded(loaded)
	require.NoError(t, st.Commit(t.Context()))
}

func requireLoaded(t *testing.T, core *Core, want int64, wantKnown bool) {
	t.Helper()
	loaded, known, err := core.LastRebuildLoaded(t.Context())
	require.NoError(t, err)
	require.Equal(t, wantKnown, known)
	require.Equal(t, want, loaded)
}

// The commit publishes the size of the list it loaded next to the commit
// time, and every process sharing the filter reads both.
func TestCore_LastRebuildLoaded_RecordedByCommit(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	peer := New(client, "t:f", readyCommands)
	requireLoaded(t, peer, 0, false)

	at := time.UnixMilli(1_700_000_000_000)
	mr.SetTime(at)
	commitLoaded(t, mr, core, 42)
	requireLoaded(t, peer, 42, true)
	requireCommitTime(t, peer, at)

	commitLoaded(t, mr, core, 0)
	requireLoaded(t, peer, 0, true)
}

// A commit that was not told the list size, and markers written by releases
// that recorded only the time or nothing, report the size as unknown; the
// commit time stays readable where it was recorded.
func TestCore_LastRebuildLoaded_UnknownFormats(t *testing.T) {
	t.Parallel()

	t.Run("commit without a reported size", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		at := time.UnixMilli(1_700_000_000_000)
		mr.SetTime(at)
		commitStaged(t, mr, core)
		requireLoaded(t, core, 0, false)
		requireCommitTime(t, core, at)
	})

	t.Run("timestamp-only marker", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitLoaded(t, mr, core, 7)
		require.NoError(t, mr.Set(core.readyKey, "1700000000000"))
		requireLoaded(t, core, 0, false)
		requireCommitTime(t, core, time.UnixMilli(1_700_000_000_000))
		requireReady(t, core, true)
	})

	t.Run("legacy marker", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitLoaded(t, mr, core, 7)
		require.NoError(t, mr.Set(core.readyKey, "1"))
		requireLoaded(t, core, 0, false)
	})

	t.Run("live key gone", func(t *testing.T) {
		t.Parallel()
		mr, core, _ := newReadyCore(t)
		commitLoaded(t, mr, core, 7)
		mr.Del("t:f")
		requireLoaded(t, core, 0, false)
	})

	for name, value := range map[string]string{"malformed size": "1700000000000:many", "negative size": "1700000000000:-1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mr, core, _ := newReadyCore(t)
			commitLoaded(t, mr, core, 7)
			require.NoError(t, mr.Set(core.readyKey, value))
			_, _, err := core.LastRebuildLoaded(t.Context())
			require.Error(t, err)
		})
	}
}

func TestCore_LastRebuildLoaded_ReservedKey(t *testing.T) {
	t.Parallel()
	mr, _, _ := newReadyCore(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	_, _, err := New(client, MetaPrefix+"x", readyCommands).LastRebuildLoaded(t.Context())
	require.ErrorIs(t, err, ErrReservedKey)
}
