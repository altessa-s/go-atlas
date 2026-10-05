// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	// DefaultBucketTTL is the default TTL for bucket keys.
	DefaultBucketTTL = 10 * time.Second
)

// ErrBucketTTLMismatch reports that a KeyValue bucket already exists with a
// key TTL other than the one requested. The bucket is left untouched: its TTL
// is what expires every key in it, including keys written by other processes
// configured with that TTL, so changing it from here would silently shorten or
// stretch their lifetimes. Align the configuration, migrate the bucket
// explicitly (see [BucketConfig.MigrateTTL]), or use another bucket.
var ErrBucketTTLMismatch = errors.New("KeyValue bucket key TTL differs from the configured TTL")

// ErrBucketStorageMismatch reports that a KeyValue bucket already exists with a
// storage type other than the one requested while [BucketConfig.StrictStorage]
// is set. The bucket is left untouched: the server cannot convert a bucket's
// storage type.
var ErrBucketStorageMismatch = errors.New("KeyValue bucket storage type differs from the configured storage")

// BucketConfig holds configuration for creating a KeyValue bucket.
type BucketConfig struct {
	// Bucket is the name of the KeyValue bucket.
	Bucket string

	// TTL is the time-to-live for keys in the bucket.
	// If zero, DefaultBucketTTL is used unless NoTTL is set.
	TTL time.Duration

	// NoTTL creates a bucket whose keys never expire; TTL is ignored. It
	// exists because a zero TTL means DefaultBucketTTL.
	NoTTL bool

	// Storage is the storage type of a bucket created by GetOrCreateBucket.
	// The zero value is [jetstream.FileStorage], the JetStream default, so
	// callers wanting memory storage must ask for [jetstream.MemoryStorage]
	// explicitly. An existing bucket keeps its storage type: the server
	// cannot convert it, so a mismatch is logged and the bucket adopted unless
	// StrictStorage is set.
	Storage jetstream.StorageType

	// StrictStorage rejects a pre-existing bucket whose storage type differs
	// from Storage with [ErrBucketStorageMismatch] instead of adopting it.
	StrictStorage bool

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

	// MigrateTTL updates a pre-existing bucket whose key TTL differs from TTL
	// to TTL. When false, such a bucket is rejected with
	// [ErrBucketTTLMismatch] and left as it is.
	MigrateTTL bool
}

// KVHelper provides common KeyValue operations for NATS JetStream.
type KVHelper struct {
	js     jetstream.JetStream
	logger *slog.Logger

	// failpoint, now and leaseTTL are test seams for the storage migration:
	// failpoint can fail a named step, now replaces the clock, leaseTTL sets
	// the TTL of a lease store the migration creates. All are zero in
	// production.
	failpoint func(step string) error
	now       func() time.Time
	leaseTTL  time.Duration
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
	cfg = cfg.normalized()

	// A bucket in the middle of a storage migration must not be used: it may
	// be sealed, missing, or not yet restored.
	if _, err := Retry(ctx, func() (jetstream.Stream, error) {
		return h.js.Stream(ctx, markerStreamName(cfg.Bucket))
	}); err == nil {
		return nil, fmt.Errorf("%w: bucket %q; finish it with MigrateBucketStorage and Resume",
			ErrBucketMigrationInProgress, cfg.Bucket)
	} else if !errors.Is(err, jetstream.ErrStreamNotFound) {
		h.logger.ErrorContext(ctx, "failed to check for a KeyValue bucket migration",
			slog.String("bucket", cfg.Bucket), slog.Any("error", err))
		return nil, err
	}

	// Try to get existing bucket
	kv, err := Retry(ctx, func() (jetstream.KeyValue, error) {
		return h.js.KeyValue(ctx, cfg.Bucket)
	})

	switch {
	case err == nil:
		// The bucket predates this call; check it against our config.
		return h.reconcile(ctx, kv, cfg)

	case !errors.Is(err, jetstream.ErrBucketNotFound):
		h.logger.ErrorContext(ctx, "failed to get KeyValue bucket",
			slog.String("bucket", cfg.Bucket),
			slog.Any("error", err))
		return nil, err
	}

	// Create only: an update here would rewrite a bucket another process
	// created between the lookup above and this call. If one did, adopt it
	// through the same TTL check as any pre-existing bucket.
	kv, err = Retry(ctx, func() (jetstream.KeyValue, error) {
		return h.js.CreateKeyValue(ctx, h.keyValueConfig(cfg))
	})
	if errors.Is(err, jetstream.ErrBucketExists) {
		kv, err = Retry(ctx, func() (jetstream.KeyValue, error) {
			return h.js.KeyValue(ctx, cfg.Bucket)
		})
		if err == nil {
			return h.reconcile(ctx, kv, cfg)
		}
	}
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

// normalized resolves the TTL defaults: NoTTL forces a zero (never expiring)
// TTL, and an unset TTL becomes DefaultBucketTTL.
func (cfg BucketConfig) normalized() BucketConfig {
	switch {
	case cfg.NoTTL:
		cfg.TTL = 0
	case cfg.TTL == 0:
		cfg.TTL = DefaultBucketTTL
	}
	return cfg
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

// reconcile checks a pre-existing bucket against cfg.
//
// A different storage type is adopted with a warning rather than rejected,
// unless cfg.StrictStorage asks for [ErrBucketStorageMismatch].
// The server cannot convert a bucket's storage, it does not change what other
// processes sharing the bucket rely on (only durability across server
// restarts), and earlier releases created memory buckets whatever storage was
// requested, so rejecting it would break every such deployment on upgrade.
// The existing storage is also kept when the TTL is migrated below, since an
// update asking for another storage type is refused by the server.
//
// The key TTL is not a preference but the expiry mechanism of every key in the
// bucket — for a lease, what releases the key when its holder dies. A bucket
// shared by processes configured with different TTLs therefore cannot be right
// for all of them, and rewriting it to whichever process started last silently
// changes the lifetime of everyone else's keys. A mismatch is rejected with
// [ErrBucketTTLMismatch] unless cfg.MigrateTTL asks for the bucket to be
// updated — e.g. to adopt a bucket created without a TTL, whose keys would
// otherwise never expire.
func (h *KVHelper) reconcile(ctx context.Context, kv jetstream.KeyValue, cfg BucketConfig) (jetstream.KeyValue, error) {
	status, err := Retry(ctx, func() (jetstream.KeyValueStatus, error) {
		return kv.Status(ctx)
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to read KeyValue bucket status",
			slog.String("bucket", cfg.Bucket), slog.Any("error", err))
		return nil, err
	}

	if existing := status.Config().Storage; existing != cfg.Storage {
		if cfg.StrictStorage {
			return nil, fmt.Errorf("%w: bucket %q has storage %s, configured %s",
				ErrBucketStorageMismatch, cfg.Bucket, existing, cfg.Storage)
		}
		h.logger.WarnContext(ctx, "KeyValue bucket storage differs from the configured storage, using the existing bucket as is; "+
			"recreate the bucket to change it (see the backend's README)",
			slog.String("bucket", cfg.Bucket),
			slog.String("existing_storage", existing.String()),
			slog.String("configured_storage", cfg.Storage.String()))
		cfg.Storage = existing
	}

	if status.TTL() == cfg.TTL {
		return kv, nil
	}

	if !cfg.MigrateTTL {
		return nil, fmt.Errorf("%w: bucket %q has key TTL %s, configured %s",
			ErrBucketTTLMismatch, cfg.Bucket, status.TTL(), cfg.TTL)
	}

	h.logger.WarnContext(ctx, "KeyValue bucket TTL differs from the configured TTL, migrating",
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
