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

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/internal/nats/natskey/natskeytest"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	locknats "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
)

// TestLocker_KeysNATSRejectsVerbatim locks, inspects and releases keys the
// server rejects as they are, each guarded by a lock of its own.
func TestLocker_KeysNATSRejectsVerbatim(t *testing.T) {
	t.Parallel()
	locker := setupLocker(t)
	ctx := t.Context()
	keys := natskeytest.Keys()

	for _, key := range keys {
		lk, err := locker.Lock(ctx, key)
		require.NoError(t, err, key)
		t.Cleanup(func() { _ = lk.Release(context.Background()) })
	}
	for _, key := range keys {
		_, err := locker.Lock(ctx, key)
		require.ErrorIs(t, err, providers.ErrLockNotHeld, key)

		info, err := locker.GetLockInfo(ctx, key)
		require.NoError(t, err, key)
		require.Equal(t, key, info.Key)
	}
}

// TestLocker_KeyNATSRejectsVerbatimRenewsAndReleases holds a lock past its TTL,
// which only renewal keeps alive, then releases it so it can be taken again.
func TestLocker_KeyNATSRejectsVerbatimRenewsAndReleases(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
	ctx := t.Context()
	locker, err := locknats.New(ctx, nc, locknats.WithBucket("renew"), locknats.WithTTL(time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = locker.Close(context.Background()) })
	key := "lock:orders/1.ü"

	lk, err := locker.Lock(ctx, key)
	require.NoError(t, err)
	before, err := lk.GetLockInfo(ctx)
	require.NoError(t, err)

	time.Sleep(2500 * time.Millisecond) // more than two TTLs
	after, err := lk.GetLockInfo(ctx)
	require.NoError(t, err)
	require.Greater(t, after.FencingToken, before.FencingToken, "renewal rewrites the key")

	require.NoError(t, lk.Release(ctx))
	_, err = locker.GetLockInfo(ctx, key)
	require.ErrorIs(t, err, providers.ErrLockNotHeld)
	again, err := locker.Lock(ctx, key)
	require.NoError(t, err)
	require.NoError(t, again.Release(ctx))
}

func TestLocker_ValidKeyIsStoredUnderItself(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()
	locker, err := locknats.New(ctx, nc, locknats.WithBucket("plain"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = locker.Close(context.Background()) })

	lk, err := locker.Lock(ctx, natskeytest.PlainKey)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lk.Release(context.Background()) })

	kv, err := js.KeyValue(ctx, "plain")
	require.NoError(t, err)
	_, err = kv.Get(ctx, natskeytest.PlainKey)
	require.NoError(t, err)
}

func TestLocker_KeyTooLong(t *testing.T) {
	t.Parallel()
	locker := setupLocker(t)
	key := strings.Repeat(":", 2000)

	_, err := locker.Lock(t.Context(), key)
	require.ErrorIs(t, err, locknats.ErrKeyTooLong)
	_, err = locker.GetLockInfo(t.Context(), key)
	require.ErrorIs(t, err, locknats.ErrKeyTooLong)
}
