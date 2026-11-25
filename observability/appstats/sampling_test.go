// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSamplingLoopConcurrentStartAndRestart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		starts := 0
		var samples atomic.Int32
		loop := samplingLoop{
			initialize: func() { starts++ },
			sample: func(ctx context.Context) {
				samples.Add(1)
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) != DefaultCacheInterval {
					t.Errorf("unexpected sampling deadline %v", deadline)
				}
			},
		}
		defer loop.stop()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var wg sync.WaitGroup
		wg.Go(func() { loop.start(ctx) })
		loop.start(ctx)
		wg.Wait()
		synctest.Wait()
		require.Equal(t, 1, starts)
		require.Zero(t, samples.Load())
		time.Sleep(DefaultCacheInterval)
		synctest.Wait()
		require.EqualValues(t, 1, samples.Load())

		loop.stop()
		loop.start(t.Context())
		cancel()
		synctest.Wait()
		require.Equal(t, 2, starts)
		require.True(t, loop.running.Load())
		time.Sleep(DefaultCacheInterval)
		synctest.Wait()
		require.EqualValues(t, 2, samples.Load())

		loop.stop()
		synctest.Wait()
		time.Sleep(DefaultCacheInterval)
		require.EqualValues(t, 2, samples.Load())
	})
}
