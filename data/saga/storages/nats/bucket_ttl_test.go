// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.
package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
)

// TestNew_BucketTTLMismatch pins that an existing bucket with a different key
// TTL is rejected rather than rewritten under the processes already using it,
// and is updated only when migration is asked for.
func TestNew_BucketTTLMismatch(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "shared-saga"

	_, err := natsstore.New(js, natsstore.WithBucket(bucket), natsstore.WithBucketTTL(time.Hour))
	require.NoError(t, err)

	_, err = natsstore.New(js, natsstore.WithBucket(bucket), natsstore.WithBucketTTL(2*time.Hour))
	require.ErrorIs(t, err, natsstore.ErrBucketTTLMismatch)

	require.Equal(t, time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "a rejected New changed the shared bucket's TTL")

	_, err = natsstore.New(js, natsstore.WithBucket(bucket), natsstore.WithBucketTTL(2*time.Hour), natsstore.WithMigrateBucketTTL())
	require.NoError(t, err)

	require.Equal(t, 2*time.Hour, testhelpers.KVBucketTTL(t, js, bucket), "WithMigrateBucketTTL must update the bucket")
}
