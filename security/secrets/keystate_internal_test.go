// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// keyStateCount returns the number of per-key state entries.
func (t *Manager[T]) keyStateCount() int {
	t.keysMu.Lock()
	defer t.keysMu.Unlock()
	return len(t.keys)
}

// TestManager_KeyStateDoesNotLeak runs many operations on distinct keys and
// checks that no per-key state outlives them.
func TestManager_KeyStateDoesNotLeak(t *testing.T) {
	t.Parallel()

	const n = 1000
	tests := []struct {
		name string
		run  func(context.Context, *testing.T, *Manager[string])
	}{
		{
			name: "forced fetch of missing keys",
			run: func(ctx context.Context, t *testing.T, mgr *Manager[string]) {
				t.Helper()
				for i := range n {
					_, err := mgr.Value(ctx, fmt.Sprintf("missing-%d", i), true)
					require.ErrorIs(t, err, ErrNotFound)
				}
			},
		},
		{
			name: "save and delete of churned keys",
			run: func(ctx context.Context, t *testing.T, mgr *Manager[string]) {
				t.Helper()
				for i := range n {
					key := fmt.Sprintf("churn-%d", i)
					require.NoError(t, mgr.Save(ctx, key, "v"))
					require.NoError(t, mgr.Delete(ctx, key))
				}
			},
		},
		{
			name: "warm cache",
			run: func(ctx context.Context, t *testing.T, mgr *Manager[string]) {
				t.Helper()
				keys := make([]string, n)
				for i := range keys {
					keys[i] = fmt.Sprintf("warm-%d", i)
				}
				require.NoError(t, mgr.WarmCache(ctx, keys))
			},
		},
		{
			name: "canceled waiters",
			run: func(ctx context.Context, t *testing.T, mgr *Manager[string]) {
				t.Helper()
				st := mgr.acquireKey("held")
				require.NoError(t, st.lock(ctx))
				for range 10 {
					waitCtx, cancel := context.WithTimeout(ctx, time.Millisecond)
					require.ErrorIs(t, mgr.Save(waitCtx, "held", "v"), context.DeadlineExceeded)
					require.ErrorIs(t, mgr.Delete(waitCtx, "held"), context.DeadlineExceeded)
					cancel()
				}
				require.Equal(t, 1, mgr.keyStateCount(), "only the holder's reference is left")
				st.unlock()
				mgr.releaseKey("held", st)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &dirtyTestProvider{values: map[string]string{}}
			mgr, err := New[string](provider, fastRetry()...)
			require.NoError(t, err)

			tc.run(t.Context(), t, mgr)
			require.Zero(t, mgr.keyStateCount(), "per-key state must not outlive the operations using it")
		})
	}
}

// TestManager_KeyStateConcurrentRelease runs concurrent fetches, Saves and
// Deletes of overlapping keys, then checks every per-key state was dropped.
// Run with -race.
func TestManager_KeyStateConcurrentRelease(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &dirtyTestProvider{values: map[string]string{}}
	mgr, err := New[string](provider, fastRetry()...)
	require.NoError(t, err)

	const workers, iterations = 8, 200
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range iterations {
				key := fmt.Sprintf("k-%d", (w+i)%5)
				switch i % 3 {
				case 0:
					_ = mgr.Save(ctx, key, "v")
				case 1:
					_, _ = mgr.Value(ctx, key, true)
				default:
					_ = mgr.Delete(ctx, key)
				}
			}
		})
	}
	wg.Wait()
	require.Zero(t, mgr.keyStateCount())
}
