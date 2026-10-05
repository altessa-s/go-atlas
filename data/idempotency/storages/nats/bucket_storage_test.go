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

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

// TestNew_BucketStorage pins that a new bucket is file-backed, as requested,
// and that an existing bucket with another storage type (such as the memory
// buckets earlier releases created by mistake) is adopted as is: the server
// cannot convert it.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	_, err := idempnats.New(js, idempnats.WithBucket("fresh"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "fresh"))

	kv, err := js.KeyValue(t.Context(), "fresh")
	require.NoError(t, err)
	status, err := kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, status.LimitMarkerTTL(), "a new bucket must support per-key TTL")

	_, err = js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:   "legacy",
		TTL:      time.Hour,
		Storage:  jetstream.MemoryStorage,
		Replicas: 1,
	})
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket("legacy"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "legacy"), "the existing storage must be kept")
}

// TestNew_StrictBucketStorage pins that WithStrictBucketStorage rejects a
// bucket with another storage type instead of adopting it.
func TestNew_StrictBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)
	testhelpers.CreateNATSKV(t, js, "legacy", time.Hour) // memory storage

	_, err := idempnats.New(js, idempnats.WithBucket("legacy"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1),
		idempnats.WithStrictBucketStorage())
	require.ErrorIs(t, err, idempnats.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "legacy"))
}
