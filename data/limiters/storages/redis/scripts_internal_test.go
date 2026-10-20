// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// TestLuaScript_MillisecondWindow drives the sliding-window script with
// controlled timestamps, so the window boundaries are checked without
// depending on wall-clock scheduling.
func TestLuaScript_MillisecondWindow(t *testing.T) {
	t.Parallel()

	// reset is floor((oldest request in the window + window) / 1000); an
	// admission into an empty window is that oldest request itself.
	type call struct {
		now     int64
		allowed bool
		reset   int64
	}
	tests := []struct {
		name     string
		windowMs int64
		calls    []call
	}{
		{
			name:     "half second",
			windowMs: 500,
			calls:    []call{{1000, true, 1}, {1400, false, 1}, {1501, true, 2}},
		},
		{
			name:     "one and a half seconds",
			windowMs: 1500,
			calls:    []call{{1000, true, 2}, {2100, false, 2}, {2501, true, 4}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, mr := testhelpers.RedisClient(t)
			const key = "window"

			for i, c := range tc.calls {
				res, err := luaScript.Run(t.Context(), client, []string{key},
					tc.windowMs, 1, c.now, "m"+strconv.Itoa(i)).Slice()
				require.NoError(t, err)
				require.Equal(t, c.allowed, toInt64(res[3]) == 1, "call at now=%d", c.now)
				require.Equal(t, c.reset, toInt64(res[2]), "reset of the call at now=%d, in Unix seconds", c.now)
			}
			require.Equal(t, time.Duration(tc.windowMs)*time.Millisecond, mr.TTL(key))
		})
	}
}

// TestLuaScriptMulti_AllOrNothing drives the multi-key script with
// controlled timestamps over two windows of different lengths: a refusal by
// one key adds nothing to the other, and each window prunes on its own
// period.
func TestLuaScriptMulti_AllOrNothing(t *testing.T) {
	t.Parallel()
	client, mr := testhelpers.RedisClient(t)
	keys := []string{"short", "long"}

	// short: 300 ms, limit 1; long: 1500 ms, limit 3.
	calls := []struct {
		now       int64
		refused   int64 // 1-based index, 0 when admitted
		remaining [2]int64
		reset     [2]int64
	}{
		{now: 1000, refused: 0, remaining: [2]int64{0, 2}, reset: [2]int64{1, 2}},
		{now: 1200, refused: 1, remaining: [2]int64{0, 2}, reset: [2]int64{1, 2}},
		{now: 1250, refused: 1, remaining: [2]int64{0, 2}, reset: [2]int64{1, 2}},
		{now: 1301, refused: 0, remaining: [2]int64{0, 1}, reset: [2]int64{1, 2}},
		{now: 1700, refused: 0, remaining: [2]int64{0, 0}, reset: [2]int64{2, 2}},
		{now: 2001, refused: 2, remaining: [2]int64{1, 0}, reset: [2]int64{0, 2}},
	}
	for i, c := range calls {
		res, err := luaScriptMulti.Run(t.Context(), client, keys,
			c.now, 300, 1, "s"+strconv.Itoa(i), 1500, 3, "l"+strconv.Itoa(i)).Slice()
		require.NoError(t, err)
		require.Len(t, res, 5)
		require.Equal(t, c.refused, toInt64(res[0]), "refused index at now=%d", c.now)
		require.Equal(t, c.remaining, [2]int64{toInt64(res[1]), toInt64(res[3])}, "remaining at now=%d", c.now)
		require.Equal(t, c.reset, [2]int64{toInt64(res[2]), toInt64(res[4])}, "reset at now=%d", c.now)
	}
	members, err := client.ZRange(t.Context(), "long", 0, -1).Result()
	require.NoError(t, err)
	require.Equal(t, []string{"l0", "l3", "l4"}, members, "refused calls add no member")
	require.Equal(t, 1500*time.Millisecond, mr.TTL("long"))
}
