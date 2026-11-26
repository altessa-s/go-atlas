// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"runtime"
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

// Serial: mutates package-level sampler and cachedMetrics.
func TestCPULoad_ConcurrentDuringInitialize_NoPanic(t *testing.T) {
	ResetMetricsCollection()
	origCache := cachedMetrics
	origInit := sampler.initialize
	origSample := sampler.sample
	t.Cleanup(func() {
		StopMetricsCollection()
		cachedMetrics = origCache
		sampler.initialize = origInit
		sampler.sample = origSample
	})

	fresh := &CachedMetrics{}
	cachedMetrics = fresh
	entered := make(chan struct{})
	release := make(chan struct{})
	sampler.initialize = func() {
		close(entered)
		<-release
		fresh.initialize()
	}
	sampler.sample = func(context.Context) {}

	var wg sync.WaitGroup
	wg.Go(func() {
		data, err := CPULoad(t.Context())
		require.NoError(t, err)
		require.Equal(t, zeroPercent, data.Service)
	})

	<-entered
	require.False(t, sampler.running.Load(), "running must remain false until initialize completes")

	var panicked any
	var secondData *CPULoadData
	secondStarted := make(chan struct{})
	wg.Go(func() {
		defer func() { panicked = recover() }()
		close(secondStarted)
		var err error
		secondData, err = CPULoad(t.Context())
		require.NoError(t, err)
	})

	<-secondStarted
	for range 64 {
		runtime.Gosched()
	}
	close(release)
	wg.Wait()
	StopMetricsCollection()

	require.Nil(t, panicked, "concurrent CPULoad during initialization must not panic")
	require.NotNil(t, secondData)
	require.Equal(t, zeroPercent, secondData.Service)
}

// Serial: mutates package-level sampler and cachedMetrics.
func TestStartStopStart_IsCacheStaleUntilFirstSample(t *testing.T) {
	ResetMetricsCollection()
	origSample := sampler.sample
	t.Cleanup(func() {
		StopMetricsCollection()
		sampler.sample = origSample
		ResetMetricsCollection()
	})

	synctest.Test(t, func(t *testing.T) {
		sampler.sample = func(_ context.Context) {
			now := time.Now().Unix()
			cachedMetrics.ServiceCPU.Store("12.34%")
			cachedMetrics.SystemCPU.Store("23.45%")
			cachedMetrics.CPUTimestamp.Store(now)
			cachedMetrics.MemoryTimestamp.Store(now)
			cachedMetrics.NetworkTimestamp.Store(now)
		}

		StartMetricsCollectionWithContext(t.Context())
		require.True(t, IsCacheStale(), "before first sample cache is stale")

		time.Sleep(DefaultCacheInterval)
		synctest.Wait()
		require.False(t, IsCacheStale(), "after first sample cache is fresh")

		StopMetricsCollection()
		synctest.Wait()

		StartMetricsCollectionWithContext(t.Context())
		require.True(t, IsCacheStale(), "restarted collection with reset zero values must report stale until its first sample")

		time.Sleep(DefaultCacheInterval)
		synctest.Wait()
		require.False(t, IsCacheStale(), "fresh again after new sample")

		StopMetricsCollection()
		synctest.Wait()
	})
}
