// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.
package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

// TestNew_BucketTTLMismatch pins that an existing bucket with a different key
// TTL is rejected rather than rewritten under the processes already using it,
// and is updated only when migration is asked for.
func TestNew_BucketTTLMismatch(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "shared-idempotency"

	_, err := idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(time.Hour))
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(2*time.Hour))
	require.ErrorIs(t, err, idempnats.ErrBucketTTLMismatch)

	kv, err := js.KeyValue(t.Context(), bucket)
	require.NoError(t, err)
	status, err := kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, status.TTL(), "a rejected New changed the shared bucket's TTL")

	_, err = idempnats.New(js, idempnats.WithBucket(bucket), idempnats.WithMaxAge(2*time.Hour), idempnats.WithMigrateBucketTTL())
	require.NoError(t, err)

	status, err = kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2*time.Hour, status.TTL(), "WithMigrateBucketTTL must update the bucket")
}
