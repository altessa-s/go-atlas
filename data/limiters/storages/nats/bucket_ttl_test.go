// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.
package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

// TestNew_BucketTTLMismatch pins that an existing bucket with a different key
// TTL is rejected rather than rewritten under the processes already using it,
// and is updated only when migration is asked for.
func TestNew_BucketTTLMismatch(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "shared-limiters"

	_, err := limitnats.New(js, limitnats.WithBucket(bucket), limitnats.WithMaxAge(time.Hour))
	require.NoError(t, err)

	_, err = limitnats.New(js, limitnats.WithBucket(bucket), limitnats.WithMaxAge(2*time.Hour))
	require.ErrorIs(t, err, limitnats.ErrBucketTTLMismatch)

	kv, err := js.KeyValue(t.Context(), bucket)
	require.NoError(t, err)
	status, err := kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, status.TTL(), "a rejected New changed the shared bucket's TTL")

	_, err = limitnats.New(js, limitnats.WithBucket(bucket), limitnats.WithMaxAge(2*time.Hour), limitnats.WithMigrateBucketTTL())
	require.NoError(t, err)

	status, err = kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, status.TTL(), "WithMigrateBucketTTL must update the bucket")
}
