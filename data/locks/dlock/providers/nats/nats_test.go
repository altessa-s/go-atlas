// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

func setupLocker(tb testing.TB) *locknats.Locker {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	nc := testhelpers.ConnectNATS(tb, ns)
	ctx := tb.Context()

	bucket := strings.ReplaceAll(tb.Name(), "/", "-")
	locker, err := locknats.New(ctx, nc, locknats.WithBucket(bucket))
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = locker.Close(context.Background()) })
	return locker
}

func TestNew(t *testing.T) {
	locker := setupLocker(t)
	require.NotNil(t, locker, "New() returned nil")
}

func TestLocker_Lock_Release(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	lk, err := locker.Lock(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, lk, "Lock() returned nil")

	info, err := lk.GetLockInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, "test-key", info.Key)

	require.NoError(t, lk.Release(ctx))
}

func TestLocker_GetLockInfo_NotHeld(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	_, err := locker.GetLockInfo(ctx, "nonexistent")
	require.Error(t, err, "GetLockInfo() on nonexistent key should return error")
	require.True(t, isErrLockNotHeld(err), "GetLockInfo() error = %v, want ErrLockNotHeld", err)
}

func TestLocker_GetLockInfo_Held(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	lk, err := locker.Lock(ctx, "info-key")
	require.NoError(t, err)
	defer lk.Release(ctx) //nolint:errcheck

	info, err := locker.GetLockInfo(ctx, "info-key")
	require.NoError(t, err)
	require.Equal(t, "info-key", info.Key)
	require.NotEmpty(t, info.Owner, "Owner should not be empty")
	require.False(t, info.AcquiredAt.IsZero(), "AcquiredAt should not be zero")
	require.NotEqual(t, uint64(0), info.FencingToken, "FencingToken should be > 0 for a held lock")
}

func TestLocker_Close_PreventsNewLocks(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	require.NoError(t, locker.Close(ctx))

	_, err := locker.Lock(ctx, "key")
	require.Error(t, err, "Lock() after Close() should return error")
}

func TestLocker_Close_Double(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	_ = locker.Close(ctx)
	err := locker.Close(ctx)
	require.Error(t, err, "second Close() should return error")
}

func TestLocker_Lock_CanceledContext(t *testing.T) {
	locker := setupLocker(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := locker.Lock(ctx, "key")
	require.Error(t, err, "Lock() with canceled context should return error")
}

func TestLocker_Lock_DifferentKeys(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	lk1, err := locker.Lock(ctx, "key-1")
	require.NoError(t, err)
	defer lk1.Release(ctx) //nolint:errcheck

	lk2, err := locker.Lock(ctx, "key-2")
	require.NoError(t, err)
	defer lk2.Release(ctx) //nolint:errcheck
}

func TestLocker_Close_ReleasesActiveLocks(t *testing.T) {
	locker := setupLocker(t)
	ctx := t.Context()

	_, err := locker.Lock(ctx, "active-key")
	require.NoError(t, err)

	// Close should release active locks without error
	require.NoError(t, locker.Close(ctx))
}

func TestLocker_Lock_WithTimeout(t *testing.T) {
	locker := setupLocker(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	lk, err := locker.Lock(ctx, "timeout-key")
	require.NoError(t, err)
	_ = lk.Release(ctx)
}

func TestLocker_Probe_OK(t *testing.T) {
	locker := setupLocker(t)
	require.NoError(t, locker.Probe(t.Context()))
}

func TestLocker_Probe_AfterClose(t *testing.T) {
	locker := setupLocker(t)
	require.NoError(t, locker.Close(t.Context()))
	require.Error(t, locker.Probe(t.Context()), "Probe() after Close() should return error")
}

func isErrLockNotHeld(err error) bool {
	return err != nil && err.Error() == errs.ErrLockNotHeld.Error()
}
