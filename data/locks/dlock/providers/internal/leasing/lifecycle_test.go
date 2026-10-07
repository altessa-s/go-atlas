// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers/internal/leasing"
)

// Close must not return while an acquisition that started before it can still
// leave a lease behind.
func TestClose_WaitsForInflightAcquisition(t *testing.T) {
	t.Parallel()
	e := newEngine(t, newMemStore(10*time.Second))
	entered, release := make(chan struct{}), make(chan struct{})
	e.AfterAcquire = func() {
		close(entered)
		<-release
	}

	lockErr := make(chan error, 1)
	go func() {
		_, err := e.Lock(context.Background(), "k")
		lockErr <- err
	}()
	<-entered

	closed := make(chan error, 1)
	go func() { closed <- e.Close(context.Background()) }()
	select {
	case <-closed:
		t.Fatal("Close returned while an acquisition was in flight")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-closed)
	require.ErrorIs(t, <-lockErr, leasing.ErrClosed)
	_, err := e.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "Close leaves no lease behind")
}

// An acquisition whose reply arrives after the lease's TTL has elapsed may
// already belong to another holder: Lock must reject it and release it.
func TestLock_LateAcquisitionIsRejected(t *testing.T) {
	t.Parallel()
	e := newEngine(t, newMemStore(10*time.Second))
	base := time.Now()
	var calls atomic.Int32
	e.Now = func() time.Time {
		if calls.Add(1) == 1 {
			return base
		}
		return base.Add(10 * time.Second)
	}

	_, err := e.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	_, err = e.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "the late acquisition must be released")
}

// When no renewal is confirmed within the lease on this process's clock, the
// lock releases the lease instead of abandoning it.
func TestRenew_UnconfirmedLeaseIsReleased(t *testing.T) {
	t.Parallel()
	e := newEngine(t, newMemStore(3*time.Second))
	var offset atomic.Int64
	e.Now = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }

	_, err := e.Lock(context.Background(), "k")
	require.NoError(t, err)
	offset.Store(int64(4 * time.Second))
	require.Eventually(t, func() bool {
		_, err := e.GetLockInfo(t.Context(), "k")
		return err != nil
	}, 2*time.Second, 20*time.Millisecond, "the live lease must be released, not left to expire")
}

// An acquisition that completes after Close began and cannot be released at
// once stays registered, so a later Close releases it.
func TestClose_RetriesFailedCleanupOfRacingAcquisition(t *testing.T) {
	t.Parallel()
	e := newEngine(t, newMemStore(10*time.Second))
	entered, release := make(chan struct{}), make(chan struct{})
	e.AfterAcquire = func() {
		close(entered)
		<-release
	}
	var faults atomic.Int32
	errFault := errors.New("release fault")
	e.ReleaseFault = func() error {
		if faults.Add(1) <= 2 {
			return errFault
		}
		return nil
	}

	lockErr := make(chan error, 1)
	go func() {
		_, err := e.Lock(context.Background(), "k")
		lockErr <- err
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- e.Close(context.Background()) }()
	time.Sleep(50 * time.Millisecond) // let Close mark the provider closed
	close(release)

	require.ErrorIs(t, <-lockErr, leasing.ErrClosed)
	require.ErrorIs(t, <-closed, errFault, "the retained acquisition's release failed again")
	_, err := e.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the lease is still live")

	require.NoError(t, e.Close(t.Context()), "a later Close retries the release")
	_, err = e.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}

// An acquisition applied by the store whose reply is lost is released by
// Lock; when that release fails too, Close retries it.
func TestLock_AmbiguousAcquisitionIsReleased(t *testing.T) {
	t.Parallel()
	errLost := errors.New("reply lost")

	t.Run("released_by_lock", func(t *testing.T) {
		t.Parallel()
		e := newEngine(t, newMemStore(10*time.Second))
		e.AcquireFault = func() error { return errLost }

		_, err := e.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errLost)
		_, err = e.GetLockInfo(t.Context(), "k")
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	})

	t.Run("retried_by_close", func(t *testing.T) {
		t.Parallel()
		e := newEngine(t, newMemStore(10*time.Second))
		e.AcquireFault = func() error { return errLost }
		var faults atomic.Int32
		e.ReleaseFault = func() error {
			if faults.Add(1) == 1 {
				return errLost
			}
			return nil
		}

		_, err := e.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errLost)
		_, err = e.GetLockInfo(t.Context(), "k")
		require.NoError(t, err, "the applied lease is still live")
		require.NoError(t, e.Close(t.Context()))
		_, err = e.GetLockInfo(t.Context(), "k")
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	})
}
