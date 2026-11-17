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

	natsstore "github.com/altessa-s/go-atlas/reliability/saga/storages/nats"
)

// TestNew_BucketStorage pins that a new bucket is file-backed, as requested.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	_, err := natsstore.New(t.Context(), js, natsstore.WithBucket("fresh"), natsstore.WithBucketTTL(time.Hour))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, natstest.KVBucketStorage(t, js, "fresh"))
}

// TestNew_BucketStorageMismatch pins that an existing bucket with another
// storage type is rejected and left untouched: the server cannot convert it.
func TestNew_BucketStorageMismatch(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	natstest.CreateNATSKV(t, js, "memory-bucket", time.Hour) // memory storage

	_, err := natsstore.New(t.Context(), js, natsstore.WithBucket("memory-bucket"), natsstore.WithBucketTTL(time.Hour))
	require.ErrorIs(t, err, natsstore.ErrBucketStorageMismatch)
	require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "memory-bucket"))
}
