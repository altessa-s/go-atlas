// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

// TestMigrateBucketStorage moves a memory bucket to file storage and checks
// that the consumed budget survives.
func TestMigrateBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()
	opts := []limitnats.Option{limitnats.WithBucket("limits")}

	testhelpers.CreateNATSKV(t, js, "limits", limitnats.DefaultMaxAge) // memory storage
	limiter, err := limitnats.New(js, opts...)
	require.NoError(t, err)
	for range 3 {
		_, err := limiter.Allow(ctx, "client", 5, time.Hour)
		require.NoError(t, err)
	}

	require.NoError(t, limitnats.MigrateBucketStorage(ctx, js, limitnats.MigrationOptions{}, opts...))
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "limits"))

	migrated, err := limitnats.New(js, append(opts, limitnats.WithStrictBucketStorage())...)
	require.NoError(t, err)
	info, err := migrated.Allow(ctx, "client", 5, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), info.Remaining, "the budget consumed before the migration must be kept")
}
