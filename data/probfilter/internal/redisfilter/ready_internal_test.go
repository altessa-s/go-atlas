// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	goredis "github.com/redis/go-redis/v9"
)

// readyCommands mirror the NOCREATE insert form of the RedisBloom storages.
var readyCommands = Commands{
	Label:       "Test",
	Add:         "T.INSERT",
	AddTokens:   []string{"NOCREATE", "ITEMS"},
	AddBatch:    "T.INSERT",
	BatchTokens: []string{"NOCREATE", "ITEMS"},
	Reserve:     "T.RESERVE",
}

// newReadyCore returns a Core over miniredis whose fake T.INSERT behaves like
// a NOCREATE insert: on a missing live key it fails with RedisBloom's
// "ERR not found" until T.RESERVE ran (T.RESERVE runs inside a script, where
// the fake cannot write keys, so the insert materializes the reserved key).
// The returned flag reports whether the fake filter is currently reserved.
func newReadyCore(t *testing.T) (*miniredis.Miniredis, *Core, *atomic.Bool) {
	t.Helper()
	mr := miniredis.RunT(t)
	reserved := &atomic.Bool{}
	require.NoError(t, mr.Server().Register("T.RESERVE", func(c *server.Peer, _ string, _ []string) {
		reserved.Store(true)
		c.WriteOK()
	}))
	require.NoError(t, mr.Server().Register("T.INSERT", func(c *server.Peer, _ string, args []string) {
		key := args[0]
		if !mr.Exists(key) {
			if !reserved.Load() {
				c.WriteError("ERR not found")
				return
			}
			_ = mr.Set(key, "filter")
		}
		items := len(args) - 1 - slices.Index(args, "ITEMS")
		c.WriteLen(items)
		for range items {
			c.WriteInt(1)
		}
	}))
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, New(client, "t:f", readyCommands), reserved
}

// commitStaged stages a replacement, gives it stand-in contents and commits it.
func commitStaged(t *testing.T, mr *miniredis.Miniredis, core *Core) {
	t.Helper()
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "rebuilt"))
	require.NoError(t, st.Commit(t.Context()))
}

func requireReady(t *testing.T, core *Core, want bool) {
	t.Helper()
	got, err := core.RebuildCommitted(t.Context())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestCore_RebuildCommitted_SetByPromotingCommit(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	requireReady(t, core, false)

	require.NoError(t, core.Add(t.Context(), "v"))
	requireReady(t, core, false) // written, never rebuilt

	commitStaged(t, mr, core)
	requireReady(t, core, true)
}

// TestCore_RebuildCommitted_PartialCommitNotReady fails a commit after it
// advanced the generation but before the rename: no ready marker may exist.
func TestCore_RebuildCommitted_PartialCommitNotReady(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	require.NoError(t, mr.Set("t:f", "old"))
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	partial := strings.Replace(commitScriptSource, "redis.call('PERSIST'", "do return redis.error_reply('ERR injected') end --", 1)
	require.NotEqual(t, commitScriptSource, partial)
	_, err = goredis.NewScript(partial).Run(t.Context(), st.core.client, commitKeys(st), int64(60000), "0").Result()
	require.ErrorContains(t, err, "injected")
	require.True(t, mr.Exists(core.genKey), "the failed attempt advanced the generation")
	requireReady(t, core, false)

	require.NoError(t, st.Commit(t.Context()))
	requireReady(t, core, true)
}

func TestCore_RebuildCommitted_LiveKeyGone(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	commitStaged(t, mr, core)
	requireReady(t, core, true)

	mr.Del("t:f") // deleted or evicted
	requireReady(t, core, false)
}

// TestCore_RebuildCommitted_RecreationClearsMarker deletes the live key after
// a commit (with the reservation already latched) and writes again: the
// write recreates the filter through the reserve path, which must drop the
// ready marker, so the recreated filter is not reported as rebuilt.
func TestCore_RebuildCommitted_RecreationClearsMarker(t *testing.T) {
	t.Parallel()
	for name, write := range map[string]func(*testing.T, *Core) error{
		"Add": func(t *testing.T, c *Core) error { return c.Add(t.Context(), "v") },
		"AddBatch": func(t *testing.T, c *Core) error {
			return c.AddBatch(t.Context(), slices.Values([]string{"v", "w"}))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mr, core, reserved := newReadyCore(t)
			require.NoError(t, write(t, core)) // latches the reservation
			commitStaged(t, mr, core)
			requireReady(t, core, true)

			mr.Del("t:f")
			reserved.Store(false)

			require.NoError(t, write(t, core), "a missing filter is recreated")
			require.True(t, mr.Exists("t:f"))
			require.False(t, mr.Exists(core.readyKey))
			requireReady(t, core, false)

			commitStaged(t, mr, core)
			requireReady(t, core, true)
		})
	}
}

func TestCore_EnsureFilter_ExistingKeepsMarker(t *testing.T) {
	t.Parallel()
	mr, core, _ := newReadyCore(t)
	commitStaged(t, mr, core)

	require.NoError(t, core.EnsureFilter(t.Context()))
	requireReady(t, core, true)
}

func TestNotExist(t *testing.T) {
	t.Parallel()
	for msg, want := range map[string]bool{
		"ERR not found":                     true,
		"ERR not found: key does not exist": true,
		"ERR key does not exist":            true,
		"ERR item exists":                   false,
		"ERR Filter is full":                false,
	} {
		require.Equal(t, want, notExist(errors.New(msg)), msg)
	}
}
