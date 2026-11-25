// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// Serial: the collector is package-level state shared by every test.
func TestMetricsCollection_RestartsAfterContextCancellation(t *testing.T) {
	StopMetricsCollection()
	t.Cleanup(StopMetricsCollection)

	ctx, cancel := context.WithCancel(t.Context())
	StartMetricsCollectionWithContext(ctx)
	require.True(t, sampler.running.Load())

	cancel()
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return !sampler.running.Load() },
		"collection still marked running after its context was canceled")

	StartMetricsCollectionWithContext(t.Context())
	sampler.mu.Lock()
	restarted := sampler.running.Load() && sampler.stopChan != nil
	sampler.mu.Unlock()
	require.True(t, restarted, "collection did not start again after its context was canceled")
}

func TestMetricsCollection_StartStopRestart(t *testing.T) {
	StopMetricsCollection() // idempotent

	StartMetricsCollection()
	sampler.mu.Lock()
	firstStop := sampler.stopChan
	firstRunning := sampler.running.Load()
	sampler.mu.Unlock()

	require.True(t, firstRunning, "running=false, want true after StartMetricsCollection")
	require.NotNil(t, firstStop, "stopChan=nil, want non-nil after StartMetricsCollection")

	StopMetricsCollection()
	sampler.mu.Lock()
	afterStopChan := sampler.stopChan
	afterStopRunning := sampler.running.Load()
	sampler.mu.Unlock()

	require.False(t, afterStopRunning, "running=true, want false after StopMetricsCollection")
	require.Nil(t, afterStopChan, "stopChan should be nil after StopMetricsCollection")

	StartMetricsCollection()
	sampler.mu.Lock()
	secondStop := sampler.stopChan
	secondRunning := sampler.running.Load()
	sampler.mu.Unlock()

	require.True(t, secondRunning, "running=false, want true after restart StartMetricsCollection")
	require.NotNil(t, secondStop, "stopChan=nil, want non-nil after restart StartMetricsCollection")
	require.False(t, firstStop == secondStop, "stopChan reused across restart, want a fresh channel")

	StopMetricsCollection()
}
