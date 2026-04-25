// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"iter"

	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

// Storage defines the interface for Cuckoo filter storage backends.
// Implementations must be safe for concurrent use.
type Storage interface {
	// MightExist checks if a value might exist in the filter.
	MightExist(ctx context.Context, value string) (bool, error)

	// Add inserts a value into the filter.
	// Returns error if the filter is full.
	Add(ctx context.Context, value string) error

	// AddBatch inserts multiple values into the filter.
	// Returns error if the filter becomes full.
	AddBatch(ctx context.Context, values iter.Seq[string]) error

	// Delete removes a value from the filter.
	// Returns true if the value was found and removed.
	Delete(ctx context.Context, value string) (bool, error)

	// Close releases resources associated with the storage.
	Close(ctx context.Context) error
}

// StatsProvider defines the interface for storage backends that provide statistics.
type StatsProvider interface {
	// Stats returns current filter statistics.
	Stats(ctx context.Context) (*stats.FilterStats, error)
}
