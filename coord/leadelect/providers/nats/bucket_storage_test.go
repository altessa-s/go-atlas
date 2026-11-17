// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

// TestProvider_New_BucketStorage pins that the election bucket gets the
// configured storage — memory by default, file with WithStorage(FileStorage),
// which used to be turned into memory.
func TestProvider_New_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	ctx := t.Context()

	_, err := lenats.New(ctx, nc, lenats.WithBucket("default"))
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "default"))

	_, err = lenats.New(ctx, nc, lenats.WithBucket("durable"), lenats.WithStorage(jetstream.FileStorage))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, natstest.KVBucketStorage(t, js, "durable"))
}

// TestProvider_New_BucketStorageMismatch pins that an existing bucket with
// another storage type is rejected and left untouched: the server cannot
// convert it.
func TestProvider_New_BucketStorageMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	natstest.CreateNATSKV(t, js, "memory-bucket", lenats.DefaultBucketKeysTTL)

	_, err := lenats.New(t.Context(), nc, lenats.WithBucket("memory-bucket"), lenats.WithStorage(jetstream.FileStorage))
	require.ErrorIs(t, err, lenats.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "memory-bucket"))
}
