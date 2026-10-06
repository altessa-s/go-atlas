// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter

import (
	"context"
	"errors"
	"iter"
	"time"
)

// ErrFilterClosed is returned by [RebuildableFilter.Rebuild] once the filter
// has been closed, so scheduled rebuilds of a discarded filter become no-ops.
var ErrFilterClosed = errors.New("filter closed")

// ErrRebuildInProgress is wrapped by a Rebuild error when another process is
// rebuilding the same shared filter right now; nothing was loaded. That
// rebuild publishes a fresh snapshot, so there is nothing to retry.
var ErrRebuildInProgress = errors.New("filter rebuild in progress elsewhere")

// ErrUnsafeEvictionPolicy is wrapped by the error of a Redis eviction-policy
// check when a server evicts any key under memory pressure (an allkeys-*
// maxmemory-policy): a shared filter's keys carry no TTL by design, and
// losing them breaks its rebuild and delete guarantees.
var ErrUnsafeEvictionPolicy = errors.New("redis maxmemory-policy may evict filter keys")

// ErrRebuildSuperseded is wrapped by a Rebuild error when the rebuild lost its
// right to publish — its rebuild lease of a shared filter expired (for
// example because the process stalled) and another rebuild took over — so its
// possibly outdated snapshot was discarded; the previous contents stay.
var ErrRebuildSuperseded = errors.New("filter rebuild superseded")

// ErrCommitIndeterminate is wrapped by a Rebuild error when the rebuilt
// contents may or may not have replaced the previous ones — for example a
// Redis commit whose reply was lost and whose outcome could not be checked.
// Either the previous or the rebuilt contents are then in place.
var ErrCommitIndeterminate = errors.New("rebuild commit outcome unknown")

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
// Implemented by filters that are periodically repopulated from their source
// of truth (Bloom and Cuckoo filters).
type RebuildableFilter interface {
	Filter

	// Rebuild recreates the filter from scratch using the provided data loader.
	// This is typically used to remove deleted items from Bloom filters.
	//
	// Implementations must rebuild atomically: the previous contents stay
	// visible to lookups until the rebuilt contents replace them in one step,
	// a failed or canceled rebuild leaves them unchanged, and values added
	// through the filter while the rebuild runs are present afterwards. The
	// one exception is an error wrapping [ErrCommitIndeterminate]: then either
	// the previous or the rebuilt contents are in place.
	Rebuild(ctx context.Context, loader DataLoader) error

	// LastRebuild returns the time of the last successful rebuild.
	// Returns zero time if the filter has never been rebuilt.
	LastRebuild() time.Time
}

// RebuildCommitReporter is implemented by filters that can tell whether their
// current contents come from a committed rebuild — including one committed by
// another process sharing the filter (the Bloom and Cuckoo facades do).
type RebuildCommitReporter interface {
	// RebuildCommitted reports whether the filter holds the contents of a
	// committed [RebuildableFilter.Rebuild] plus the values added since. For
	// a filter shared through Redis it is true once any process committed a
	// rebuild and the filter key has not been deleted or recreated since;
	// for a process-local filter it is true after a successful Rebuild of
	// this filter. An error means the state could not be established.
	RebuildCommitted(ctx context.Context) (bool, error)
}

// Observer receives operation outcomes from an [ObservableFilter].
// Implementations must be safe for concurrent use and cheap: ObserveLookup
// runs on the lookup hot path.
type Observer interface {
	// ObserveLookup reports one MightExist call: its answer, its error, and
	// how long it took.
	ObserveLookup(found bool, err error, elapsed time.Duration)

	// ObserveAdd reports n values successfully added by Add or AddBatch.
	ObserveAdd(n int)

	// ObserveRebuild reports one Rebuild call: how long it took and its error.
	ObserveRebuild(elapsed time.Duration, err error)
}

// ObservableFilter is a Filter that reports its operations to an [Observer].
// [Manager.Register] attaches an observer that records the probfilter metrics
// when the Manager has a metrics collector.
type ObservableFilter interface {
	Filter

	// SetObserver installs o; nil removes the current observer.
	SetObserver(o Observer)
}
