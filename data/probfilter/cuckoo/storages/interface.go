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

	// Stage creates an empty replacement filter for a rebuild with room for at
	// least expectedItems (never less than the configured capacity). The live
	// filter keeps serving unchanged until [Staging.Commit].
	Stage(ctx context.Context, expectedItems int64) (Staging, error)

	// Close releases resources associated with the storage.
	Close(ctx context.Context) error
}

// Staging is a replacement Cuckoo filter populated while the live filter
// keeps serving. It is used by a single rebuild goroutine.
type Staging interface {
	// AddBatch inserts values into the replacement filter.
	AddBatch(ctx context.Context, values iter.Seq[string]) error

	// Commit atomically replaces the live filter with the replacement. It
	// fails without swapping when ctx is already canceled; on any error the
	// live filter is unchanged.
	Commit(ctx context.Context) error

	// Abort discards the replacement; the live filter is unchanged.
	Abort(ctx context.Context) error
}

// StatsProvider defines the interface for storage backends that provide statistics.
type StatsProvider interface {
	// Stats returns current filter statistics.
	Stats(ctx context.Context) (*stats.FilterStats, error)
}
