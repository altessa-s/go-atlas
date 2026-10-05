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

	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
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

			ns := testhelpers.StartNATSServer(t)
			_, js := testhelpers.ConnectJetStream(t, ns)
			kv := testhelpers.CreateNATSKV(t, js, "shared", existing)

			helper := natskvlease.NewKVHelper(js, nil)
			got, err := helper.GetOrCreateBucket(t.Context(), natskvlease.BucketConfig{
				Bucket:     "shared",
				TTL:        tc.ttl,
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

			ns := testhelpers.StartNATSServer(t)
			_, js := testhelpers.ConnectJetStream(t, ns)
			kv := testhelpers.CreateNATSKV(t, js, "raced", existing)

			helper := natskvlease.NewKVHelper(&racedJetStream{JetStream: js}, nil)
			_, err := helper.GetOrCreateBucket(t.Context(), natskvlease.BucketConfig{
				Bucket:     "raced",
				TTL:        tc.ttl,
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
