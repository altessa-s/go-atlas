// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	// DefaultBucketTTL is the default TTL for bucket keys.
	DefaultBucketTTL = 10 * time.Second
)

// BucketConfig holds configuration for creating a KeyValue bucket.
type BucketConfig struct {
	// Bucket is the name of the KeyValue bucket.
	Bucket string

	// TTL is the time-to-live for keys in the bucket.
	// If zero, DefaultBucketTTL is used.
	TTL time.Duration

	// Storage is the storage type (Memory or File).
	// Defaults to MemoryStorage.
	Storage jetstream.StorageType

	// Compression enables compression for the bucket.
	Compression bool
}

// KVHelper provides common KeyValue operations for NATS JetStream.
type KVHelper struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewKVHelper creates a new KVHelper instance.
func NewKVHelper(js jetstream.JetStream, logger *slog.Logger) *KVHelper {
	if logger == nil {
		logger = slog.Default()
	}
	return &KVHelper{
		js:     js,
		logger: logger,
	}
}

// GetOrCreateBucket retrieves an existing bucket or creates a new one.
// It uses retry logic to handle transient network errors.
func (h *KVHelper) GetOrCreateBucket(ctx context.Context, cfg BucketConfig) (jetstream.KeyValue, error) {
	if cfg.TTL == 0 {
		cfg.TTL = DefaultBucketTTL
	}
	if cfg.Storage == 0 {
		cfg.Storage = jetstream.MemoryStorage
	}

	// Try to get existing bucket
	kv, err := Retry(ctx, func() (jetstream.KeyValue, error) {
		return h.js.KeyValue(ctx, cfg.Bucket)
	})

	if err != nil {
		// If the bucket does not exist, create it
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			kv, err = Retry(ctx, func() (jetstream.KeyValue, error) {
				return h.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
					Bucket:      cfg.Bucket,
					TTL:         cfg.TTL,
					Storage:     cfg.Storage,
					Compression: cfg.Compression,
				})
			})
			if err != nil {
				h.logger.ErrorContext(ctx, "failed to create KeyValue bucket",
					slog.String("bucket", cfg.Bucket),
					slog.Any("error", err))
				return nil, err
			}
			h.logger.DebugContext(ctx, "created KeyValue bucket", slog.String("bucket", cfg.Bucket))
		} else {
			h.logger.ErrorContext(ctx, "failed to get KeyValue bucket",
				slog.String("bucket", cfg.Bucket),
				slog.Any("error", err))
			return nil, err
		}
	}

	// Defensive check
	if kv == nil && err == nil {
		return nil, errors.New("failed to create or get KeyValue bucket: unexpected nil result")
	}

	return kv, nil
}

// KVOps provides common KeyValue CRUD operations with retry logic.
type KVOps struct {
	kv     jetstream.KeyValue
	logger *slog.Logger
}

// NewKVOps creates a new KVOps instance.
func NewKVOps(kv jetstream.KeyValue, logger *slog.Logger) *KVOps {
	if logger == nil {
		logger = slog.Default()
	}
	return &KVOps{
		kv:     kv,
		logger: logger,
	}
}

// Get retrieves a value from the KeyValue store.
func (o *KVOps) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	return Retry(ctx, func() (jetstream.KeyValueEntry, error) {
		return o.kv.Get(ctx, key)
	})
}

// Create creates a new key-value pair. Fails if key already exists.
func (o *KVOps) Create(ctx context.Context, key string, value []byte) (uint64, error) {
	return Retry(ctx, func() (uint64, error) {
		return o.kv.Create(ctx, key, value)
	})
}

// Update updates an existing key with optimistic locking using revision.
func (o *KVOps) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	return Retry(ctx, func() (uint64, error) {
		return o.kv.Update(ctx, key, value, revision)
	})
}

// Delete deletes a key from the store.
func (o *KVOps) Delete(ctx context.Context, key string) error {
	_, err := Retry(ctx, func() (struct{}, error) {
		return struct{}{}, o.kv.Delete(ctx, key)
	})
	return err
}

// DeleteWithRevision deletes a key with optimistic locking using revision.
func (o *KVOps) DeleteWithRevision(ctx context.Context, key string, revision uint64) error {
	_, err := Retry(ctx, func() (struct{}, error) {
		return struct{}{}, o.kv.Delete(ctx, key, jetstream.LastRevision(revision))
	})
	return err
}

// Watch creates a watcher for changes to a specific key.
func (o *KVOps) Watch(ctx context.Context, key string) (jetstream.KeyWatcher, error) {
	return Retry(ctx, func() (jetstream.KeyWatcher, error) {
		return o.kv.Watch(ctx, key)
	})
}

// KV returns the underlying KeyValue store.
func (o *KVOps) KV() jetstream.KeyValue {
	return o.kv
}
