// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Storage is a NATS JetStream KeyValue-backed idempotency key store.
// TTL is handled at the bucket level via MaxAge configuration.
type Storage struct {
	natsbase.Base
	opts *options
}

var _ storages.Storage = (*Storage)(nil)

// New creates a new NATS JetStream Storage with the given options.
// Creates the bucket if it does not exist. Returns error if js is nil.
//
// Example:
//
//	storage, err := nats.New(js, nats.WithBucket("idempotency"))
func New(js jetstream.JetStream, opts ...Option) (*Storage, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context cannot be nil")
	}

	options := newOptions(opts...)

	base, err := natsbase.NewBaseWithBucket(context.Background(), js, natskvlease.BucketConfig{
		Bucket:  options.bucket,
		TTL:     options.maxAge,
		Storage: jetstream.FileStorage,
	}, nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Storage{Base: base, opts: options}, nil
}

// AttemptLock tries to acquire a lock for the given key.
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, error) {
	if key == "" {
		return true, nil, nil
	}

	// Use Create to ensure we only set if key does not exist (atomic lock)
	_, err := s.KV().Create(ctx, key, val)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Key exists, get current state
			entry, getErr := s.KV().Get(ctx, key)
			if getErr != nil {
				if errors.Is(getErr, jetstream.ErrKeyNotFound) {
					// Race condition: existed during Create, but deleted before Get?
					// Assume we failed to lock? Or retry?
					// For simplicity return false and empty state? Or error?
					// If it's not found now, maybe we should have acquired it.
					// Let's return error to be safe.
					return false, nil, coreerrs.WrapOperation(getErr, "get existing state during conflict")
				}
				return false, nil, coreerrs.WrapOperation(getErr, "get existing state from NATS")
			}

			return false, entry.Value(), nil
		}
		return false, nil, coreerrs.WrapOperation(err, "attempt lock in NATS")
	}

	return true, nil, nil
}

// Complete marks the key as successfully processed.
func (s *Storage) Complete(ctx context.Context, key string, val []byte) error {
	if key == "" {
		return nil
	}

	// Overwrite existing key
	_, err := s.KV().Put(ctx, key, val)
	if err != nil {
		return coreerrs.WrapOperation(err, "complete idempotency key in NATS")
	}

	return nil
}

// Delete removes the key from storage.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}

	if err := s.KV().Delete(ctx, key); err != nil {
		return coreerrs.WrapOperation(err, "delete idempotency key from NATS")
	}

	return nil
}
