// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	idempnats "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
)

// TestNew_BucketStorage pins that a new bucket is file-backed, as requested.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	_, err := idempnats.New(js, idempnats.WithBucket("fresh"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, natstest.KVBucketStorage(t, js, "fresh"))

	kv, err := js.KeyValue(t.Context(), "fresh")
	require.NoError(t, err)
	status, err := kv.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, time.Hour, status.LimitMarkerTTL(), "a new bucket must support per-key TTL")
}

// TestNew_BucketStorageMismatch pins that an existing bucket with another
// storage type is rejected and left untouched: the server cannot convert it.
func TestNew_BucketStorageMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	natstest.CreateNATSKV(t, js, "memory-bucket", time.Hour) // memory storage

	_, err := idempnats.New(js, idempnats.WithBucket("memory-bucket"), idempnats.WithMaxAge(time.Hour), idempnats.WithReplicas(1))
	require.ErrorIs(t, err, idempnats.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "memory-bucket"))
}

func TestNew_RejectsMirroredBucket(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()
	_, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "source", TTL: time.Hour})
	require.NoError(t, err)
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "mirror", TTL: time.Hour, Mirror: &jetstream.StreamSource{Name: "source"},
	})
	require.NoError(t, err)

	_, err = idempnats.New(js, idempnats.WithBucket("mirror"), idempnats.WithMaxAge(time.Hour))
	require.ErrorIs(t, err, idempnats.ErrMirroredBucket)
}
