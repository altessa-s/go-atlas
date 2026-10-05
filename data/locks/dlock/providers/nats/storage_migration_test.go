// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

// TestMigrateBucketStorage moves the lock bucket from memory to file storage
// with WithStorage and checks that fencing tokens keep growing.
func TestMigrateBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()

	before, err := locknats.New(ctx, nc, locknats.WithBucket("locks"))
	require.NoError(t, err)
	var oldToken uint64
	for range 3 {
		lk, err := before.Lock(ctx, "resource")
		require.NoError(t, err)
		info, err := lk.GetLockInfo(ctx)
		require.NoError(t, err)
		oldToken = info.FencingToken
		require.NoError(t, lk.Release(ctx))
	}
	require.NoError(t, before.Close(ctx))
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "locks"))

	opts := []locknats.Option{locknats.WithBucket("locks"), locknats.WithStorage(jetstream.FileStorage)}
	require.NoError(t, locknats.MigrateBucketStorage(ctx, nc, locknats.MigrationOptions{}, opts...))
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "locks"))

	after, err := locknats.New(ctx, nc, append(opts, locknats.WithStrictBucketStorage())...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = after.Close(context.Background()) })
	lk, err := after.Lock(ctx, "resource")
	require.NoError(t, err)
	info, err := lk.GetLockInfo(ctx)
	require.NoError(t, err)
	require.Greater(t, info.FencingToken, oldToken, "the fencing token must stay monotonic across the migration")
	require.NoError(t, lk.Release(ctx))
}
