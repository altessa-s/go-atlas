// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	limitnats "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
)

// TestNew_BucketStorage pins that a new bucket is file-backed, as requested.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	_, err := limitnats.New(t.Context(), js, limitnats.WithBucket("fresh"))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, natstest.KVBucketStorage(t, js, "fresh"))
}

// TestNew_BucketStorageMismatch pins that an existing bucket with another
// storage type is rejected and left untouched: the server cannot convert it.
func TestNew_BucketStorageMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	natstest.CreateNATSKV(t, js, "memory-bucket", limitnats.DefaultMaxAge) // memory storage

	_, err := limitnats.New(t.Context(), js, limitnats.WithBucket("memory-bucket"))
	require.ErrorIs(t, err, limitnats.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "memory-bucket"))
}
