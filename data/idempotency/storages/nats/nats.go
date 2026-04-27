// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/binary"
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
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}

	// Use Create to ensure we only set if key does not exist (atomic lock)
	revision, err := s.KV().Create(ctx, key, val)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Key exists, get current state
			entry, getErr := s.KV().Get(ctx, key)
			if getErr != nil {
				if errors.Is(getErr, jetstream.ErrKeyNotFound) {
					// Race: existed during Create, gone before Get.
					// Surface as transport error so caller can retry.
					return false, nil, nil, coreerrs.WrapOperation(getErr, "get existing state during conflict")
				}
				return false, nil, nil, coreerrs.WrapOperation(getErr, "get existing state from NATS")
			}

			return false, entry.Value(), nil, nil
		}
		return false, nil, nil, coreerrs.WrapOperation(err, "attempt lock in NATS")
	}

	return true, nil, encodeRevisionToken(revision), nil
}

// Complete marks the key as successfully processed.
//
// Uses [jetstream.KV.Update] with the revision encoded in lockToken to
// detect a stolen lock. Returns [storages.ErrLockStolen] when the
// revision no longer matches (typically because the lock TTL expired
// and another caller's AttemptLock issued a fresh Create).
func (s *Storage) Complete(ctx context.Context, key string, val []byte, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}

	if lockToken == nil {
		// No CAS guard requested — fall back to unconditional Put.
		// Used by tests and adapters that bypass the safe path.
		if _, err := s.KV().Put(ctx, key, val); err != nil {
			return coreerrs.WrapOperation(err, "complete idempotency key in NATS")
		}
		return nil
	}

	revision, err := decodeRevisionToken(lockToken)
	if err != nil {
		return err
	}

	if _, err := s.KV().Update(ctx, key, val, revision); err != nil {
		// ErrKeyExists is what NATS returns for "wrong last sequence",
		// which is the signal that the revision we expected is no
		// longer current — somebody else owns the key now.
		if errors.Is(err, jetstream.ErrKeyExists) {
			return storages.ErrLockStolen
		}
		// Update against a missing key may surface as ErrKeyNotFound
		// (TTL'd between AttemptLock and Complete). Same effect: lock gone.
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return storages.ErrLockStolen
		}
		return coreerrs.WrapOperation(err, "complete idempotency key in NATS")
	}

	return nil
}

// encodeRevisionToken serializes a NATS KV revision into 8-byte big-endian
// bytes for use as the opaque lockToken. Returns a fresh slice each call.
func encodeRevisionToken(revision uint64) []byte {
	token := make([]byte, 8)
	binary.BigEndian.PutUint64(token, revision)
	return token
}

// decodeRevisionToken parses an 8-byte big-endian lockToken back into a
// NATS KV revision. Returns ErrLockStolen for malformed tokens — a token
// that didn't come from this backend's AttemptLock can't possibly match
// any real revision.
func decodeRevisionToken(token []byte) (uint64, error) {
	if len(token) != 8 {
		return 0, fmt.Errorf("%w: malformed token (len=%d, want 8)", storages.ErrLockStolen, len(token))
	}
	return binary.BigEndian.Uint64(token), nil
}

// Delete removes the key from storage.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return storages.ErrEmptyKey
	}

	if err := s.KV().Delete(ctx, key); err != nil {
		return coreerrs.WrapOperation(err, "delete idempotency key from NATS")
	}

	return nil
}
