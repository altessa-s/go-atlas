// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"iter"
	"time"

	"github.com/altessa-s/go-atlas/cache/probfilter/stats"
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

// RebuildCommitReporter is implemented by storages shared between processes
// (Redis) that can tell whether the live filter holds the contents of a
// rebuild committed by any process; see
// probfilter.RebuildCommitReporter.
type RebuildCommitReporter interface {
	// RebuildCommitted reports whether a committed rebuild published the
	// live filter and the filter has not been deleted or recreated since.
	RebuildCommitted(ctx context.Context) (bool, error)
}

// RebuildCommitTimeReporter is implemented by storages shared between
// processes (Redis) that record when the rebuild that published the live
// filter committed; see probfilter.RebuildCommitTimeReporter.
type RebuildCommitTimeReporter interface {
	// LastRebuildCommit returns the commit time, on the storage's own clock,
	// of the last rebuild committed by any process, or the zero time when
	// RebuildCommitted would report false or the time was not recorded.
	LastRebuildCommit(ctx context.Context) (time.Time, error)
}

// RebuildCommitAgeReporter is implemented by storages shared between
// processes (Redis) that measure the age of the last commit on their own
// clock; see probfilter.RebuildCommitAgeReporter.
type RebuildCommitAgeReporter interface {
	// RebuildCommitAge returns the age and the commit time of the last
	// rebuild committed by any process, both on the storage's clock; known
	// is false when LastRebuildCommit would report the zero time.
	RebuildCommitAge(ctx context.Context) (age time.Duration, committed time.Time, known bool, err error)
}

// RebuildLoadReporter is implemented by storages shared between processes
// (Redis) that record, with the commit, how many values the rebuild loaded
// from its data source; see probfilter.RebuildLoadReporter.
type RebuildLoadReporter interface {
	// LastRebuildLoaded returns the number of values loaded by the last
	// rebuild committed by any process; known is false when RebuildCommitted
	// would report false or the count was not recorded.
	LastRebuildLoaded(ctx context.Context) (loaded int64, known bool, err error)
}

// StatsProvider defines the interface for storage backends that provide statistics.
type StatsProvider interface {
	// Stats returns current filter statistics.
	Stats(ctx context.Context) (*stats.FilterStats, error)
}
