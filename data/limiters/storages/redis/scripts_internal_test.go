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
