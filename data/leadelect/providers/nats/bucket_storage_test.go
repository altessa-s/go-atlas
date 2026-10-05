// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// TestProvider_New_BucketStorage pins that the election bucket gets the
// configured storage — memory by default, file with WithStorage(FileStorage),
// which used to be turned into memory — and that an existing bucket with
// another storage type is adopted as is.
func TestProvider_New_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)
	ctx := t.Context()

	_, err := lenats.New(ctx, nc, lenats.WithBucket("default"))
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "default"))

	_, err = lenats.New(ctx, nc, lenats.WithBucket("durable"), lenats.WithStorage(jetstream.FileStorage))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "durable"))

	// An existing memory bucket stays memory when file storage is asked for.
	_, err = lenats.New(ctx, nc, lenats.WithBucket("default"), lenats.WithStorage(jetstream.FileStorage))
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "default"), "the existing storage must be kept")
}
