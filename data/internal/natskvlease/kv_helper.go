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

	// Replicas is the number of replicas for the KeyValue bucket.
	// Zero falls back to the NATS client default of 1.
	Replicas int

	// LimitMarkerTTL controls how long the bucket retains delete-tombstone
	// markers when keys expire. Setting this to a positive value also
	// enables per-key TTL via [jetstream.KeyTTL] when calling Create or
	// Put. Zero leaves per-key TTL disabled, which is backward-compatible
	// with existing buckets and NATS server versions older than 2.11.
	LimitMarkerTTL time.Duration
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

	switch {
	case err == nil:
		// The bucket predates this call; make sure its key TTL matches ours.
		return h.reconcileTTL(ctx, kv, cfg)

	case !errors.Is(err, jetstream.ErrBucketNotFound):
		h.logger.ErrorContext(ctx, "failed to get KeyValue bucket",
			slog.String("bucket", cfg.Bucket),
			slog.Any("error", err))
		return nil, err
	}

	kv, err = Retry(ctx, func() (jetstream.KeyValue, error) {
		return h.js.CreateOrUpdateKeyValue(ctx, h.keyValueConfig(cfg))
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to create KeyValue bucket",
			slog.String("bucket", cfg.Bucket),
			slog.Any("error", err))
		return nil, err
	}

	// Defensive check
	if kv == nil {
		return nil, errors.New("failed to create or get KeyValue bucket: unexpected nil result")
	}

	h.logger.DebugContext(ctx, "created KeyValue bucket", slog.String("bucket", cfg.Bucket))

	return kv, nil
}

// keyValueConfig projects a BucketConfig onto the driver's bucket config.
func (h *KVHelper) keyValueConfig(cfg BucketConfig) jetstream.KeyValueConfig {
	return jetstream.KeyValueConfig{
		Bucket:         cfg.Bucket,
		TTL:            cfg.TTL,
		Storage:        cfg.Storage,
		Compression:    cfg.Compression,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: cfg.LimitMarkerTTL,
	}
}

// reconcileTTL brings a pre-existing bucket's key TTL in line with cfg.
//
// Every caller of this helper uses the bucket for leases, where the TTL is not
// a preference but the expiry mechanism: it is what releases the key when the
// holder dies without resigning. Adopting an existing bucket's TTL unchecked
// meant a bucket created earlier without one (an older release, an operator,
// a differently-configured component) silently produced leases that never
// expire — a lock without a TTL, which hangs the election until someone
// intervenes by hand.
func (h *KVHelper) reconcileTTL(ctx context.Context, kv jetstream.KeyValue, cfg BucketConfig) (jetstream.KeyValue, error) {
	status, err := Retry(ctx, func() (jetstream.KeyValueStatus, error) {
		return kv.Status(ctx)
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to read KeyValue bucket status",
			slog.String("bucket", cfg.Bucket), slog.Any("error", err))
		return nil, err
	}

	if status.TTL() == cfg.TTL {
		return kv, nil
	}

	h.logger.WarnContext(ctx, "KeyValue bucket TTL differs from the configured lease TTL, updating",
		slog.String("bucket", cfg.Bucket),
		slog.Duration("existing_ttl", status.TTL()),
		slog.Duration("configured_ttl", cfg.TTL))

	updated, err := Retry(ctx, func() (jetstream.KeyValue, error) {
		return h.js.CreateOrUpdateKeyValue(ctx, h.keyValueConfig(cfg))
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update KeyValue bucket TTL",
			slog.String("bucket", cfg.Bucket), slog.Any("error", err))
		return nil, err
	}

	return updated, nil
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
