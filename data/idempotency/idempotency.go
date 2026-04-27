// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// serializedState is the on-the-wire shape Keeper writes through the
// underlying Storage. The Nonce field is added on every AttemptLock to
// guarantee that two consecutive in-progress writes for the same key
// produce distinct bytes — required for byte-equality CAS guards in
// the memory and redis backends. NATS uses revisions and ignores the
// nonce. The public [State] type does NOT carry this field.
type serializedState struct {
	Status storages.Status `json:"status"`
	Data   any             `json:"data,omitempty"`
	Nonce  string          `json:"nonce,omitempty"`
}

// Re-export types from storages for convenience
type (
	Status = storages.Status
	State  = storages.State
)

const (
	// StatusInProgress indicates the request is currently being processed.
	StatusInProgress = storages.StatusInProgress
	// StatusSuccess indicates the request was completed successfully.
	StatusSuccess = storages.StatusSuccess
)

// ErrEmptyKey is returned by [Keeper] when an empty idempotency key is
// supplied. Re-exported from [storages.ErrEmptyKey] for convenience —
// `errors.Is(err, idempotency.ErrEmptyKey)` matches both layers.
var ErrEmptyKey = storages.ErrEmptyKey

// ErrLockStolen is returned by [Keeper.Complete] when the lock has
// been taken over by another holder between AttemptLock and Complete
// (typically because the lock TTL expired during processing).
// Re-exported from [storages.ErrLockStolen].
var ErrLockStolen = storages.ErrLockStolen

// ErrMissingLockState is returned by [Keeper.Complete] when the caller
// passes a nil lock state. The state returned by AttemptLock carries
// the CAS token; passing nil would silently disable the stolen-lock
// guard. Re-exported from [storages.ErrMissingLockState].
var ErrMissingLockState = storages.ErrMissingLockState

// Idempotency defines the interface for idempotency key operations.
// Implementations must be safe for concurrent use.
type Idempotency interface {
	// AttemptLock tries to acquire a lock for the given key.
	// On success returns (true, *State, nil) where *State carries an
	// opaque CAS token; pass it back to [Idempotency.Complete] to
	// detect a stolen lock.
	// On collision returns (false, *State, nil) with the State of
	// the existing holder.
	AttemptLock(ctx context.Context, key string) (bool, *storages.State, error)

	// Complete marks the key as successfully processed.
	// lockState must be the *State returned by AttemptLock that
	// acquired the lock; passing nil yields [ErrMissingLockState].
	// Returns [ErrLockStolen] when the lock has been taken over by
	// another holder since AttemptLock.
	Complete(ctx context.Context, key string, data any, lockState *storages.State) error

	// Delete removes the key from storage (e.g. on failure).
	Delete(ctx context.Context, key string) error
}

// Keeper provides duplicate request detection using idempotency keys.
// It supports multiple storage backends for distributed systems.
type Keeper struct {
	storage storages.Storage
	opts    *options
	metrics *keeperMetrics
}

var _ Idempotency = (*Keeper)(nil)

// New creates a new Keeper with the specified storage backend.
//
// Example:
//
//	keeper := idempotency.New(memoryStorage)
func New(storage storages.Storage, opt ...Option) *Keeper {
	options := newOptions(opt...)

	if options.serializer == nil {
		options.serializer = &serializer.JSON{}
	}

	return &Keeper{
		storage: storage,
		opts:    options,
		metrics: newKeeperMetrics(options.collector),
	}
}

// check operations are not directly supported via Keeper in new interface, as flow should be AttemptLock -> [Work] -> Complete/Delete

// AttemptLock tries to acquire a lock for the given key. An empty key
// returns [ErrEmptyKey] — silently succeeding would disable dedupe for
// any caller that reaches Keeper without validating its input.
func (i *Keeper) AttemptLock(ctx context.Context, key string) (bool, *storages.State, error) {
	if key == "" {
		return false, nil, ErrEmptyKey
	}

	// Embed a per-attempt nonce so two consecutive lock attempts on
	// the same key produce distinct serialized bytes. Storage backends
	// that CAS on byte equality (memory, redis) need this to tell two
	// holders apart. NATS uses revisions and ignores the field.
	wire := serializedState{
		Status: storages.StatusInProgress,
		Nonce:  uuid.NewString(),
	}

	val, err := i.opts.serializer.Serialize(wire)
	if err != nil {
		return false, nil, coreerrs.WrapOperation(err, "serialize state")
	}

	ok, existingVal, lockToken, err := i.storage.AttemptLock(ctx, key, val)
	if err != nil {
		i.metrics.errors.Inc()
		return false, nil, err
	}

	if ok {
		i.metrics.locksAcquired.Inc()
		// Carry the CAS token on the State so Complete can validate
		// our claim later. The State itself isn't serialized at this
		// point; we hand the freshly-built object back to the caller.
		acquired := &storages.State{Status: storages.StatusInProgress}
		acquired.SetLockToken(lockToken)
		return true, acquired, nil
	}

	i.metrics.locksDenied.Inc()

	// Lock failed, key exists. Deserialize existing state via the
	// wire shape and strip the nonce before handing back to the
	// caller — they shouldn't need to know about CAS plumbing.
	var existing serializedState
	if err := i.opts.serializer.Deserialize(existingVal, &existing); err != nil {
		return false, nil, coreerrs.WrapOperation(err, "deserialize existing state")
	}

	return false, &storages.State{Status: existing.Status, Data: existing.Data}, nil
}

// Complete marks the key as successfully processed. The lockState must
// be the *State returned by [Keeper.AttemptLock] that acquired the
// lock — its embedded CAS token gates the write. An empty key returns
// [ErrEmptyKey]; a nil lockState returns [ErrMissingLockState];
// a stolen lock returns [ErrLockStolen].
func (i *Keeper) Complete(ctx context.Context, key string, data any, lockState *storages.State) error {
	if key == "" {
		return ErrEmptyKey
	}
	if lockState == nil {
		return ErrMissingLockState
	}

	state := storages.State{
		Status: storages.StatusSuccess,
		Data:   data,
	}

	val, err := i.opts.serializer.Serialize(state)
	if err != nil {
		return coreerrs.WrapOperation(err, "serialize state")
	}

	if err := i.storage.Complete(ctx, key, val, lockState.LockToken()); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.completions.Inc()
	return nil
}

// Delete removes the key from storage (e.g. on failure). An empty key
// returns [ErrEmptyKey].
func (i *Keeper) Delete(ctx context.Context, key string) error {
	if key == "" {
		return ErrEmptyKey
	}
	if err := i.storage.Delete(ctx, key); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.deletions.Inc()
	return nil
}
