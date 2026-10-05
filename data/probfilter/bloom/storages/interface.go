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

	// Stage creates an empty replacement filter for a rebuild, sized for
	// expectedItems (the configured size when expectedItems <= 0). The live
	// filter keeps serving unchanged until [Staging.Commit].
	Stage(ctx context.Context, expectedItems int64) (Staging, error)

	// LastRebuild returns the time of the last successful rebuild.
	LastRebuild() time.Time

	// SetLastRebuild updates the last rebuild timestamp.
	SetLastRebuild(t time.Time)

	// Close releases resources associated with the storage.
	Close(ctx context.Context) error
}

// Staging is a replacement Bloom filter populated while the live filter keeps
// serving. It is used by a single rebuild goroutine.
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

// ExclusiveRebuilder is implemented by storages shared between processes
// (Redis), whose rebuilds must be serialized: a rebuild first takes the
// storage's rebuild lease, before its data source is read, and only the lease
// holder can publish, so an older snapshot never overwrites a newer one.
type ExclusiveRebuilder interface {
	// BeginRebuild acquires the rebuild lease. It fails with an error wrapping
	// probfilter.ErrRebuildInProgress when another rebuild holds it.
	BeginRebuild(ctx context.Context) (RebuildLease, error)
}

// RebuildLease is a held rebuild lease.
type RebuildLease interface {
	// Stage creates the replacement filter of this rebuild, like
	// Storage.Stage; its Commit fails with an error wrapping
	// probfilter.ErrRebuildSuperseded once the lease was lost.
	Stage(ctx context.Context, expectedItems int64) (Staging, error)

	// Release gives the lease up.
	Release(ctx context.Context) error
}

// StatsProvider defines the interface for storage backends that provide statistics.
type StatsProvider interface {
	// Stats returns current filter statistics.
	Stats(ctx context.Context) (*stats.FilterStats, error)
}
