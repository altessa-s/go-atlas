// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dlock_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock"
	"github.com/altessa-s/go-atlas/coord/dlock/providers"
)

// lostLock is a lock that reports the loss of its lease and records how it
// was released.
type lostLock struct {
	lost       chan struct{}
	releaseErr error

	releases      int
	releaseCtxErr error
	releaseBound  time.Duration
}

func (l *lostLock) GetLockInfo(context.Context) (*providers.LockInfo, error) {
	return &providers.LockInfo{}, nil
}

func (l *lostLock) Release(ctx context.Context) error {
	l.releases++
	l.releaseCtxErr = ctx.Err()
	if deadline, ok := ctx.Deadline(); ok {
		l.releaseBound = time.Until(deadline)
	}
	return l.releaseErr
}

func (l *lostLock) Lost() <-chan struct{} { return l.lost }

// lostProvider hands out one lostLock.
type lostProvider struct{ lock *lostLock }

func (p lostProvider) Lock(context.Context, string) (providers.Lock, error) { return p.lock, nil }

func (p lostProvider) GetLockInfo(context.Context, string) (*providers.LockInfo, error) {
	return &providers.LockInfo{}, nil
}

func (p lostProvider) Close(context.Context) error { return nil }

func TestSynchronize_CancelsFnWhenTheLockIsLost(t *testing.T) {
	t.Parallel()
	lk := &lostLock{lost: make(chan struct{})}
	dl := dlock.New(lostProvider{lock: lk})

	var cause error
	err := dl.Synchronize(t.Context(), "key", func(ctx context.Context) error {
		close(lk.lost)
		select {
		case <-ctx.Done():
			cause = context.Cause(ctx)
		case <-time.After(5 * time.Second):
		}
		return ctx.Err()
	})

	require.ErrorIs(t, cause, dlock.ErrLockLost)
	require.ErrorIs(t, err, dlock.ErrLockLost)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, lk.releases)
}

func TestSynchronize_KeepsFnErrorWhileTheLockIsHeld(t *testing.T) {
	t.Parallel()
	lk := &lostLock{lost: make(chan struct{})}
	dl := dlock.New(lostProvider{lock: lk})

	errFn := errors.New("fn failed")
	err := dl.Synchronize(t.Context(), "key", func(ctx context.Context) error {
		require.NoError(t, ctx.Err())
		return errFn
	})

	require.ErrorIs(t, err, errFn)
	require.NotErrorIs(t, err, dlock.ErrLockLost)
}

func TestSynchronize_ReleasesUnderABoundedDetachedContext(t *testing.T) {
	t.Parallel()
	lk := &lostLock{lost: make(chan struct{})}
	dl := dlock.New(lostProvider{lock: lk}, dlock.WithReleaseTimeout(time.Minute))

	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, dl.Synchronize(ctx, "key", func(context.Context) error {
		cancel() // the caller gives up while fn runs
		return nil
	}))

	require.Equal(t, 1, lk.releases)
	require.NoError(t, lk.releaseCtxErr, "the release must not inherit the caller's cancellation")
	require.Positive(t, lk.releaseBound)
	require.LessOrEqual(t, lk.releaseBound, time.Minute)
}

func TestSynchronize_LogsAFailedRelease(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	lk := &lostLock{lost: make(chan struct{}), releaseErr: errors.New("storage down")}
	dl := dlock.New(lostProvider{lock: lk}, dlock.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))

	require.ErrorIs(t, dl.Synchronize(t.Context(), "key", func(context.Context) error { return nil }), lk.releaseErr)

	require.Equal(t, 1, lk.releases)
	require.Contains(t, logs.String(), "failed to release lock")
}

func TestSynchronize_PanicReturnsErrorAndReleasesOnce(t *testing.T) {
	t.Parallel()
	lk := &lostLock{releaseErr: errors.New("release failed")}
	dl := dlock.New(lostProvider{lock: lk})

	err := dl.Synchronize(t.Context(), "key", func(context.Context) error { panic("callback panic") })
	require.ErrorIs(t, err, dlock.ErrPanicked)
	require.ErrorIs(t, err, lk.releaseErr)
	require.Equal(t, 1, lk.releases)
}

func TestSynchronize_JoinsReleaseErrorWithLossAndCallbackError(t *testing.T) {
	t.Parallel()
	lk := &lostLock{lost: make(chan struct{}), releaseErr: errors.New("release failed")}
	dl := dlock.New(lostProvider{lock: lk})
	fnErr := errors.New("callback failed")

	err := dl.Synchronize(t.Context(), "key", func(context.Context) error {
		close(lk.lost)
		return fnErr
	})
	require.ErrorIs(t, err, dlock.ErrLockLost)
	require.ErrorIs(t, err, fnErr)
	require.ErrorIs(t, err, lk.releaseErr)
}
