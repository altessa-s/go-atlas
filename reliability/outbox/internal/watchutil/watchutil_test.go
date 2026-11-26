// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package watchutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox/internal/watchutil"
)

func TestSleep(t *testing.T) {
	t.Parallel()

	t.Run("positive duration completes", func(t *testing.T) {
		t.Parallel()
		require.True(t, watchutil.Sleep(t.Context(), time.Millisecond))
	})

	t.Run("canceled context stops immediately", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.False(t, watchutil.Sleep(ctx, time.Hour))
	})

	t.Run("non-positive duration honors context", func(t *testing.T) {
		t.Parallel()
		for _, d := range []time.Duration{0, -time.Millisecond, -time.Second} {
			require.True(t, watchutil.Sleep(t.Context(), d))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.False(t, watchutil.Sleep(ctx, d))
		}
	})
}

func TestReconnectBackoff(t *testing.T) {
	t.Parallel()

	next := watchutil.ReconnectBackoff()
	first := next(0, nil)
	require.Greater(t, first, time.Duration(0))
	require.LessOrEqual(t, first, watchutil.ReconnectMaxDelay)

	capped := next(100, nil)
	require.LessOrEqual(t, capped, time.Duration(float64(watchutil.ReconnectMaxDelay)*(1+watchutil.ReconnectJitter)))
}

func TestShouldLogFailure(t *testing.T) {
	t.Parallel()

	require.False(t, watchutil.ShouldLogFailure(0))
	require.True(t, watchutil.ShouldLogFailure(1))
	require.False(t, watchutil.ShouldLogFailure(2))
	require.True(t, watchutil.ShouldLogFailure(watchutil.LogEvery))
	require.False(t, watchutil.ShouldLogFailure(watchutil.LogEvery+1))
	require.True(t, watchutil.ShouldLogFailure(2*watchutil.LogEvery))
}
