// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
)

func TestLeader_StopFromCallbackDuringStart(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		releaseStart := make(chan struct{})
		started := make(chan error, 1)
		stopped := make(chan error, 1)
		resigned := false
		prov := &mockProvider{}
		prov.startFn = func(_ context.Context, cfg providers.Config) error {
			prov.isLeader = true
			cfg.BecameCh <- struct{}{}
			<-releaseStart
			return nil
		}
		prov.stopFn = func(context.Context) error {
			prov.isLeader = false
			resigned = true
			return nil
		}
		le := leadelect.New(prov, "election", "node-1")
		t.Cleanup(func() { require.NoError(t, le.Stop(t.Context())) })
		le.RegisterOnBecomesLeader(func(ctx context.Context, _ leadelect.LeaderElector) {
			stopped <- le.Stop(ctx)
		})
		go func() { started <- le.Start(t.Context()) }()
		synctest.Wait()
		close(releaseStart)
		require.NoError(t, <-started)
		require.NoError(t, <-stopped)
		require.False(t, le.IsRunning())
		require.False(t, le.IsLeader())
		require.True(t, resigned)
		require.NoError(t, le.Stop(t.Context()))

		prov.startFn = nil
		require.NoError(t, le.Start(t.Context()))
		require.True(t, le.IsRunning())
		require.NoError(t, le.Stop(t.Context()))
	})
}
