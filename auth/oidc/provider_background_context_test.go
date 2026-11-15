// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

type backgroundCtxKey struct{}

// ctxRecordingStorage counts the scheduled syncs that ran under a live
// context carrying the constructor context's value. While hold is set, a
// scheduled sync announces itself on entered and stays in Sync, whatever its
// context does, until release is closed.
type ctxRecordingStorage struct {
	syncStorage
	live atomic.Int32

	hold     atomic.Bool
	entered  chan struct{}
	release  chan struct{}
	returned atomic.Int32
}

func (s *ctxRecordingStorage) Sync(ctx context.Context) error {
	if s.syncs.Load() > 0 && ctx.Err() == nil && ctx.Value(backgroundCtxKey{}) == "kept" {
		s.live.Add(1)
	}
	err := s.syncStorage.Sync(ctx)
	if s.hold.CompareAndSwap(true, false) {
		s.entered <- struct{}{}
		<-s.release
		s.returned.Add(1)
	}
	return err
}

// tickSchedule fires its job every tick instead of on wall-clock cron
// periods (robfig/cron's shortest interval is one second).
type tickSchedule struct{}

const tick = 2 * time.Millisecond

func (tickSchedule) Next(now time.Time) time.Time { return now.Add(tick) }

// A startup-timeout context must not silence the background work once it is
// canceled: the refresh cron keeps running until Close, which stops it and
// waits for a job that is still running.
func TestNewProvider_BackgroundWorkOutlivesConstructorContext(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	storage := &ctxRecordingStorage{entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), backgroundCtxKey{}, "kept"))
	// The configured schedules never tick during the test; the jobs they
	// configure are driven through the provider's own cron below.
	p, err := idp.tryNewProviderWithContext(ctx, t, WithJWKSRefreshSchedule("@every 24h"),
		WithRevocationStorage(storage), WithRevocationSyncSchedule("@every 24h"))
	require.NoError(t, err)
	cancel()

	jobs := p.refreshJobs()
	require.Len(t, jobs, 2)
	jwks, live := idp.jwksRequests.Load(), storage.live.Load()
	for _, job := range jobs {
		p.scheduleRefreshJob(p.refreshCron, tickSchedule{}, job)
	}

	// Two more of each: one could have been in flight when ctx was canceled.
	testhelpers.WaitFor(t, 10*time.Second, func() bool {
		return idp.jwksRequests.Load() >= jwks+2 && storage.live.Load() >= live+2
	}, "scheduled JWKS refresh and revocation sync must survive the constructor context")
	require.NoError(t, p.RefreshJWKS(t.Context()))

	// Close must wait for a scheduled job that is still running.
	storage.hold.Store(true)
	<-storage.entered
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		p.Close()
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while a scheduled sync was still running")
	case <-time.After(25 * tick):
	}
	close(storage.release)
	<-closed
	require.EqualValues(t, 1, storage.returned.Load(), "Close must return only after the running sync did")

	require.ErrorIs(t, p.RefreshJWKS(t.Context()), context.Canceled)
	jwks, syncs := idp.jwksRequests.Load(), storage.syncs.Load()
	for _, job := range jobs {
		p.runRefreshJob(job)
	}
	time.Sleep(25 * tick) // the schedule would have fired many times over
	require.Equal(t, jwks, idp.jwksRequests.Load(), "a closed provider must not refresh")
	require.Equal(t, syncs, storage.syncs.Load(), "a closed provider must not sync")
}

// The constructor context still bounds the work NewProvider does itself.
func TestNewProvider_ConstructorContextBoundsInitialFetch(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := idp.tryNewProviderWithContext(ctx, t)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, idp.jwksRequests.Load())
}
