// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	limitnats "github.com/altessa-s/go-atlas/data/limiters/storages/nats"
)

// TestNew_BucketStorage pins that a new bucket is file-backed, as requested,
// and that an existing bucket with another storage type (such as the memory
// buckets earlier releases created by mistake) is adopted as is: the server
// cannot convert it.
func TestNew_BucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	_, js := testhelpers.ConnectJetStream(t, ns)

	_, err := limitnats.New(js, limitnats.WithBucket("fresh"))
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, testhelpers.KVBucketStorage(t, js, "fresh"))

	_, err = js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:   "legacy",
		TTL:      limitnats.DefaultMaxAge,
		Storage:  jetstream.MemoryStorage,
		Replicas: 1,
	})
	require.NoError(t, err)

	_, err = limitnats.New(js, limitnats.WithBucket("legacy"))
	require.NoError(t, err)
	require.Equal(t, jetstream.MemoryStorage, testhelpers.KVBucketStorage(t, js, "legacy"), "the existing storage must be kept")
}
