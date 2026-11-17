// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	locknats "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
)

// TestNew_BucketStorage pins that the lock bucket is memory-backed.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()

	fresh, err := locknats.New(ctx, nc, locknats.WithBucket("fresh"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = fresh.Close(context.Background()) })
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "fresh"))

}

// TestNew_BucketStorageMismatch pins that an existing bucket with another
// storage type is rejected and left untouched: the server cannot convert it.
func TestNew_BucketStorageMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	_, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:  "durable",
		TTL:     locknats.DefaultBucketKeysTTL,
		Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)

	_, err = locknats.New(t.Context(), nc, locknats.WithBucket("durable"))
	require.ErrorIs(t, err, locknats.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.FileStorage, natstest.KVBucketStorage(t, js, "durable"))
}
