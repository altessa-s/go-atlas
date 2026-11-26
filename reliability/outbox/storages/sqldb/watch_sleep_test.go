// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSleepNonPositiveDuration(t *testing.T) {
	t.Parallel()

	for _, d := range []time.Duration{0, -time.Millisecond, -time.Second} {
		require.True(t, sleep(t.Context(), d), "live ctx with d=%v must return true immediately", d)

		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		for range 64 {
			require.False(t, sleep(canceled, d), "canceled ctx with d=%v must deterministically return false", d)
		}
	}
}
