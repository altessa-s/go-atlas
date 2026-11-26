// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

type blockedCreateKV struct {
	jetstream.KeyValue
	entered chan struct{}
	resume  chan struct{}
}

func (kv blockedCreateKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if key == "blocked" {
		close(kv.entered)
		<-kv.resume
	}
	return kv.KeyValue.Create(ctx, key, value, opts...)
}

func TestClose_StopsRenewalBeforeWaitingForAcquisition(t *testing.T) {
	t.Parallel()
	const ttl = time.Second
	ctx := t.Context()
	ns := natstest.StartNATSServer(t)
	locker, err := New(ctx, natstest.ConnectNATS(t, ns), WithBucket("close-timeout"), WithTTL(ttl))
	require.NoError(t, err)

	kv := blockedCreateKV{KeyValue: locker.kv, entered: make(chan struct{}), resume: make(chan struct{})}
	locker.kvOps = natskvlease.NewKVOps(kv, nil)
	held, err := locker.Lock(ctx, "held")
	require.NoError(t, err)
	handle, ok := held.(*lock)
	require.True(t, ok)
	done := make(chan error, 1)
	go func() {
		_, err := locker.Lock(ctx, "blocked")
		done <- err
	}()
	defer func() {
		close(kv.resume)
		require.Error(t, <-done)
		require.NoError(t, locker.Close(ctx))
	}()
	select {
	case <-kv.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("acquisition never reached Create")
	}
	closeCtx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	require.ErrorIs(t, locker.Close(closeCtx), context.DeadlineExceeded)
	require.False(t, handle.IsLocked(), "Close stopped renewal before the wait timed out")
	require.Equal(t, 1, locker.TrackedLocks(), "a later Close must still be able to release the lease")
	_, err = locker.Lock(ctx, "after-close")
	require.ErrorIs(t, err, errClosed)

	successor, err := New(ctx, natstest.ConnectNATS(t, ns), WithBucket("close-timeout"), WithTTL(ttl))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, successor.Close(context.WithoutCancel(ctx))) })
	require.Eventually(t, func() bool {
		_, err := successor.Lock(ctx, "held")
		return err == nil
	}, 2*ttl, 20*time.Millisecond, "the held lock kept renewing after Close")
	require.False(t, handle.IsLocked(), "the original handle stays invalid after takeover")
}

type unconfirmedUpdateKV struct {
	jetstream.KeyValue
	fail atomic.Bool
}

func (kv *unconfirmedUpdateKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if kv.fail.Load() {
		// Apply the renewal without acknowledging it. The refreshed key
		// remains live after the original renewal window has closed.
		if _, err := kv.KeyValue.Update(ctx, key, value, revision); err != nil {
			return 0, err
		}
		return 0, errors.New("renewal reply lost")
	}
	return kv.KeyValue.Update(ctx, key, value, revision)
}

func TestLock_CancelAfterAmbiguousRenewalLossReleasesAndUntracks(t *testing.T) {
	t.Parallel()
	const ttl = 3 * time.Second
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ns := natstest.StartNATSServer(t)
	locker, err := New(t.Context(), natstest.ConnectNATS(t, ns), WithBucket("cancel-after-loss"), WithTTL(ttl))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, locker.Close(context.WithoutCancel(t.Context()))) })
	kv := &unconfirmedUpdateKV{KeyValue: locker.kv}
	kv.fail.Store(true)
	locker.kvOps = natskvlease.NewKVOps(kv, nil)

	held, err := locker.Lock(ctx, "held")
	require.NoError(t, err)
	handle, ok := held.(*lock)
	require.True(t, ok)
	require.Eventually(t, func() bool { return !handle.IsLocked() }, 2*ttl, time.Millisecond)
	require.Equal(t, 1, locker.TrackedLocks())
	kv.fail.Store(false)
	_, err = locker.GetLockInfo(t.Context(), "held")
	require.NoError(t, err, "the unacknowledged renewal left the key live")
	cancel()
	require.Eventually(t, func() bool { return locker.TrackedLocks() == 0 }, 500*time.Millisecond, time.Millisecond)
	_, err = locker.GetLockInfo(t.Context(), "held")
	require.ErrorIs(t, err, providers.ErrLockNotHeld)
	reacquired, err := locker.Lock(t.Context(), "held")
	require.NoError(t, err)
	require.NoError(t, reacquired.Release(t.Context()))
}

func TestClose_ReturnsReleaseFailureAndKeepsLockTracked(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
	locker, err := New(ctx, nc, WithBucket("close-failure"))
	require.NoError(t, err)
	_, err = locker.Lock(ctx, "held")
	require.NoError(t, err)
	ns.Shutdown()
	nc.Close()

	require.Error(t, locker.Close(ctx))
	require.Equal(t, 1, locker.TrackedLocks())
}

type failingDeleteKV struct {
	jetstream.KeyValue
	err error
}

func (kv *failingDeleteKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if kv.err != nil {
		return kv.err
	}
	return kv.KeyValue.Delete(ctx, key, opts...)
}

func TestClose_RetriesFailedRelease(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ns := natstest.StartNATSServer(t)
	locker, err := New(ctx, natstest.ConnectNATS(t, ns), WithBucket("close-retry"))
	require.NoError(t, err)

	deleteErr := errors.New("delete failed")
	kv := &failingDeleteKV{KeyValue: locker.kv, err: deleteErr}
	locker.kvOps = natskvlease.NewKVOps(kv, nil)
	_, err = locker.Lock(ctx, "held")
	require.NoError(t, err)
	require.ErrorIs(t, locker.Close(ctx), deleteErr)
	require.Equal(t, 1, locker.TrackedLocks())

	kv.err = nil
	require.NoError(t, locker.Close(ctx))
	require.Zero(t, locker.TrackedLocks())
	_, err = locker.GetLockInfo(ctx, "held")
	require.ErrorIs(t, err, providers.ErrLockNotHeld)
}
