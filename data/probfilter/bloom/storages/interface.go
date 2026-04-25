// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"iter"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter/stats"
)

// Storage defines the interface for Bloom filter storage backends.
// Implementations must be safe for concurrent use.
type Storage interface {
	// MightExist checks if a value might exist in the filter.
	MightExist(ctx context.Context, value string) (bool, error)

	// Add inserts a value into the filter.
	Add(ctx context.Context, value string) error

	// AddBatch inserts multiple values into the filter.
	AddBatch(ctx context.Context, values iter.Seq[string]) error

	// Reset clears the filter and prepares it for rebuild.
	// If expectedItems > 0, the filter may resize to accommodate the expected items.
	Reset(ctx context.Context, expectedItems int64) error

	// LastRebuild returns the time of the last successful rebuild.
	LastRebuild() time.Time

	// SetLastRebuild updates the last rebuild timestamp.
	SetLastRebuild(t time.Time)

	// Close releases resources associated with the storage.
	Close(ctx context.Context) error
}

// StatsProvider defines the interface for storage backends that provide statistics.
type StatsProvider interface {
	// Stats returns current filter statistics.
	Stats(ctx context.Context) (*stats.FilterStats, error)
}
