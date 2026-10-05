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

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

// TestNew_BucketTTLMismatch pins that a provider configured with a TTL other
// than the one an existing bucket was created with is rejected rather than
// rewriting the bucket. Rewriting it used to shorten (or stretch) the locks of
// every provider already sharing the bucket.
func TestNew_BucketTTLMismatch(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)
	ctx := t.Context()

	const bucket = "shared-locks"

	first, err := locknats.New(ctx, nc, locknats.WithBucket(bucket), locknats.WithTTL(10*time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close(context.Background()) })

	_, err = locknats.New(ctx, nc, locknats.WithBucket(bucket), locknats.WithTTL(2*time.Second))
	require.ErrorIs(t, err, locknats.ErrBucketTTLMismatch)

	js, err := jetstream.New(nc)
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, testhelpers.KVBucketTTL(t, js, bucket), "a rejected provider changed the shared bucket's TTL")

	lk, err := first.Lock(ctx, "resource")
	require.NoError(t, err, "the provider already using the bucket must be unaffected")
	require.NoError(t, lk.Release(ctx))

	migrated, err := locknats.New(ctx, nc, locknats.WithBucket(bucket), locknats.WithTTL(2*time.Second),
		locknats.WithMigrateBucketTTL())
	require.NoError(t, err)
	t.Cleanup(func() { _ = migrated.Close(context.Background()) })

	require.Equal(t, 2*time.Second, testhelpers.KVBucketTTL(t, js, bucket), "WithMigrateBucketTTL must update the bucket")
}
