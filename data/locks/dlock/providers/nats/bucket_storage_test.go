// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

// TestNew_BucketStorage pins that the lock bucket is memory-backed and that an
// existing bucket with another storage type is adopted as is: the server
// cannot convert it.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()

	fresh, err := locknats.New(ctx, nc, locknats.WithBucket("fresh"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = fresh.Close(context.Background()) })
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "fresh"))

	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "durable",
		TTL:     locknats.DefaultBucketKeysTTL,
		Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)

	adopted, err := locknats.New(ctx, nc, locknats.WithBucket("durable"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adopted.Close(context.Background()) })
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "durable"), "the existing storage must be kept")
}
