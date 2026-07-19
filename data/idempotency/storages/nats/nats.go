// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
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
		Bucket:   options.bucket,
		TTL:      options.maxAge,
		Storage:  jetstream.FileStorage,
		Replicas: options.replicas,
		// Enable per-key TTL so AttemptLockWithTTL can pass
		// jetstream.KeyTTL(d) on Create. Marker retention matches the
		// bucket TTL — we don't watch tombstones, so any non-zero value
		// is fine. Requires NATS server 2.11+; older servers fail
		// bucket creation here, which is the right time to surface the
		// requirement.
		LimitMarkerTTL: options.maxAge,
	}, nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Storage{Base: base, opts: options}, nil
}

// AttemptLock tries to acquire a lock for the given key using the
// bucket's configured TTL.
func (s *Storage) AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, []byte, error) {
	return s.AttemptLockWithTTL(ctx, key, val, 0)
}

// AttemptLockWithTTL is like [Storage.AttemptLock] but lockTtl
// overrides the bucket's TTL for this key when positive.
func (s *Storage) AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTtl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}

	var createOpts []jetstream.KVCreateOpt
	createOpts = coreslices.AppendIf(createOpts, lockTtl > 0, jetstream.KeyTTL(lockTtl))

	// Use Create to ensure we only set if key does not exist (atomic lock)
	revision, err := s.KV().Create(ctx, key, val, createOpts...)
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

// revisionTokenLen is the byte length of a serialized NATS KV revision
// (uint64 in big-endian). Used by encodeRevisionToken /
// decodeRevisionToken to keep the size invariant explicit.
const revisionTokenLen = 8

// encodeRevisionToken serializes a NATS KV revision into a big-endian
// byte slice for use as the opaque lockToken. Returns a fresh slice
// each call.
func encodeRevisionToken(revision uint64) []byte {
	token := make([]byte, revisionTokenLen)
	binary.BigEndian.PutUint64(token, revision)
	return token
}

// decodeRevisionToken parses a big-endian lockToken back into a NATS
// KV revision. Returns ErrLockStolen for malformed tokens — a token
// that didn't come from this backend's AttemptLock can't possibly match
// any real revision.
func decodeRevisionToken(token []byte) (uint64, error) {
	if len(token) != revisionTokenLen {
		return 0, fmt.Errorf("%w: malformed token (len=%d, want %d)", storages.ErrLockStolen, len(token), revisionTokenLen)
	}
	return binary.BigEndian.Uint64(token), nil
}

// Steal atomically replaces the value when current bytes equal
// expectedVal. Performs Get → byte-compare → Update(... revision).
// The revision check on Update protects against the TOCTOU race
// between Get and Update: if another caller updates the key in
// between, our Update fails with ErrKeyExists and we surface
// [storages.ErrLockStolen].
//
// Returns the new lockToken (encoded post-Update revision) on
// success.
func (s *Storage) Steal(ctx context.Context, key string, expectedVal, newVal []byte) ([]byte, error) {
	if key == "" {
		return nil, storages.ErrEmptyKey
	}

	entry, err := s.KV().Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, storages.ErrLockStolen
		}
		return nil, coreerrs.WrapOperation(err, "get current value during steal")
	}

	if !bytes.Equal(entry.Value(), expectedVal) {
		return nil, storages.ErrLockStolen
	}

	revision, err := s.KV().Update(ctx, key, newVal, entry.Revision())
	if err != nil {
		// ErrKeyExists = wrong last sequence = somebody else wrote in
		// the Get/Update gap. ErrKeyNotFound = the key disappeared
		// (TTL'd). Both surface as ErrLockStolen.
		if errors.Is(err, jetstream.ErrKeyExists) || errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, storages.ErrLockStolen
		}
		return nil, coreerrs.WrapOperation(err, "update during steal")
	}

	return encodeRevisionToken(revision), nil
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
