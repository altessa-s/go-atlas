// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskey"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/reliability/saga"

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

// ErrKeyTooLong is returned, before any network I/O, for an instance ID whose
// escaped form exceeds the NATS KeyValue key budget (see the package README).
var ErrKeyTooLong = natskey.ErrKeyTooLong

// ErrInvalidID is returned, before any network I/O, for an instance ID that is
// not valid UTF-8. The instance is stored as JSON, which would rewrite such an
// ID, so it could not be found again under the ID it was created with.
var ErrInvalidID = errors.New("saga instance id is not valid UTF-8")

// indexPrefix marks the keys of the recoverable index, one per live instance:
// "=rec." followed by the instance's physical key. natskey.Encode writes '='
// only before two hex digits, so no instance key starts with "=r" and the index
// namespace cannot collide with an instance ID.
const indexPrefix = "=rec."

// indexGrace is how old a recoverable-index key must be before the scan drops
// it for a missing instance, which covers the window between Create writing
// the index key and the instance record.
const indexGrace = 5 * time.Minute

// Store is a durable [saga.Storage] backed by a NATS JetStream KeyValue bucket.
// Each saga instance is stored as a JSON document under its ID. The bucket's
// monotonically increasing revision is used directly as the optimistic-
// concurrency token ([saga.Instance.Version]), so two coordinators cannot
// advance the same instance — the loser's Update fails with
// [saga.ErrVersionConflict].
//
// Instance IDs are escaped into valid NATS KV keys, so any valid UTF-8 ID works;
// an ID that already is a valid key without '=' (ULIDs and UUIDs) is stored
// verbatim. An ID that is not valid UTF-8 is rejected with [ErrInvalidID].
//
// Live (non-terminal) instances are also listed in a recoverable index kept in
// the same bucket, one "=rec.<key>" entry each, so FetchRecoverable reads only
// live instances and never walks terminal ones. The index key is written before
// the instance record on Create and on every non-terminal Update, and removed
// after the terminal write; a crash between the two writes leaves at worst a
// dangling index key, which the scan removes, never a live instance without an
// index key. DeleteTerminal has no index to read and walks the whole bucket.
type Store struct {
	natsbase.Base
	opts *options
}

var _ saga.Storage = (*Store)(nil)

// New creates a Store, creating the KeyValue bucket on ctx if it does not
// exist. It returns an error if js is nil or the bucket cannot be provisioned.
//
// Example:
//
//	store, err := nats.New(ctx, js, nats.WithBucket("saga"))
func New(ctx context.Context, js jetstream.JetStream, opts ...Option) (*Store, error) {
	if js == nil {
		return nil, fmt.Errorf("JetStream context cannot be nil")
	}

	o := newOptions(opts...)

	base, err := natsbase.NewBaseWithBucket(ctx, js, bucketConfig(o), nil)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create NATS KeyValue bucket")
	}

	return &Store{Base: base, opts: o}, nil
}

// bucketConfig is the bucket New creates or reconciles.
func bucketConfig(o *options) natskvlease.BucketSpec {
	return natskvlease.BucketSpec{
		Bucket:     o.bucket,
		TTL:        o.bucketTTL,
		MigrateTTL: o.migrateBucketTTL,
		Storage:    jetstream.FileStorage,
	}
}

// Create stores a new instance, returning [saga.ErrInstanceExists] when the
// key is already present. The created revision is written back into
// inst.Version so the caller's first Update uses the correct token.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}

	key, err := physicalKey(inst.ID)
	if err != nil {
		return err
	}

	if !inst.Status.IsTerminal() {
		if err = s.putIndex(ctx, key); err != nil {
			return err
		}
	}
	rev, err := s.KV().Create(ctx, key, data)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return saga.ErrInstanceExists
		}
		return coreerrs.WrapOperation(classifyKeyError(err), "create saga instance in NATS")
	}

	inst.Version = revToVersion(rev)
	return nil
}

// Get loads an instance, returning [saga.ErrInstanceNotFound] when absent.
// The returned instance's Version reflects the current KV revision.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	key, err := physicalKey(id)
	if err != nil {
		return nil, err
	}

	entry, err := s.KV().Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, saga.ErrInstanceNotFound
		}
		return nil, coreerrs.WrapOperation(classifyKeyError(err), "get saga instance from NATS")
	}
	return decodeEntry(entry)
}

