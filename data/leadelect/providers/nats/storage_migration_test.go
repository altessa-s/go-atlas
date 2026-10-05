// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// TestMigrateBucketStorage moves the election bucket from memory to file
// storage with WithStorage and checks that Fence keeps growing.
func TestMigrateBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()

	lead := func(opts ...lenats.Option) uint64 {
		t.Helper()
		prov, err := lenats.New(ctx, nc, opts...)
		require.NoError(t, err)
		require.NoError(t, prov.Start(ctx, providers.Config{
			Key:      "election",
			TTL:      2 * time.Second,
			NodeId:   "node-1",
			LostCh:   make(chan struct{}, 1),
			BecameCh: make(chan struct{}, 1),
			StopCh:   make(chan struct{}, 1),
		}))
		require.Eventually(t, prov.IsLeader, 3*time.Second, 20*time.Millisecond)
		fence := prov.Fence()
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, prov.Stop(stopCtx))
		return fence
	}

	oldFence := lead(lenats.WithBucket("elect"))
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "elect"))

	opts := []lenats.Option{lenats.WithBucket("elect"), lenats.WithStorage(jetstream.FileStorage)}
	require.NoError(t, lenats.MigrateBucketStorage(ctx, nc, lenats.MigrationOptions{}, opts...))
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "elect"))

	newFence := lead(append(opts, lenats.WithStrictBucketStorage())...)
	require.Greater(t, newFence, oldFence, "Fence must stay monotonic across the migration")
}
