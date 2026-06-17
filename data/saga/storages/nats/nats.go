// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/data/internal/natsbase"
	"github.com/altessa-s/go-atlas/data/internal/natskvlease"
	"github.com/altessa-s/go-atlas/data/saga"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// Store is a durable [saga.Store] backed by a NATS JetStream KeyValue bucket.
// Each saga instance is stored as a JSON document under its ID. The bucket's
// monotonically increasing revision is used directly as the optimistic-
// concurrency token ([saga.Instance.Version]), so two coordinators cannot
// advance the same instance — the loser's Update fails with
// [sagaerrs.ErrVersionConflict].
//
// Instance IDs are used verbatim as KV keys and so must be valid NATS KV keys
// (letters, digits, and -_/=.). ULIDs and UUIDs qualify.
type Store struct {
	natsbase.Base
	opts *options
}

var _ saga.Store = (*Store)(nil)

// New creates a Store, creating the KeyValue bucket if it does not exist.
// It returns an error if js is nil or the bucket cannot be provisioned.
//
// Example:
//
//	store, err := nats.New(js, nats.WithBucket("saga"))
func New(js jetstream.JetStream, opts ...Option) (*Store, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context cannot be nil")
	}

	o := newOptions(opts...)

	base, err := natsbase.NewBaseWithBucket(context.Background(), js, natskvlease.BucketConfig{
		Bucket:  o.bucket,
		TTL:     o.bucketTTL,
		Storage: jetstream.FileStorage,
	}, nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Store{Base: base, opts: o}, nil
}

// Create stores a new instance, returning [sagaerrs.ErrInstanceExists] when the
// key is already present. The created revision is written back into
// inst.Version so the caller's first Update uses the correct token.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}

	rev, err := s.KV().Create(ctx, inst.ID, data)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return sagaerrs.ErrInstanceExists
		}
		return coreerrs.WrapOperation(err, "create saga instance in NATS")
	}

	inst.Version = revToVersion(rev)
	return nil
}

// Get loads an instance, returning [sagaerrs.ErrInstanceNotFound] when absent.
// The returned instance's Version reflects the current KV revision.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	entry, err := s.KV().Get(ctx, id)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, sagaerrs.ErrInstanceNotFound
		}
		return nil, coreerrs.WrapOperation(err, "get saga instance from NATS")
	}
	return decodeEntry(entry)
}

// Update overwrites the instance using a revision-checked write. A stale
// inst.Version (another coordinator advanced the instance) yields
// [sagaerrs.ErrVersionConflict]; a missing key yields
// [sagaerrs.ErrInstanceNotFound]. The new revision is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}

	rev, err := s.KV().Update(ctx, inst.ID, data, versionToRev(inst.Version))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return sagaerrs.ErrInstanceNotFound
		}
		// ErrKeyExists is NATS's "wrong last sequence": either the revision we
		// expected is stale, or the key is gone entirely (an Update against a
		// missing key also reports wrong-last-sequence). Disambiguate with a
		// Get so the caller sees ErrInstanceNotFound vs ErrVersionConflict.
		if errors.Is(err, jetstream.ErrKeyExists) {
			if _, gerr := s.KV().Get(ctx, inst.ID); errors.Is(gerr, jetstream.ErrKeyNotFound) {
				return sagaerrs.ErrInstanceNotFound
			}
			return sagaerrs.ErrVersionConflict
		}
		return coreerrs.WrapOperation(err, "update saga instance in NATS")
	}

	inst.Version = revToVersion(rev)
	return nil
}

// FetchRecoverable scans the bucket and returns up to limit non-terminal
// instances that are mid-compensation or past their deadline. NATS KV has no
// query support, so this lists every key and reads each one; durable
// deployments with very large instance counts should prefer a query-capable
// backend. A non-positive limit means no cap.
func (s *Store) FetchRecoverable(ctx context.Context, now time.Time, limit int) ([]*saga.Instance, error) {
	lister, err := s.KV().ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, coreerrs.WrapOperation(err, "list saga keys in NATS")
	}
	defer func() { _ = lister.Stop() }()

	var out []*saga.Instance
	for key := range lister.Keys() {
		entry, err := s.KV().Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue // Deleted between listing and reading.
			}
			return nil, coreerrs.WrapOperation(err, "get saga instance during scan")
		}

		inst, err := decodeEntry(entry)
		if err != nil {
			return nil, err
		}
		if inst.Status.IsTerminal() {
			continue
		}
		timedOut := !inst.Deadline.IsZero() && !now.Before(inst.Deadline)
		if inst.Status != saga.StatusCompensating && !timedOut {
			continue
		}

		out = append(out, inst)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Delete removes an instance. NATS KV deletes are idempotent, so removing a
// missing instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := s.KV().Delete(ctx, id); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return coreerrs.WrapOperation(err, "delete saga instance from NATS")
	}
	return nil
}

// decodeEntry unmarshals a KV entry into an instance and stamps its Version
// with the entry's revision (the authoritative concurrency token).
func decodeEntry(entry jetstream.KeyValueEntry) (*saga.Instance, error) {
	var inst saga.Instance
	if err := json.Unmarshal(entry.Value(), &inst); err != nil {
		return nil, coreerrs.WrapOperation(err, "unmarshal saga instance")
	}
	inst.Version = revToVersion(entry.Revision())
	return &inst, nil
}

// revToVersion / versionToRev bridge the NATS KV revision (uint64) and the
// saga Version (int64). Revisions start at 1 and never approach the int64
// ceiling, and Version is always non-negative, so the conversions are safe.
func revToVersion(rev uint64) int64 { return int64(rev) } //nolint:gosec // KV revision fits int64
func versionToRev(v int64) uint64   { return uint64(v) }  //nolint:gosec // Version is non-negative