// Update overwrites the instance using a revision-checked write. A stale
// inst.Version (another coordinator advanced the instance) yields
// [saga.ErrVersionConflict]; a missing key yields
// [saga.ErrInstanceNotFound]. The new revision is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}

	key, err := physicalKey(inst.ID)
	if err != nil {
		return err
	}

	// KV revisions start at 1, so version 0 matches no stored instance; KV
	// Update would read revision 0 as "create if absent", so it is resolved
	// here without a write.
	if inst.Version == 0 {
		if _, gerr := s.KV().Get(ctx, key); errors.Is(gerr, jetstream.ErrKeyNotFound) {
			return saga.ErrInstanceNotFound
		} else if gerr != nil {
			return coreerrs.WrapOperation(classifyKeyError(gerr), "update saga instance in NATS")
		}
		return saga.ErrVersionConflict
	}

	terminal := inst.Status.IsTerminal()
	var marker jetstream.KeyValueEntry // Observed before the write; guards its removal.
	if terminal {
		marker = s.observeIndex(ctx, key)
	} else if err = s.putIndex(ctx, key); err != nil {
		return err
	}
	rev, err := s.KV().Update(ctx, key, data, versionToRev(inst.Version))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return saga.ErrInstanceNotFound
		}
		// ErrKeyExists is NATS's "wrong last sequence": either the revision we
		// expected is stale, or the key is gone entirely (an Update against a
		// missing key also reports wrong-last-sequence). Disambiguate with a
		// Get so the caller sees ErrInstanceNotFound vs ErrVersionConflict.
		if errors.Is(err, jetstream.ErrKeyExists) {
			if _, gerr := s.KV().Get(ctx, key); errors.Is(gerr, jetstream.ErrKeyNotFound) {
				return saga.ErrInstanceNotFound
			}
			return saga.ErrVersionConflict
		}
		return coreerrs.WrapOperation(classifyKeyError(err), "update saga instance in NATS")
	}

	inst.Version = revToVersion(rev)
	if terminal {
		s.dropIndex(ctx, marker) // Best effort: a dangling index key is dropped by the scan.
	}
	return nil
}

// observeIndex reads the recoverable-index key of the instance with physical key
// key, or returns nil when there is none or it cannot be read. A removal guarded
// by the observed revision never deletes a key a concurrent Create has written
// since.
func (s *Store) observeIndex(ctx context.Context, key string) jetstream.KeyValueEntry {
	entry, err := s.KV().Get(ctx, indexPrefix+key)
	if err != nil {
		return nil
	}
	return entry
}

// putIndex records the instance with physical key key in the recoverable index.
func (s *Store) putIndex(ctx context.Context, key string) error {
	if _, err := s.KV().Put(ctx, indexPrefix+key, []byte{'1'}); err != nil {
		return coreerrs.WrapOperation(classifyKeyError(err), "index saga instance in NATS")
	}
	return nil
}

// FetchRecoverable scans the recoverable index and returns up to limit
// non-terminal instances that are mid-compensation or past their deadline,
// restricted to definition when it is non-empty. Only live instances are read:
// the index key of a terminal instance, or of one that never materialized, is
// dropped as the scan meets it. An undecodable record is skipped. A
// non-positive limit means no cap.
func (s *Store) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	lister, err := s.KV().ListKeysFiltered(ctx, indexPrefix+">")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, coreerrs.WrapOperation(err, "list saga index in NATS")
	}
	defer closeLister(lister)

	var out []*saga.Instance
	for indexKey := range lister.Keys() {
		inst, candidate, err := s.examine(ctx, indexKey, definition, now)
		if err != nil {
			return nil, err
		}
		if !candidate {
			continue
		}
		out = append(out, inst)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// examine resolves one recoverable-index key during a scan. It returns the
// instance when it is a recovery candidate, a Corrupt stub when its record
// cannot be decoded, and nil when the key is not a candidate (terminal, leased,
// another definition, or dangling; dangling and terminal keys are dropped).
func (s *Store) examine(ctx context.Context, indexKey, definition string, now time.Time) (inst *saga.Instance, candidate bool, err error) {
	// The index entry is read first: its revision guards the delete below.
	idx, err := s.KV().Get(ctx, indexKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, coreerrs.WrapOperation(err, "get saga index key during scan")
	}
	physical := strings.TrimPrefix(indexKey, indexPrefix)
	entry, err := s.KV().Get(ctx, physical)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			if time.Since(idx.Created()) >= indexGrace {
				s.dropIndex(ctx, idx)
			}
			return nil, false, nil
		}
		return nil, false, coreerrs.WrapOperation(err, "get saga instance during scan")
	}

	inst, err = decodeEntry(entry)
	if err != nil {
		// Reported as a stub the orchestrator quarantines under a revision-checked
		// Update, so one bad record neither stops the scan nor is read again.
		id, derr := natskey.Decode(physical)
		if derr != nil {
			return nil, false, nil
		}
		return &saga.Instance{ID: id, Version: revToVersion(entry.Revision()), Corrupt: true}, true, nil
	}
	if inst.Status.IsTerminal() {
		s.dropIndex(ctx, idx)
		return nil, false, nil
	}
	if (definition != "" && inst.Definition != definition) || !inst.Recoverable(now) {
		return nil, false, nil
	}
	return inst, true, nil
}

