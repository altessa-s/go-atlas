// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import "context"

// Status represents the state of an idempotency key.
type Status string

const (
	// StatusInProgress indicates the request is currently being processed.
	StatusInProgress Status = "IN_PROGRESS"
	// StatusSuccess indicates the request was completed successfully.
	StatusSuccess Status = "SUCCESS"
)

// State holds the current status and metadata of an idempotency key.
type State struct {
	Status Status `json:"status"`
	Data   any    `json:"data,omitempty"`
}

// Storage interface for idempotency key storage.
type Storage interface {
	// AttemptLock tries to acquire a lock for the given key.
	// It operates atomically:
	// - If key does not exist: Sets key to val and returns (true, nil, nil).
	// - If key exists: Returns (false, existingVal, nil).
	AttemptLock(ctx context.Context, key string, val []byte) (bool, []byte, error)

	// Complete marks the idempotency key as completed by updating its value.
	// It overwrites the existing value with val.
	Complete(ctx context.Context, key string, val []byte) error

	// Delete removes the key from storage (e.g. on failure).
	Delete(ctx context.Context, key string) error
}
