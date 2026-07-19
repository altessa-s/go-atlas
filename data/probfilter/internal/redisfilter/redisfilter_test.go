// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	goredis "github.com/redis/go-redis/v9"
)

// testCommands is a synthetic RedisBloom-like command set. miniredis does not
// ship the RedisBloom module, so the tests register fake T.* handlers on the
// miniredis server to exercise the shared plumbing end to end.
var testCommands = redisfilter.Commands{
	Label:    "Test",
	Exists:   "T.EXISTS",
	Add:      "T.ADD",
	AddBatch: "T.MADD",
	Reserve:  "T.RESERVE",
	Info:     "T.INFO",
}

// fakeModule records the T.* commands the Core sends and emulates the
// create-on-first-use behavior of RedisBloom: item commands fail with a
// "not exist" error until the reserve command has been seen.
type fakeModule struct {
	mu           sync.Mutex
	created      bool
	existsReply  int
	reserveCalls [][]string
	addCalls     [][]string
	batchCalls   [][]string
}

func (f *fakeModule) register(tb testing.TB, mr *miniredis.Miniredis, cmds redisfilter.Commands) {
	tb.Helper()

	srv := mr.Server()

	require.NoError(tb, srv.Register(cmds.Exists, func(c *server.Peer, _ string, args []string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		c.WriteInt(f.existsReply)
	}))

	require.NoError(tb, srv.Register(cmds.Add, func(c *server.Peer, _ string, args []string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.created {
			c.WriteError("ERR not found: key does not exist")
			return
		}
		f.addCalls = append(f.addCalls, slices.Clone(args))
		c.WriteInt(1)
	}))

	require.NoError(tb, srv.Register(cmds.AddBatch, func(c *server.Peer, _ string, args []string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.created {
			c.WriteError("ERR not found: key does not exist")
			return
		}
		f.batchCalls = append(f.batchCalls, slices.Clone(args))
		c.WriteInt(1)
	}))

	require.NoError(tb, srv.Register(cmds.Reserve, func(c *server.Peer, _ string, args []string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.created {
			c.WriteError("ERR item exists")
			return
		}
		f.created = true
		f.reserveCalls = append(f.reserveCalls, slices.Clone(args))
		c.WriteOK()
	}))
}

// moduleState is a lock-free copy of the fakeModule state for assertions.
type moduleState struct {
	created      bool
	reserveCalls [][]string
	addCalls     [][]string
	batchCalls   [][]string
}

func (f *fakeModule) snapshot() moduleState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return moduleState{
		created:      f.created,
		reserveCalls: slices.Clone(f.reserveCalls),
		addCalls:     slices.Clone(f.addCalls),
		batchCalls:   slices.Clone(f.batchCalls),
	}
}

func newTestClient(tb testing.TB) (*miniredis.Miniredis, goredis.UniversalClient) {
	tb.Helper()

	client, mr := testhelpers.RedisClient(tb)
	return mr, client
}

func TestCore_FilterKey(t *testing.T) {
	t.Parallel()

	_, client := newTestClient(t)
	core := redisfilter.New(client, "prefix:name", testCommands)
	require.Equal(t, "prefix:name", core.FilterKey())
}

func TestCore_MightExist(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{existsReply: 1}
	fake.register(t, mr, testCommands)

	core := redisfilter.New(client, "t:f", testCommands)

	ok, err := core.MightExist(t.Context(), "value")
	require.NoError(t, err)
	require.True(t, ok)

	fake.mu.Lock()
	fake.existsReply = 0
	fake.mu.Unlock()

	ok, err = core.MightExist(t.Context(), "value")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCore_MightExist_Error(t *testing.T) {
	t.Parallel()

	// No T.* handlers registered: miniredis answers "unknown command".
	_, client := newTestClient(t)
	core := redisfilter.New(client, "t:f", testCommands)

	_, err := core.MightExist(t.Context(), "value")
	require.Error(t, err)
	// The operation label is a compatibility contract of the public storages.
	require.ErrorContains(t, err, "failed to check existence in Redis Test filter")
}

func TestCore_Add_EnsuresFilterOnNotExist(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)

	core := redisfilter.New(client, "t:f", testCommands, 0.01, int64(500))

	require.NoError(t, core.Add(t.Context(), "value"))

	snap := fake.snapshot()
	require.True(t, snap.created, "reserve command should have created the filter")
	require.Equal(t, [][]string{{"t:f", "0.01", "500"}}, snap.reserveCalls)
	require.Equal(t, [][]string{{"t:f", "value"}}, snap.addCalls)
}

