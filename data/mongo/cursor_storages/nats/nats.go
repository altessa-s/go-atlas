// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/kvstore"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// backend implements kvstore.Backend for NATS JetStream KeyValue.
type backend struct {
	natsbase.Base
}

// Get retrieves a value from NATS KeyValue.
func (b *backend) Get(ctx context.Context, key string) ([]byte, error) {
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
	_, err := b.KV().Put(ctx, key, value)
	return err
}

// Delete removes a key from NATS KeyValue (idempotent).
func (b *backend) Delete(ctx context.Context, key string) error {
	if err := b.KV().Purge(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil // Idempotent
		}
		return err
	}
	return nil
}

// Storage is a NATS JetStream KeyValue implementation of mongotools.CursorStorage.
// This implementation is suitable for production deployments with multiple instances
// where cursor sharing across instances is required.
//
// Features:
//   - Distributed: Cursors shared across all application instances
//   - Automatic expiration: NATS KeyValue TTL handles cursor cleanup
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
//	    TTL:    1 * time.Hour,
//	})
//
//	storage := nats.New(kv)
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
// (TTL, storage type, replicas, etc.).
//
// Parameters:
//   - kv: NATS JetStream KeyValue bucket for storing cursor metadata
//
// Returns:
//   - *Storage: Ready to use storage instance
//
// Example:
//
//	nc, _ := nats.Connect(nats.DefaultURL)
//	js, _ := jetstream.New(nc)
//	kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
//	    Bucket:  "cursors",
//	    TTL:     1 * time.Hour,
//	    Storage: jetstream.FileStorage,
//	})
//	storage := nats.New(kv)
func New(kv jetstream.KeyValue) *Storage {
	base := natsbase.NewBase(kv)
	return &Storage{
		JSONStorage: kvstore.NewJSONStorage(&backend{Base: base}, "nats"),
		Base:        base,
	}
}

// Delete removes cursor metadata from NATS KeyValue.
// Returns nil if key doesn't exist (idempotent).
//
// Thread-safe: NATS handles concurrent access.
func (s *Storage) Delete(ctx context.Context, key string) error {
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
