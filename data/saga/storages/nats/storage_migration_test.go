// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
)

// TestMigrateBucketStorage moves a memory bucket to file storage: instances
// survive with their data, and their versions — Execution.Fence — keep growing.
func TestMigrateBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()
	opts := []natsstore.Option{natsstore.WithBucket("sagas"), natsstore.WithBucketTTL(time.Hour)}

	testhelpers.CreateNATSKV(t, js, "sagas", time.Hour) // memory storage
	store, err := natsstore.New(js, opts...)
	require.NoError(t, err)
	inst := &saga.Instance{ID: "o1", Definition: "d", Status: saga.StatusRunning, Data: []byte(`{"n":1}`)}
	require.NoError(t, store.Create(ctx, inst))
	for range 3 {
		require.NoError(t, store.Update(ctx, inst))
	}
	oldVersion := inst.Version

	require.NoError(t, natsstore.MigrateBucketStorage(ctx, js, natsstore.MigrationOptions{}, opts...))
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "sagas"))

	migrated, err := natsstore.New(js, append(opts, natsstore.WithStrictBucketStorage())...)
	require.NoError(t, err)
	got, err := migrated.Get(ctx, "o1")
	require.NoError(t, err)
	require.Equal(t, saga.StatusRunning, got.Status)
	require.JSONEq(t, `{"n":1}`, string(got.Data))
	require.Greater(t, got.Version, oldVersion, "the instance version must stay monotonic across the migration")
	require.NoError(t, migrated.Update(ctx, got), "the migrated instance must be updatable with its new version")
}
