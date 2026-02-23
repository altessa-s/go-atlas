// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"context"
	"iter"
	"time"
)

// Filter defines the core interface for probabilistic filters.
// Implementations must be safe for concurrent use.
type Filter interface {
	// MightExist checks if a value might exist in the filter.
	// Returns true if the value might exist (with possible false positives).
	// Returns false if the value definitely does not exist.
	MightExist(ctx context.Context, value string) (bool, error)

	// Add inserts a value into the filter.
	Add(ctx context.Context, value string) error

	// AddBatch inserts multiple values into the filter.
	// More efficient than calling Add repeatedly for multiple values.
	AddBatch(ctx context.Context, values iter.Seq[string]) error
}

// StatsProvider defines the interface for filters that provide statistics.
type StatsProvider interface {
	// Stats returns current filter statistics.
	Stats(ctx context.Context) (*FilterStats, error)
}

// DeletableFilter extends Filter with deletion capability.
// Implemented by filters that support removal (e.g., Cuckoo filters).
type DeletableFilter interface {
	Filter

	// Delete removes a value from the filter.
	// Returns true if the value was found and removed.
	// Returns false if the value was not found.
	Delete(ctx context.Context, value string) (bool, error)
}

// RebuildableFilter extends Filter with rebuild capability.
// Implemented by filters that require periodic rebuilds (e.g., Bloom filters).
type RebuildableFilter interface {
	Filter

	// Rebuild recreates the filter from scratch using the provided data loader.
	// This is typically used to remove deleted items from Bloom filters.
	Rebuild(ctx context.Context, loader DataLoader) error

	// LastRebuild returns the time of the last successful rebuild.
	// Returns zero time if the filter has never been rebuilt.
	LastRebuild() time.Time
}
