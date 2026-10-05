// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
)

func TestCore_Stage_ReservesStagingKeyInSameSlot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		live       string
		wantPrefix string
	}{
		{name: "plain key becomes the hash tag", live: "t:f", wantPrefix: redisfilter.MetaPrefix + "{t:f}:staging:"},
		{name: "existing hash tag is kept", live: "t:{tenant}:f", wantPrefix: redisfilter.MetaPrefix + "{tenant}:staging:"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mr, client := newTestClient(t)
			fake := &fakeModule{}
			fake.register(t, mr, testCommands)

			core := redisfilter.New(client, tc.live, testCommands, int64(100))
			st, err := core.Stage(t.Context(), 0.5, int64(42))
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(st.Key(), tc.wantPrefix), "staging key %q", st.Key())

			state := fake.snapshot()
			require.Equal(t, [][]string{{st.Key(), "0.5", "42"}}, state.reserveCalls,
				"the staging filter is reserved with the explicit args")
		})
	}
}

func TestCore_Stage_UniqueKeys(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	require.NoError(t, mr.Server().Register(testCommands.Reserve, func(c *server.Peer, _ string, _ []string) { c.WriteOK() }))

	core := redisfilter.New(client, "t:f", testCommands)
	a, err := core.Stage(t.Context())
	require.NoError(t, err)
	b, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, a.Key(), b.Key())
}

func TestStaging_AddBatchCommit(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)
	require.NoError(t, mr.Set("t:f", "old"))

	core := redisfilter.New(client, "t:f", testCommands)
	st, err := core.Stage(t.Context(), int64(10))
	require.NoError(t, err)
	require.NoError(t, st.AddBatch(t.Context(), slices.Values([]string{"a", "b"})))
	require.Equal(t, [][]string{{st.Key(), "NOCREATE", "ITEMS", "a", "b"}}, fake.snapshot().insertCalls,
		"adds go to the staging key with the non-creating batch command")

	// Stand-in for the module value the fake reserve does not materialize.
	require.NoError(t, mr.Set(st.Key(), "new"))
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got, "the live key is untouched before Commit")

	require.NoError(t, st.Commit(t.Context()))
	got, err = mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got, "Commit renames the staging key onto the live key")
	require.False(t, mr.Exists(st.Key()))
}

func TestStaging_CommitCanceledContext(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)
	require.NoError(t, mr.Set("t:f", "old"))

	st, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, st.Commit(ctx), context.Canceled)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got)
}

func TestStaging_CommitMissingStagingKey(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)
	require.NoError(t, mr.Set("t:f", "old"))

	st, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.NoError(t, err)

	err = st.Commit(t.Context())
	require.ErrorContains(t, err, "promote Redis Test filter")
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got)
}

func TestStaging_Abort(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)
	require.NoError(t, mr.Set("t:f", "old"))

	st, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	require.NoError(t, st.Abort(t.Context()))
	require.False(t, mr.Exists(st.Key()))
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got)
}

func TestCore_Stage_ReserveError(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	require.NoError(t, mr.Server().Register(testCommands.Reserve, func(c *server.Peer, _ string, _ []string) {
		c.WriteError("ERR boom")
	}))

	_, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.ErrorContains(t, err, "create Redis Test filter")
}

func TestStaging_TTL(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)

	st, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.NoError(t, err)

	// Stand-in for the module value the fake reserve does not materialize.
	require.NoError(t, mr.Set(st.Key(), "new"))
	require.NoError(t, st.AddBatch(t.Context(), slices.Values([]string{"a"})))
	require.Equal(t, redisfilter.StagingTTL, mr.TTL(st.Key()), "every staging batch refreshes the TTL")

	require.NoError(t, st.Commit(t.Context()))
	require.Zero(t, mr.TTL("t:f"), "the staging TTL must not reach the live key")
}

func TestStaging_ExpiredKeyIsNotRecreated(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)

	st, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.NoError(t, err)

	// Simulate the staging filter expiring mid-rebuild.
	fake.mu.Lock()
	fake.created = false
	fake.mu.Unlock()

	err = st.AddBatch(t.Context(), slices.Values([]string{"a"}))
	require.ErrorContains(t, err, "batch add to Redis Test filter")
	require.Len(t, fake.snapshot().reserveCalls, 1, "a staging filter must never be auto-reserved")
}