func TestCore_Add_ErrorWrapped(t *testing.T) {
	t.Parallel()

	_, client := newTestClient(t)
	core := redisfilter.New(client, "t:f", testCommands)

	err := core.Add(t.Context(), "value")
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to add to Redis Test filter")
}

func TestCore_AddBatch_ChunksAndPreservesOrder(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{created: true}
	fake.register(t, mr, testCommands)

	withTokens := testCommands
	withTokens.BatchTokens = []string{"ITEMS"}
	core := redisfilter.New(client, "t:f", withTokens)

	const total = 2500
	items := make([]string, total)
	for i := range items {
		items[i] = "item-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i%10))
	}

	require.NoError(t, core.AddBatch(t.Context(), slices.Values(items)))

	snap := fake.snapshot()
	require.Len(t, snap.batchCalls, 3, "2500 items with a 3-element header should flush 3 batches")

	var got []string
	for _, call := range snap.batchCalls {
		require.GreaterOrEqual(t, len(call), 2)
		require.Equal(t, "t:f", call[0], "batch must target the filter key")
		require.Equal(t, "ITEMS", call[1], "batch must carry the extra header tokens")
		got = append(got, call[2:]...)
	}
	require.Equal(t, items, got, "batched items must arrive complete and in order")
}

func TestCore_AddBatch_Empty(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{created: true}
	fake.register(t, mr, testCommands)

	core := redisfilter.New(client, "t:f", testCommands)

	require.NoError(t, core.AddBatch(t.Context(), slices.Values([]string(nil))))
	require.Empty(t, fake.snapshot().batchCalls, "no values must issue no commands")
}

func TestCore_AddBatch_EnsuresFilterOnNotExist(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)

	core := redisfilter.New(client, "t:f", testCommands, int64(100))

	require.NoError(t, core.AddBatch(t.Context(), slices.Values([]string{"a", "b"})))

	snap := fake.snapshot()
	require.Equal(t, [][]string{{"t:f", "100"}}, snap.reserveCalls)
	require.Equal(t, [][]string{{"t:f", "a", "b"}}, snap.batchCalls)
}

func TestCore_EnsureFilter_IgnoresExisting(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{created: true}
	fake.register(t, mr, testCommands)

	core := redisfilter.New(client, "t:f", testCommands, int64(100))

	// The fake replies "ERR item exists" for an already-created filter.
	require.NoError(t, core.EnsureFilter(t.Context()))
}

func TestCore_EnsureFilter_ErrorWrapped(t *testing.T) {
	t.Parallel()

	_, client := newTestClient(t)
	core := redisfilter.New(client, "t:f", testCommands, int64(100))

	err := core.EnsureFilter(t.Context())
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to create Redis Test filter")
}

func TestCore_Reserve_ExplicitArgs(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)

	// Configured args must be ignored in favor of the explicit ones.
	core := redisfilter.New(client, "t:f", testCommands, int64(100))

	require.NoError(t, core.Reserve(t.Context(), 0.5, int64(9000)))
	require.Equal(t, [][]string{{"t:f", "0.5", "9000"}}, fake.snapshot().reserveCalls)
}

