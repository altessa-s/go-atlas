// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
)

type heldStateProvider struct {
	mockProvider
	entered, resume chan struct{}
	hold            atomic.Bool
}

func (p *heldStateProvider) IsRunning() bool {
	if p.hold.CompareAndSwap(true, false) {
		close(p.entered)
		<-p.resume
	}
	return p.mockProvider.IsRunning()
}

func TestCallbackStopDuringIdempotentStart(t *testing.T) {
	t.Parallel()
	p := &heldStateProvider{entered: make(chan struct{}), resume: make(chan struct{})}
	callback, stopped := make(chan struct{}), make(chan error, 1)
	resigned := false
	p.startFn = func(_ context.Context, cfg providers.Config) error {
		p.isLeader = true
		cfg.BecameCh <- struct{}{}
		return nil
	}
	p.stopFn = func(context.Context) error { p.isLeader = false; resigned = true; return nil }
	le := leadelect.New(p, "election", "node")
	t.Cleanup(func() { require.NoError(t, le.Stop(t.Context())) })
	le.RegisterOnBecomesLeader(func(ctx context.Context, _ leadelect.LeaderElector) {
		<-callback
		stopped <- le.Stop(ctx)
	})
	require.NoError(t, le.Start(t.Context()))
	p.hold.Store(true)
	started := make(chan error, 1)
	go func() { started <- le.Start(t.Context()) }()
	<-p.entered
	close(callback)
	require.NoError(t, <-stopped)
	close(p.resume)
	require.NoError(t, <-started)
	require.False(t, le.IsRunning())
	require.False(t, le.IsLeader())
	require.True(t, resigned)
	require.NoError(t, le.Stop(t.Context()))
	p.startFn = nil
	require.NoError(t, le.Start(t.Context()))
	require.True(t, le.IsRunning())
}
