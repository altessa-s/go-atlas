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
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskey"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrBucketTTLMismatch is returned by New when the bucket already exists with a
// key TTL other than the configured one and [WithMigrateBucketTTL] is not set.
// The bucket is left untouched.
var ErrBucketTTLMismatch = natskvlease.ErrBucketTTLMismatch

// ErrBucketStorageMismatch is returned by New when the bucket already exists
// with another storage type. The bucket is left untouched:
// the server cannot convert a bucket's storage type.
var ErrBucketStorageMismatch = natskvlease.ErrBucketStorageMismatch

// ErrKeyTooLong is returned, before any network I/O, for a key whose escaped
// form exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

// Storage is a NATS JetStream KeyValue-backed idempotency key store.
// Locks use the bucket MaxAge by default and accept a per-key TTL override.
type Storage struct {
	natsbase.Base
	opts *options
	js   jetstream.JetStream
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

	base, err := natsbase.NewBaseWithBucket(context.Background(), js, bucketConfig(options), nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Storage{Base: base, opts: options, js: js}, nil
}

// bucketConfig is the bucket New creates or reconciles.
func bucketConfig(o *options) natskvlease.BucketSpec {
	return natskvlease.BucketSpec{
		Bucket:   o.bucket,
		TTL:      o.maxAge,
		Storage:  jetstream.FileStorage,
		Replicas: o.replicas,
		// Enable per-key TTL so AttemptLockWithTTL can pass
		// jetstream.KeyTTL(d) on Create. Marker retention matches the
		// bucket TTL — we don't watch tombstones, so any non-zero value
		// is fine. Requires NATS server 2.11+; older servers fail
		// bucket creation here, which is the right time to surface the
		// requirement.
		LimitMarkerTTL: o.maxAge,
		MigrateTTL:     o.migrateBucketTTL,
	}
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
	key, err := physicalKey(key)
	if err != nil {
		return false, nil, nil, err
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
	key, err := physicalKey(key)
	if err != nil {
		return err
	}

	if lockToken == nil {
		// No CAS guard requested — fall back to unconditional Put.
		// Used by tests and adapters that bypass the safe path.
		if _, err = s.KV().Put(ctx, key, val); err != nil {
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

// UsesKeeperClock delegates the age check to the Keeper. NATS KV requires
// synchronized Keeper clocks because it cannot supply a server-side clock
// to the conditional update.
func (s *Storage) UsesKeeperClock() bool { return true }

// Steal checks the observed value and replaces it using the KV revision as
// a compare-and-swap guard. A positive maxLockAge is refused because NATS
// cannot verify it. The Keeper passes zero after checking the lock's age.
func (s *Storage) Steal(ctx context.Context, key string, expected, replacement []byte, maxLockAge, ttl time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}
	key, err := physicalKey(key)
	if err != nil {
		return false, nil, nil, err
	}

	current, found, err := s.currentForSteal(ctx, key)
	if !found || err != nil {
		return false, nil, nil, err
	}
	if maxLockAge > 0 || !bytes.Equal(current.Value(), expected) {
		return false, current.Value(), nil, nil
	}

	revision, err := s.updateForSteal(ctx, key, replacement, current.Revision(), ttl)
	if err == nil {
		return true, nil, encodeRevisionToken(revision), nil
	}
	if !isStealConflict(err) {
		return false, nil, nil, coreerrs.WrapOperation(err, "update during steal")
	}
	current, found, err = s.currentForSteal(ctx, key)
	if !found || err != nil {
		return false, nil, nil, err
	}
	return false, current.Value(), nil, nil
}

func (s *Storage) currentForSteal(ctx context.Context, key string) (jetstream.KeyValueEntry, bool, error) {
	entry, err := s.KV().Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, coreerrs.WrapOperation(err, "get current NATS value during steal")
	}
	return entry, true, nil
}

// KV.Update has no per-key TTL option. With a TTL override publish using
// the same subject and revision guard as KV.Update, with a TTL header.
func (s *Storage) updateForSteal(ctx context.Context, key string, val []byte, revision uint64, ttl time.Duration) (uint64, error) {
	if ttl <= 0 {
		return s.KV().Update(ctx, key, val, revision)
	}
	ack, err := s.js.Publish(ctx, putPrefix(s.js, s.opts.bucket)+key, val,
		jetstream.WithExpectStream("KV_"+s.opts.bucket),
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(ttl),
	)
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}

func putPrefix(js jetstream.JetStream, bucket string) string {
	opts := js.Options()
	prefix := strings.TrimSuffix(opts.APIPrefix, ".")
	if opts.Domain != "" {
		prefix = "$JS." + opts.Domain + ".API"
	}
	subject := "$KV." + bucket + "."
	if prefix == "" || prefix+"." == jetstream.DefaultAPIPrefix {
		return subject
	}
	return prefix + "." + subject
}

func isStealConflict(err error) bool {
	if errors.Is(err, jetstream.ErrKeyRevisionMismatch) ||
		errors.Is(err, jetstream.ErrKeyExists) ||
		errors.Is(err, jetstream.ErrKeyNotFound) {
		return true
	}
	apiErr, ok := errors.AsType[*jetstream.APIError](err)
	return ok && (apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence ||
		apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequenceConstant)
}

// Delete removes the key from storage.
func (s *Storage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	key, err := physicalKey(key)
	if err != nil {
		return err
	}

	if err := s.KV().Delete(ctx, key); err != nil {
		return coreerrs.WrapOperation(err, "delete idempotency key from NATS")
	}

	return nil
}

// Release deletes key only while lockToken names its current revision.
func (s *Storage) Release(ctx context.Context, key string, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	if len(lockToken) == 0 {
		return storages.ErrMissingLockState
	}
	key, err := physicalKey(key)
	if err != nil {
		return err
	}
	revision, err := decodeRevisionToken(lockToken)
	if err != nil {
		return err
	}
	if revision == 0 {
		return storages.ErrLockStolen
	}
	if err := s.KV().Delete(ctx, key, jetstream.LastRevision(revision)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return storages.ErrLockStolen
		}
		return coreerrs.WrapOperation(err, "release idempotency key in NATS")
	}
	return nil
}

// physicalKey maps a logical idempotency key onto the NATS KeyValue key it is
// stored under. Every operation goes through it exactly once, so the composite
// keys the transport middlewares and the Keeper build (with ':' and other
// bytes NATS rejects) reach the same entry everywhere.
func physicalKey(key string) (string, error) {
	physical, err := natskey.Encode(key)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "encode NATS KeyValue key")
	}
	return physical, nil
}
