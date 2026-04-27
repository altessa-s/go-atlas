// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/uniq/providers"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Provider implements the uniq.Provider interface using NATS Key-Value store.
// It provides thread-safe operations for managing unique values with support
// for TTL through NATS JetStream.
type Provider struct {
	natsbase.Base
	opts *options
}

var (
	_ providers.Provider = (*Provider)(nil)
	_ providers.Prober   = (*Provider)(nil)
)

// New creates a new NATS provider with the specified connection, bucket name, and TTL.
// The bucket is used to namespace keys and avoid conflicts between different
// applications or components. If the bucket doesn't exist, it will be created
// with the specified TTL.
//
// The TTL (Time To Live) determines how long keys will be stored before they
// are automatically removed by NATS.
func New(nc *nats.Conn, opts ...Option) (*Provider, error) {
	if nc == nil {
		return nil, fmt.Errorf("nats connection cannot be nil")
	}

	// Check NATS server version for JetStream support
	if err := natskvlease.ValidateNatsVersion(nc); err != nil {
		return nil, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	if err = natskvlease.ValidateJetStreamEnabled(context.Background(), js, nil); err != nil {
		if errors.Is(err, nats.ErrJetStreamNotEnabled) {
			return nil, natskvlease.ErrNatsVersionNotSupported
		}
		return nil, err
	}

	options := newOptions(opts...)

	base, err := natsbase.NewBaseWithBucket(context.Background(), js, natskvlease.BucketConfig{
		Bucket:  options.bucket,
		TTL:     options.ttl,
		Storage: options.storageType,
		// Enable per-key TTL via [jetstream.KeyTTL]. Marker retention
		// matches the bucket TTL — uniq doesn't watch tombstones, so
		// this is just a "non-zero to enable" signal. Requires NATS
		// server 2.11+; older servers fail bucket creation here.
		LimitMarkerTTL: options.ttl,
	}, nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create/update key-value store")
	}

	return &Provider{
		Base: base,
		opts: options,
	}, nil
}

// Add adds a key to the NATS store with the configured TTL.
// If the key already exists, it will be overwritten with the new TTL.
func (p *Provider) Add(ctx context.Context, key string) error {
	return p.AddWithValue(ctx, key, []byte(""))
}

// AddWithValue adds a key with an associated value to the NATS store.
// If the key already exists, it will be overwritten with the new value.
func (p *Provider) AddWithValue(ctx context.Context, key string, value []byte) error {
	_, err := p.KV().Put(ctx, key, value)
	if err != nil {
		return coreerrs.WrapOperation(err, "add key")
	}
	return nil
}

// TryAdd atomically inserts a key only if it doesn't exist using NATS
// JetStream's KV Create operation. Returns (true, nil) when the insert
// succeeded, (false, nil) when the key was already present.
//
// ttl overrides the bucket's TTL for this key when positive (passed
// via [jetstream.KeyTTL]). Zero or negative falls back to the bucket
// TTL configured at construction.
func (p *Provider) TryAdd(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return p.TryAddWithValue(ctx, key, []byte(""), ttl)
}

// TryAddWithValue is like [Provider.TryAdd] but stores an associated
// value when the insert succeeds. The value is ignored when the key
// already exists.
func (p *Provider) TryAddWithValue(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	var opts []jetstream.KVCreateOpt
	if ttl > 0 {
		opts = append(opts, jetstream.KeyTTL(ttl))
	}
	if _, err := p.KV().Create(ctx, key, value, opts...); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return false, nil
		}
		return false, coreerrs.WrapOperation(err, "try add key")
	}
	return true, nil
}

// Exist checks if a key exists in the NATS store.
// Returns true if the key exists and hasn't expired, false otherwise.
func (p *Provider) Exist(ctx context.Context, key string) (bool, error) {
	_, err := p.KV().Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	if err != nil {
		return false, coreerrs.WrapOperation(err, "check key existence")
	}
	return true, nil
}

// GetValue retrieves the value associated with a key in the NATS store.
func (p *Provider) GetValue(ctx context.Context, key string) ([]byte, error) {
	kvEntry, err := p.KV().Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, nil // Key does not exist
	}
	if err != nil {
		return nil, coreerrs.Wrapf(err, "failed to get value for key %s", key)
	}
	return kvEntry.Value(), nil
}

// Remove removes a key from the NATS store.
// If the key doesn't exist, no error is returned.
func (p *Provider) Remove(ctx context.Context, key string) error {
	err := p.KV().Delete(ctx, key)
	if err != nil {
		return coreerrs.WrapOperation(err, "remove key")
	}
	return nil
}

// Clear removes all keys from the NATS store atomically.
// The operation is atomic and will either remove all keys or none of them.
func (p *Provider) Clear(ctx context.Context) error {
	// Delete all keys atomically using a single operation
	err := p.KV().Purge(ctx, "")
	if err != nil {
		return coreerrs.WrapOperation(err, "purge keys")
	}

	return nil
}

// Probe implements [providers.Prober]. Returns nil when the underlying
// JetStream KV bucket responds; any error from kv.Status surfaces as an
// unhealthy probe (typically caused by a closed *nats.Conn or a missing
// bucket).
func (p *Provider) Probe(ctx context.Context) error {
	if _, err := p.KV().Status(ctx); err != nil {
		return coreerrs.Wrap(err, "kv bucket unreachable")
	}
	return nil
}
