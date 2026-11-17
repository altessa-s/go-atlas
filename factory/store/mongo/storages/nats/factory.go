// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/store/mongo"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	mongofactory "github.com/altessa-s/go-atlas/factory/store/mongo"
	natskvlease "github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	natsstorage "github.com/altessa-s/go-atlas/store/mongo/storages/nats"
)

const (
	// DefaultBucket is the default NATS KeyValue bucket name for cursor storage.
	DefaultBucket = "cursor"
)

var (
	// ErrJetStreamRequired is returned when the NATS cursor storage is built
	// without a JetStream context.
	ErrJetStreamRequired = errors.New("cursor storage: jetstream is required for storage type nats")
	// ErrConfigRequired is returned when the storage configuration has no
	// nats block.
	ErrConfigRequired = errors.New("cursor storage: nats configuration is required for storage type nats")
)

// ErrBucketTTLMismatch is returned by the storage function when the bucket
// already exists with a key TTL other than the builder's and migrateBucketTTL
// is not set. The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by the storage function when the
// bucket already exists with another storage type. The bucket is left
// untouched.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// Builder returns the storage function of the nats type, ready for
// mongofactory.CursorStorageBuilder.UseStorage. The storage keeps cursors in
// a KeyValue bucket of js.
//
// The builder's TTL must be positive: cursors are never deleted, so the
// bucket's key TTL is their only cleanup, and a zero or negative TTL fails with
// coreerrs.ErrInvalidArgument (natsstorage.DefaultTTL is the recommended
// value). The bucket is created with that TTL or, when it exists, checked
// against it like every other NATS KV backend: a different key TTL — no TTL
// included — fails with [ErrBucketTTLMismatch] unless migrateBucketTTL is set,
// which updates the bucket in place, and a different storage type fails with
// [ErrBucketStorageMismatch].
func Builder(js jetstream.JetStream) mongofactory.StorageFunc {
	return func(ctx context.Context, cfg *storageconfig.BackendConfig, ttl time.Duration, logger *slog.Logger) (mongo.Storage, error) {
		if js == nil {
			return nil, ErrJetStreamRequired
		}
		if cfg.NATS == nil {
			return nil, ErrConfigRequired
		}
		if ttl <= 0 {
			return nil, corefactory.Errorf(coreerrs.ErrInvalidArgument,
				"cursor storage: storage type nats requires a positive TTL, got %s: set it with WithTTL", ttl)
		}

		kv, err := natskvlease.NewKVHelper(js, logger).GetOrCreateBucket(ctx, natskvlease.BucketSpec{
			Bucket:     cmp.Or(cfg.NATS.Bucket, DefaultBucket),
			TTL:        ttl,
			Storage:    jetstream.FileStorage,
			Replicas:   cmp.Or(cfg.NATS.Replicas, 1),
			MigrateTTL: cfg.NATS.MigrateBucketTTL,
		})
		if err != nil {
			return nil, coreerrs.Wrap(err, "failed to create NATS KeyValue bucket")
		}

		storage, err := natsstorage.New(ctx, kv)
		if err != nil {
			return nil, err
		}
		return storage, nil
	}
}