func TestCore_Reserve_ExistingIsError(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	fake := &fakeModule{created: true}
	fake.register(t, mr, testCommands)

	core := redisfilter.New(client, "t:f", testCommands)

	err := core.Reserve(t.Context(), int64(100))
	require.Error(t, err, "unlike EnsureFilter, Reserve must surface an exists error")
	require.ErrorContains(t, err, "failed to create Redis Test filter")
}

func TestCore_DeleteFilter(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	require.NoError(t, mr.Set("t:f", "payload"))

	core := redisfilter.New(client, "t:f", testCommands)

	require.NoError(t, core.DeleteFilter(t.Context()))
	require.False(t, mr.Exists("t:f"), "filter key must be deleted")
}

func TestCore_Info(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	require.NoError(t, mr.Server().Register(testCommands.Info, func(c *server.Peer, _ string, _ []string) {
		c.WriteLen(4)
		c.WriteBulk("Capacity")
		c.WriteInt(100)
		c.WriteBulk("Number of items inserted")
		c.WriteInt(42)
	}))

	core := redisfilter.New(client, "t:f", testCommands)

	result, found, err := core.Info(t.Context())
	require.NoError(t, err)
	require.True(t, found)

	fields := map[string]int64{}
	for key, raw := range redisfilter.InfoFields(result) {
		val, convErr := redisfilter.ToInt64(raw)
		require.NoError(t, convErr)
		fields[key] = val
	}
	require.Equal(t, map[string]int64{"Capacity": 100, "Number of items inserted": 42}, fields)
}

func TestCore_Info_NotExist(t *testing.T) {
	t.Parallel()

	mr, client := newTestClient(t)
	require.NoError(t, mr.Server().Register(testCommands.Info, func(c *server.Peer, _ string, _ []string) {
		c.WriteError("ERR not found: key does not exist")
	}))

	core := redisfilter.New(client, "t:f", testCommands)

	result, found, err := core.Info(t.Context())
	require.NoError(t, err, "a missing filter is not an error")
	require.False(t, found)
	require.Nil(t, result)
}

func TestCore_Info_ErrorWrapped(t *testing.T) {
	t.Parallel()

	_, client := newTestClient(t)
	core := redisfilter.New(client, "t:f", testCommands)

	_, _, err := core.Info(t.Context())
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to get Redis Test filter info")
}

func TestInfoFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result any
		want   map[string]any
	}{
		{
			name:   "pairs",
			result: []any{"Capacity", int64(10), "Size", int64(20)},
			want:   map[string]any{"Capacity": int64(10), "Size": int64(20)},
		},
		{
			name:   "odd length drops trailing key",
			result: []any{"Capacity", int64(10), "Dangling"},
			want:   map[string]any{"Capacity": int64(10)},
		},
		{
			name:   "non-string keys skipped",
			result: []any{int64(1), int64(2), "Size", int64(20)},
			want:   map[string]any{"Size": int64(20)},
		},
		{
			name:   "non-slice reply",
			result: "OK",
			want:   map[string]any{},
		},
		{
			name:   "nil reply",
			result: nil,
			want:   map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := map[string]any{}
			for key, val := range redisfilter.InfoFields(tc.result) {
				got[key] = val
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestInfoFields_EarlyStop(t *testing.T) {
	t.Parallel()

	var seen []string
	for key := range redisfilter.InfoFields([]any{"A", 1, "B", 2, "C", 3}) {
		seen = append(seen, key)
		if len(seen) == 2 {
			break
		}
	}
	require.Equal(t, []string{"A", "B"}, seen)
}

func TestToInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    int64
		wantErr bool
	}{
		{name: "int64", input: int64(42), want: 42},
		{name: "int", input: 7, want: 7},
		{name: "numeric string", input: "1234", want: 1234},
		{name: "bad string", input: "not-a-number", wantErr: true},
		{name: "unsupported type", input: 3.14, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := redisfilter.ToInt64(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
