// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelectit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// TestBucket_TTLReconciledOnAdoption asserts that adopting a bucket which
// predates the provider does not leave leases that never expire.
//
// The bucket's key TTL is the mechanism that releases the election key when a
// holder dies without resigning. A bucket created earlier without one — by an
// older release, an operator, or a differently configured component — would
// otherwise produce a lock with no TTL, and the election would hang until
// someone intervened by hand.
func TestBucket_TTLReconciledOnAdoption(t *testing.T) {
	t.Parallel()

	nc := connect(t)
	ctx := t.Context()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	bucket := "adopted-" + uniqueSuffix()
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  bucket,
		Storage: jetstream.MemoryStorage,
		// TTL deliberately omitted: keys would never expire.
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteKeyValue(context.WithoutCancel(ctx), bucket) })

	kv, err := js.KeyValue(ctx, bucket)
	require.NoError(t, err)
	status, err := kv.Status(ctx)
	require.NoError(t, err)
	require.Zero(t, status.TTL(), "precondition: the adopted bucket starts without a key TTL")

	_, err = lenats.New(ctx, nc, lenats.WithBucket(bucket))
	require.NoError(t, err)

	status, err = kv.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, lenats.DefaultBucketKeysTTL, status.TTL(),
		"an adopted bucket must be reconciled to the lease TTL")
}

// TestBucket_KeyExpiresWithoutRenewal asserts the server-side half of the lease
// directly: a key written into the election bucket and then left alone is aged
// out by JetStream. Every failover after an abrupt loss rests on this, and it
// is the one part of the mechanism that lives entirely in the server.
func TestBucket_KeyExpiresWithoutRenewal(t *testing.T) {
	t.Parallel()

	nc := connect(t)
	ctx := t.Context()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	bucket := "expiry-" + uniqueSuffix()

	// Build the bucket through the provider so the TTL under test is the one
	// the provider actually configures, not one the test invented.
	_, err = lenats.New(ctx, nc, lenats.WithBucket(bucket))
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteKeyValue(context.WithoutCancel(ctx), bucket) })

	kv, err := js.KeyValue(ctx, bucket)
	require.NoError(t, err)

	_, err = kv.Create(ctx, "abandoned", []byte("node-1"))
	require.NoError(t, err)

	_, err = kv.Get(ctx, "abandoned")
	require.NoError(t, err, "precondition: the key exists right after it is written")

	require.Eventually(t, func() bool {
		_, getErr := kv.Get(ctx, "abandoned")
		return errors.Is(getErr, jetstream.ErrKeyNotFound)
	}, lenats.DefaultBucketKeysTTL+5*time.Second, 250*time.Millisecond,
		"an unrenewed election key must be aged out by the server")
}
