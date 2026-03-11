// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

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

// Idempotency defines the interface for idempotency key operations.
// Implementations must be safe for concurrent use.
type Idempotency interface {
	// AttemptLock tries to acquire a lock for the given key.
	// Returns true if the lock was acquired (key didn't exist).
	// Returns false and the current state if the key already exists.
	AttemptLock(ctx context.Context, key string) (bool, *storages.State, error)

	// Complete marks the key as successfully processed.
	// data is optional and used for result storage.
	Complete(ctx context.Context, key string, data any) error

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

// AttemptLock tries to acquire a lock for the given key.
func (i *Keeper) AttemptLock(ctx context.Context, key string) (bool, *storages.State, error) {
	if key == "" {
		return true, nil, nil
	}

	state := storages.State{
		Status: storages.StatusInProgress,
	}

	val, err := i.opts.serializer.Serialize(state)
	if err != nil {
		return false, nil, coreerrs.WrapOperation(err, "serialize state")
	}

	ok, existingVal, err := i.storage.AttemptLock(ctx, key, val)
	if err != nil {
		i.metrics.errors.Inc()
		return false, nil, err
	}

	if ok {
		i.metrics.locksAcquired.Inc()
		return true, nil, nil
	}

	i.metrics.locksDenied.Inc()

	// Lock failed, key exists. Deserialize existing state.
	var existingState storages.State
	if err := i.opts.serializer.Deserialize(existingVal, &existingState); err != nil {
		return false, nil, coreerrs.WrapOperation(err, "deserialize existing state")
	}

	return false, &existingState, nil
}

// Complete marks the key as successfully processed.
func (i *Keeper) Complete(ctx context.Context, key string, data any) error {
	if key == "" {
		return nil
	}

	state := storages.State{
		Status: storages.StatusSuccess,
		Data:   data,
	}

	val, err := i.opts.serializer.Serialize(state)
	if err != nil {
		return coreerrs.WrapOperation(err, "serialize state")
	}

	if err := i.storage.Complete(ctx, key, val); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.completions.Inc()
	return nil
}

// Delete removes the key from storage (e.g. on failure).
func (i *Keeper) Delete(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	if err := i.storage.Delete(ctx, key); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.deletions.Inc()
	return nil
}
