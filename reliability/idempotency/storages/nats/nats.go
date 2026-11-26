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

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskey"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

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

// ErrBucketPerKeyTTLRequired is returned by New when the bucket does not
// support the server-side TTL used by lock leases.
var ErrBucketPerKeyTTLRequired = errors.New("NATS KeyValue bucket must allow per-key TTL")

// ErrBucketHistoryMismatch is returned by New when a bucket stores multiple
// revisions per key. An older revision must never reappear after expiry.
var ErrBucketHistoryMismatch = errors.New("NATS KeyValue bucket must keep one revision per key")

// ErrMirroredBucket is returned by New when the bucket is a mirror.
var ErrMirroredBucket = errors.New("mirrored NATS KeyValue buckets are not supported")

// ErrKeyTooLong is returned, before any network I/O, for a key whose escaped
// form exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

// Storage is a NATS JetStream KeyValue-backed idempotency key store.
// JetStream expires lock leases. Completed records use the bucket MaxAge.
type Storage struct {
	natsbase.Base
	maxAge time.Duration
}

var (
	_ storages.Storage          = (*Storage)(nil)
	_ storages.LockLeaseStorage = (*Storage)(nil)
)

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

	ctx := context.Background()
	streamName := "KV_" + options.bucket
	stream, err := js.Stream(ctx, streamName)
	if err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		return nil, coreerrs.WrapOperation(err, "inspect NATS KeyValue stream")
	}
	if stream != nil {
		if shapeErr := validateBucketShape(stream.CachedInfo().Config); shapeErr != nil {
			return nil, shapeErr
		}
	}

	base, err := natsbase.NewBaseWithBucket(ctx, js, bucketConfig(options), nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	// Re-read after setup: the bucket may have been created or migrated.
	stream, err = js.Stream(ctx, streamName)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "inspect NATS KeyValue stream")
	}
	cfg := stream.CachedInfo().Config
	if err := validateBucketShape(cfg); err != nil {
		return nil, err
	}
	if !cfg.AllowMsgTTL {
		return nil, ErrBucketPerKeyTTLRequired
	}
	return &Storage{Base: base, maxAge: cfg.MaxAge}, nil
}

func validateBucketShape(cfg jetstream.StreamConfig) error {
	if cfg.Mirror != nil {
		return ErrMirroredBucket
	}
	if cfg.MaxMsgsPerSubject != 1 {
		return ErrBucketHistoryMismatch
	}
	return nil
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

// AttemptLockWithTTL creates a lock with a server-side lease. A positive
// lockTTL is rounded up to whole seconds and capped by the bucket's MaxAge.
// A nonpositive lockTTL uses the bucket's MaxAge.
func (s *Storage) AttemptLockWithTTL(ctx context.Context, key string, val []byte, lockTTL time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}
	key, err := physicalKey(key)
	if err != nil {
		return false, nil, nil, err
	}

	var createOpts []jetstream.KVCreateOpt
	if lockTTL > 0 && lockTTL < s.maxAge {
		// JetStream truncates per-key TTL to seconds. Use the bucket
		// limit if rounding reaches it, avoiding overflow in the addition.
		roundUp := (time.Second - lockTTL%time.Second) % time.Second
		if roundUp < s.maxAge-lockTTL {
			createOpts = []jetstream.KVCreateOpt{jetstream.KeyTTL(lockTTL + roundUp)}
		}
	}

	revision, err := s.KV().Create(ctx, key, encodeRecord(lockRecord, val), createOpts...)
	if isRevisionConflict(err) {
		// If the key expired between Create and Get, return no existing
		// value so the Keeper can retry the atomic acquisition.
		value, getErr := s.currentValue(ctx, key)
		return false, value, nil, getErr
	}
	if err != nil {
		return false, nil, nil, coreerrs.WrapOperation(err, "attempt lock in NATS")
	}

	return true, nil, encodeRevisionToken(revision), nil
}

// Complete marks the key as successfully processed.
//
// Uses [jetstream.KV.Update] with the revision encoded in lockToken to
// detect a stolen lock. Returns [storages.ErrLockStolen] when the
// revision no longer matches, including when the lease expires.
func (s *Storage) Complete(ctx context.Context, key string, val []byte, lockToken []byte) error {
	if key == "" {
		return storages.ErrEmptyKey
	}
	key, err := physicalKey(key)
	if err != nil {
		return err
	}

	val = encodeRecord(completedRecord, val)
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
		if isRevisionConflict(err) {
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
	revision := binary.BigEndian.Uint64(token)
	if revision == 0 {
		return 0, storages.ErrLockStolen
	}
	return revision, nil
}

// UsesLockLease reports that JetStream expires lock leases.
func (s *Storage) UsesLockLease() bool { return true }

// Steal returns the current value without replacing it. Use AttemptLock or
// AttemptLockWithTTL to acquire the key after JetStream expires its lease.
func (s *Storage) Steal(ctx context.Context, key string, _, _ []byte, _, _ time.Duration) (bool, []byte, []byte, error) {
	if key == "" {
		return false, nil, nil, storages.ErrEmptyKey
	}
	key, err := physicalKey(key)
	if err != nil {
		return false, nil, nil, err
	}
	value, err := s.currentValue(ctx, key)
	return false, value, nil, err
}

// Preserve the lock and completion tags when storing opaque caller payloads.
const (
	lockRecord      = "\x00idem\x01L"
	completedRecord = "\x00idem\x01C"
)

func encodeRecord(kind string, value []byte) []byte {
	record := make([]byte, 0, len(kind)+len(value))
	return append(append(record, kind...), value...)
}

func decodeRecord(record []byte) ([]byte, error) {
	if value, ok := bytes.CutPrefix(record, []byte(lockRecord)); ok {
		return value, nil
	}
	if value, ok := bytes.CutPrefix(record, []byte(completedRecord)); ok {
		return value, nil
	}
	return nil, errors.New("invalid NATS idempotency record")
}

func (s *Storage) currentValue(ctx context.Context, key string) ([]byte, error) {
	entry, err := s.KV().Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "get existing state from NATS")
	}
	return decodeRecord(entry.Value())
}

func isRevisionConflict(err error) bool {
	if errors.Is(err, jetstream.ErrKeyRevisionMismatch) ||
		errors.Is(err, jetstream.ErrKeyExists) ||
		errors.Is(err, jetstream.ErrKeyNotFound) ||
		errors.Is(err, jetstream.ErrKeyDeleted) {
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
	if err := s.KV().Delete(ctx, key, jetstream.LastRevision(revision)); err != nil {
		if isRevisionConflict(err) {
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
