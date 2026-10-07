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

	type call struct {
		now     int64
		allowed bool
	}
	tests := []struct {
		name      string
		windowMs  int64
		calls     []call
		wantReset int64
	}{
		{
			name:      "half second",
			windowMs:  500,
			calls:     []call{{1000, true}, {1400, false}, {1501, true}},
			wantReset: 1,
		},
		{
			name:      "one and a half seconds",
			windowMs:  1500,
			calls:     []call{{1000, true}, {2100, false}, {2501, true}},
			wantReset: 2,
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
				if i == 1 {
					require.Equal(t, tc.wantReset, toInt64(res[2]), "reset is in Unix seconds")
				}
			}
			require.Equal(t, time.Duration(tc.windowMs)*time.Millisecond, mr.TTL(key))
		})
	}
}
