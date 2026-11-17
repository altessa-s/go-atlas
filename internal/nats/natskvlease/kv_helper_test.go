// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

// TestGetOrCreateBucket_ExistingBucketTTL pins how a pre-existing bucket with a
// different key TTL is handled. Its TTL expires every key in it, including keys
// of processes configured with that TTL, so it is rejected and left untouched
// unless migration is asked for explicitly.
func TestGetOrCreateBucket_ExistingBucketTTL(t *testing.T) {
	t.Parallel()

	const existing = 5 * time.Second

	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		migrate bool
		wantErr error
		wantTTL time.Duration
	}{
		{name: "same TTL is adopted", ttl: existing, wantTTL: existing},
		{name: "different TTL is rejected", ttl: 2 * time.Second, wantErr: natskvlease.ErrBucketTTLMismatch, wantTTL: existing},
		{name: "different TTL is migrated on request", ttl: 2 * time.Second, migrate: true, wantTTL: 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ns := natstest.StartNATSServer(t)
			_, js := natstest.ConnectJetStream(t, ns)
			kv := natstest.CreateNATSKV(t, js, "shared", existing)

			helper := natskvlease.NewKVHelper(js, nil)
			got, err := helper.GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{
				Bucket:     "shared",
				TTL:        tc.ttl,
				Storage:    jetstream.MemoryStorage, // as CreateNATSKV made it
				MigrateTTL: tc.migrate,
			})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
			}

			status, err := kv.Status(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.wantTTL, status.TTL())
		})
	}
}

// racedJetStream reports the bucket missing on the first lookup, as if another
// process created it between that lookup and the creation that follows.
type racedJetStream struct {
	jetstream.JetStream

	looked atomic.Bool
}

func (r *racedJetStream) KeyValue(ctx context.Context, bucket string) (jetstream.KeyValue, error) {
	if r.looked.CompareAndSwap(false, true) {
		return nil, jetstream.ErrBucketNotFound
	}
	return r.JetStream.KeyValue(ctx, bucket)
}

// TestGetOrCreateBucket_ConcurrentCreation pins that a bucket created by
// someone else between the lookup and the creation goes through the same TTL
// check as any pre-existing bucket, instead of being overwritten by the
// creation call.
func TestGetOrCreateBucket_ConcurrentCreation(t *testing.T) {
	t.Parallel()

	const existing = 5 * time.Second

	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		migrate bool
		wantErr error
		wantTTL time.Duration
	}{
		{name: "same TTL is adopted", ttl: existing, wantTTL: existing},
		{name: "different TTL is rejected", ttl: 2 * time.Second, wantErr: natskvlease.ErrBucketTTLMismatch, wantTTL: existing},
		{name: "different TTL is migrated on request", ttl: 2 * time.Second, migrate: true, wantTTL: 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ns := natstest.StartNATSServer(t)
			_, js := natstest.ConnectJetStream(t, ns)
			kv := natstest.CreateNATSKV(t, js, "raced", existing)

			helper := natskvlease.NewKVHelper(&racedJetStream{JetStream: js}, nil)
			_, err := helper.GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{
				Bucket:     "raced",
				TTL:        tc.ttl,
				Storage:    jetstream.MemoryStorage, // as CreateNATSKV made it
				MigrateTTL: tc.migrate,
			})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			status, err := kv.Status(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.wantTTL, status.TTL())
		})
	}
}

// TestGetOrCreateBucket_Storage pins that a new bucket gets the requested
// storage type. FileStorage is the zero value of jetstream.StorageType and was
// once mistaken for "unset" and turned into MemoryStorage.
func TestGetOrCreateBucket_Storage(t *testing.T) {
	t.Parallel()

	for _, storage := range []jetstream.StorageType{jetstream.FileStorage, jetstream.MemoryStorage} {
		t.Run(storage.String(), func(t *testing.T) {
			t.Parallel()

			ns := natstest.StartNATSServer(t)
			_, js := natstest.ConnectJetStream(t, ns)

			_, err := natskvlease.NewKVHelper(js, nil).GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{
				Bucket:  "fresh",
				TTL:     time.Minute,
				Storage: storage,
			})
			require.NoError(t, err)
			require.Equal(t, storage, natstest.KVBucketStorage(t, js, "fresh"))
		})
	}
}

// TestGetOrCreateBucket_ExistingBucketStorage pins that an existing bucket
// with another storage type is rejected with ErrBucketStorageMismatch and left
// untouched — the server cannot convert it — even when a TTL migration is
// requested, and that a matching storage type is accepted.
func TestGetOrCreateBucket_ExistingBucketStorage(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		migrate bool
	}{
		{name: "same TTL", ttl: time.Minute},
		{name: "TTL migration", ttl: 2 * time.Minute, migrate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ns := natstest.StartNATSServer(t)
			_, js := natstest.ConnectJetStream(t, ns)
			natstest.CreateNATSKV(t, js, "existing", time.Minute) // memory storage

			got, err := natskvlease.NewKVHelper(js, nil).GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{
				Bucket:     "existing",
				TTL:        tc.ttl,
				Storage:    jetstream.FileStorage,
				MigrateTTL: tc.migrate,
			})
			require.ErrorIs(t, err, natskvlease.ErrBucketStorageMismatch)
			require.Nil(t, got)
			require.Equal(t, jetstream.MemoryStorage, natstest.KVBucketStorage(t, js, "existing"))
			require.Equal(t, time.Minute, natstest.KVBucketTTL(t, js, "existing"), "a rejected bucket must not be migrated")

			_, err = natskvlease.NewKVHelper(js, nil).GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{
				Bucket:  "existing",
				TTL:     time.Minute,
				Storage: jetstream.MemoryStorage,
			})
			require.NoError(t, err, "a matching storage type is accepted")
		})
	}
}

// TestGetOrCreateBucket_NoTTL pins that NoTTL creates a bucket whose keys never
// expire, where a zero TTL alone means DefaultBucketTTL.
func TestGetOrCreateBucket_NoTTL(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	helper := natskvlease.NewKVHelper(js, nil)

	_, err := helper.GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{Bucket: "forever", NoTTL: true})
	require.NoError(t, err)
	require.Zero(t, natstest.KVBucketTTL(t, js, "forever"))

	_, err = helper.GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{Bucket: "defaulted"})
	require.NoError(t, err)
	require.Equal(t, natskvlease.DefaultBucketTTL, natstest.KVBucketTTL(t, js, "defaulted"))

	_, err = helper.GetOrCreateBucket(t.Context(), natskvlease.BucketSpec{Bucket: "forever", NoTTL: true})
	require.NoError(t, err, "an existing bucket without TTL matches NoTTL")
}