// closeLister releases a key lister on every exit. Stop only ends the watcher;
// when the caller stopped reading early the producer can be blocked sending to
// the lister's full channel, so the rest is drained until the lister closes it.
func closeLister(l jetstream.KeyLister) {
	_ = l.Stop()
	go func() {
		for range l.Keys() { //nolint:revive // Drained to release the producer.
		}
	}()
}

// dropIndex deletes a recoverable-index key only if it is unchanged since it
// was read, so a concurrent re-index of the instance wins.
func (s *Store) dropIndex(ctx context.Context, idx jetstream.KeyValueEntry) {
	if idx == nil {
		return
	}
	_ = s.KV().Delete(ctx, idx.Key(), jetstream.LastRevision(idx.Revision())) // Best effort.
}

// DeleteTerminal removes up to limit terminal instances last updated before
// olderThan, restricted to definition when it is non-empty, and returns how
// many it removed. A non-positive limit means no cap. NATS KV has no query, so
// this walks every key of the bucket; the live-instance index is not used.
// Each delete is revision-checked, so an instance changed since it was read is
// kept.
func (s *Store) DeleteTerminal(ctx context.Context, definition string, olderThan time.Time, limit int) (int, error) {
	lister, err := s.KV().ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, coreerrs.WrapOperation(err, "list saga keys in NATS")
	}
	defer closeLister(lister)

	deleted := 0
	for key := range lister.Keys() {
		if strings.HasPrefix(key, indexPrefix) {
			continue
		}
		entry, err := s.KV().Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return deleted, coreerrs.WrapOperation(err, "get saga instance during retention")
		}
		inst, err := decodeEntry(entry)
		if err != nil {
			continue
		}
		if !inst.Status.IsTerminal() || !inst.UpdatedAt.Before(olderThan) || (definition != "" && inst.Definition != definition) {
			continue
		}
		if err := s.KV().Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue // Changed since it was read.
			}
			return deleted, coreerrs.WrapOperation(err, "delete terminal saga instance from NATS")
		}
		deleted++
		if limit > 0 && deleted >= limit {
			break
		}
	}
	return deleted, nil
}

// Delete removes an instance. NATS KV deletes are idempotent, so removing a
// missing instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	key, err := physicalKey(id)
	if err != nil {
		return err
	}

	marker := s.observeIndex(ctx, key)
	if err := s.KV().Delete(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return coreerrs.WrapOperation(classifyKeyError(err), "delete saga instance from NATS")
	}
	// Guarded by the marker revision observed before the delete: a Create of the
	// same ID that wrote a new marker since keeps it.
	s.dropIndex(ctx, marker)
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

// physicalKey maps an instance ID onto the NATS KeyValue key it is stored
// under. FetchRecoverable reads the listed physical keys directly and must not
// pass them through here.
func physicalKey(id string) (string, error) {
	if !utf8.ValidString(id) {
		return "", errors.Join(saga.ErrInvalidStorageKey, ErrInvalidID)
	}
	key, err := natskey.Encode(id)
	if err != nil {
		return "", coreerrs.WrapOperation(errors.Join(saga.ErrInvalidStorageKey, err), "encode NATS KeyValue key")
	}
	return key, nil
}

// classifyKeyError retains the client cause for callers inspecting storage diagnostics.
func classifyKeyError(err error) error {
	if errors.Is(err, jetstream.ErrInvalidKey) {
		return errors.Join(saga.ErrInvalidStorageKey, err)
	}
	return err
}
