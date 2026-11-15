// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskey"
	"github.com/altessa-s/go-atlas/store/mongo/storages/internal/kvstore"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// DefaultTTL is the recommended key TTL of the cursor bucket (1 hour), the
// same lifetime the Redis and in-memory cursor storages default to.
const DefaultTTL = 1 * time.Hour

// ErrKeyTooLong is returned, before any network I/O, for a cursor key whose
// escaped form exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

// backend implements kvstore.Storage for NATS JetStream KeyValue. It takes
// logical keys and escapes each into a valid KeyValue key.
type backend struct {
	natsbase.Base
}

// Get retrieves a value from NATS KeyValue.
func (b *backend) Get(ctx context.Context, key string) ([]byte, error) {
	key, err := natskey.Encode(key)
	if err != nil {
		return nil, err
	}
	entry, err := b.KV().Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, kvstore.ErrKeyNotFound
		}
		return nil, err
	}
	return entry.Value(), nil
}

// Set stores a value in NATS KeyValue.
func (b *backend) Set(ctx context.Context, key string, value []byte) error {
	key, err := natskey.Encode(key)
	if err != nil {
		return err
	}
	_, err = b.KV().Put(ctx, key, value)
	return err
}

// Delete removes a key from NATS KeyValue (idempotent).
func (b *backend) Delete(ctx context.Context, key string) error {
	key, err := natskey.Encode(key)
	if err != nil {
		return err
	}
	if err := b.KV().Purge(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil // Idempotent
		}
		return err
	}
	return nil
}

// Storage is a NATS JetStream KeyValue implementation of mongotools.Storage.
// This implementation is suitable for production deployments with multiple instances
// where cursor sharing across instances is required.
//
// Features:
//   - Distributed: Cursors shared across all application instances
//   - Automatic expiration: the bucket's key TTL is the only cursor cleanup,
//     so [New] requires a bucket that has one
//   - Persistent: Survives application restarts (if JetStream is persistent)
//   - Thread-safe: NATS handles concurrent access
//   - Configurable bucket name: Allows multiple isolated cursor storages
//
// Example usage:
//
//	nc, _ := nats.Connect(nats.DefaultURL)
//	js, _ := jetstream.New(nc)
//	kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
//	    Bucket: "cursors",
//	    TTL:    nats.DefaultTTL,
//	})
//
//	storage, err := nats.New(ctx, kv)
//
//	result, err := mongotools.ListCursor(ctx, collection,
//	    mongotools.WithListCursorStorage(storage),
//	    mongotools.WithListCursorLimit(50),
//	)
type Storage struct {
	*kvstore.JSONStorage
	natsbase.Base
}

// New creates a new NATS JetStream KeyValue cursor storage.
//
// The KeyValue bucket should be created by the caller with desired configuration
// (TTL, storage type, replicas, etc.). Its key TTL must be positive: cursors
// are never deleted, so the TTL is what keeps the bucket from growing by one
// entry per issued page forever. New reads the bucket status and rejects a
// bucket without a TTL with coreerrs.ErrInvalidArgument.
//
// Parameters:
//   - ctx: Bounds the bucket status read
//   - kv: NATS JetStream KeyValue bucket for storing cursor metadata
//
// Returns:
//   - *Storage: Ready to use storage instance
//   - error: The bucket has no key TTL, or its status could not be read
//
// Example:
//
//	nc, _ := nats.Connect(nats.DefaultURL)
//	js, _ := jetstream.New(nc)
//	kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
//	    Bucket:  "cursors",
//	    TTL:     nats.DefaultTTL,
//	    Storage: jetstream.FileStorage,
//	})
//	storage, err := nats.New(ctx, kv)
func New(ctx context.Context, kv jetstream.KeyValue) (*Storage, error) {
	status, err := kv.Status(ctx)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "get bucket status")
	}
	if ttl := status.TTL(); ttl <= 0 {
		return nil, coreerrs.Wrapf(coreerrs.ErrInvalidArgument,
			"nats cursor storage: bucket %q has no key TTL, its cursors would never expire; set a positive TTL on the bucket", status.Bucket())
	}

	base := natsbase.NewBase(kv)
	return &Storage{
		JSONStorage: kvstore.NewJSONStorage(&backend{Base: base}, "nats"),
		Base:        base,
	}, nil
}

// Delete removes cursor metadata from NATS KeyValue.
// Returns nil if key doesn't exist (idempotent).
//
// Thread-safe: NATS handles concurrent access.
func (s *Storage) Delete(ctx context.Context, key string) error {
	key, err := natskey.Encode(key)
	if err != nil {
		return coreerrs.WrapOperation(err, "delete cursor from nats")
	}

	// Override to use Purge for NATS (removes all revisions)
	if err := s.KV().Purge(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return coreerrs.WrapOperation(err, "delete cursor from nats")
	}
	return nil
}

// Ping checks if the NATS JetStream connection is alive.
// Useful for health checks and debugging.
//
// Returns error if NATS is unreachable or unhealthy.
func (s *Storage) Ping(ctx context.Context) error {
	// Check bucket status
	_, err := s.KV().Status(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "get bucket status")
	}

	return nil
}
